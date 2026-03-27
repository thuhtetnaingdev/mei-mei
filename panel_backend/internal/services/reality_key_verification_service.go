package services

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"panel_backend/internal/models"
	"strings"
	"time"

	"golang.org/x/crypto/curve25519"
	"gorm.io/gorm"
)

// RealityKeyVerificationService handles verification and auto-fix of VLESS REALITY keys
type RealityKeyVerificationService struct {
	db          *gorm.DB
	nodeService *NodeService
	httpClient  *http.Client
	sharedToken string
}

// KeyVerificationResult represents the result of verifying keys on a single node
type KeyVerificationResult struct {
	NodeID           uint      `json:"nodeId"`
	NodeName         string    `json:"nodeName"`
	PanelPublicKey   string    `json:"panelPublicKey"`
	PanelShortID     string    `json:"panelShortId"`
	NodePublicKey    string    `json:"nodePublicKey"`
	NodeShortID      string    `json:"nodeShortId"`
	PublicKeyMatch   bool      `json:"publicKeyMatch"`
	ShortIDMatch     bool      `json:"shortIDMatch"`
	Status           string    `json:"status"` // "verified", "mismatch", "node_unreachable"
	Error            string    `json:"error,omitempty"`
	VerifiedAt       time.Time `json:"verifiedAt"`
	AutoFixTriggered bool      `json:"autoFixTriggered"`
	AutoFixSuccess   bool      `json:"autoFixSuccess"`
}

// KeyMismatchHistory represents a record of key mismatch detection and fix
type KeyMismatchHistory struct {
	ID        uint       `json:"id"`
	NodeID    uint       `json:"nodeId"`
	DetectedAt time.Time `json:"detectedAt"`
	FixedAt   *time.Time `json:"fixedAt"`
	Reason    string     `json:"reason"` // "scheduled_verification", "manual_check", "node_reinstall"
	Success   bool       `json:"success"`
	Error     string     `json:"error,omitempty"`
}

// NewRealityKeyVerificationService creates a new RealityKeyVerificationService
func NewRealityKeyVerificationService(
	db *gorm.DB,
	nodeService *NodeService,
	sharedToken string,
	requestTimeout time.Duration,
) *RealityKeyVerificationService {
	return &RealityKeyVerificationService{
		db:          db,
		nodeService: nodeService,
		httpClient:  &http.Client{Timeout: requestTimeout},
		sharedToken: sharedToken,
	}
}

// VerifyAllNodes verifies keys for all nodes and returns results
func (s *RealityKeyVerificationService) VerifyAllNodes() ([]KeyVerificationResult, error) {
	var nodes []models.Node
	if err := s.db.Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	results := make([]KeyVerificationResult, 0, len(nodes))
	for _, node := range nodes {
		result, err := s.VerifySingleNode(node.ID)
		if err != nil {
			result = &KeyVerificationResult{
				NodeID:     node.ID,
				NodeName:   node.Name,
				Status:     "error",
				Error:      err.Error(),
				VerifiedAt: time.Now(),
			}
		}
		results = append(results, *result)
	}

	return results, nil
}

// VerifySingleNode verifies keys for a single node
func (s *RealityKeyVerificationService) VerifySingleNode(nodeID uint) (*KeyVerificationResult, error) {
	var node models.Node
	if err := s.db.First(&node, "id = ?", nodeID).Error; err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	result := &KeyVerificationResult{
		NodeID:         node.ID,
		NodeName:       node.Name,
		PanelPublicKey: node.RealityPublicKey,
		PanelShortID:   node.RealityShortID,
		VerifiedAt:     time.Now(),
	}

	// Fetch node status via API
	nodeStatus, err := s.fetchNodeStatus(node)
	if err != nil {
		result.Status = "node_unreachable"
		result.Error = err.Error()
		// Update verification timestamp even on failure
		now := time.Now()
		s.db.Model(&node).Updates(map[string]interface{}{
			"last_key_verification_at": &now,
		})
		return result, nil // Return gracefully, not an error
	}

	result.NodePublicKey = nodeStatus.RealityPublicKey
	result.NodeShortID = nodeStatus.RealityShortID

	// Compare keys
	result.PublicKeyMatch = (node.RealityPublicKey == nodeStatus.RealityPublicKey)
	result.ShortIDMatch = (node.RealityShortID == nodeStatus.RealityShortID)

	// Update node's syncVerificationStatus for frontend display
	updates := map[string]interface{}{
		"last_key_verification_at": time.Now(),
	}

	if result.PublicKeyMatch && result.ShortIDMatch {
		result.Status = "verified"
		updates["sync_verification_status"] = "verified"
		updates["sync_verification_error"] = ""
		// Check if this verification follows a recent auto-fix
		if node.KeyMismatchAutoFixedAt != nil && node.KeyMismatchDetectedAt == nil {
			result.AutoFixSuccess = true
		}
	} else {
		result.Status = "mismatch"
		updates["sync_verification_status"] = "mismatch"
		updates["sync_verification_error"] = "REALITY keys mismatch"
		// Record mismatch detection time if not already set
		if node.KeyMismatchDetectedAt == nil {
			updates["key_mismatch_detected_at"] = time.Now()
		}
	}

	s.db.Model(&node).Updates(updates)

	return result, nil
}

// AutoFixMismatch generates new keys and pushes them to the node
func (s *RealityKeyVerificationService) AutoFixMismatch(nodeID uint) error {
	var node models.Node
	if err := s.db.First(&node, "id = ?", nodeID).Error; err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	// Record mismatch detection time
	now := time.Now()
	if node.KeyMismatchDetectedAt == nil {
		s.db.Model(&node).Updates(map[string]interface{}{
			"key_mismatch_detected_at": &now,
		})
	}

	// Generate new keypair
	privateKey, publicKey, shortID, err := s.generateNewKeypair()
	if err != nil {
		return fmt.Errorf("failed to generate keypair: %w", err)
	}

	privateKeyHash := s.hashPrivateKey(privateKey)

	// Update panel DB
	if err := s.updatePanelDB(node.ID, publicKey, shortID, privateKeyHash); err != nil {
		return fmt.Errorf("failed to update panel DB: %w", err)
	}

	// Push keys to node via SSH
	if err := s.pushKeysToNodeViaSSH(node, privateKey, publicKey, shortID); err != nil {
		return fmt.Errorf("failed to push keys to node: %w", err)
	}

	// Wait for node to come back online and restart sing-box
	time.Sleep(5 * time.Second)

	// Verify fix
	result, err := s.VerifySingleNode(nodeID)
	if err != nil {
		return fmt.Errorf("failed to verify fix: %w", err)
	}

	if result.Status == "verified" {
		fixedAt := time.Now()
		s.db.Model(&node).Updates(map[string]interface{}{
			"key_mismatch_auto_fixed_at": &fixedAt,
			"key_mismatch_detected_at":   nil,
		})
		return nil
	}

	return errors.New("auto-fix completed but verification failed")
}

// generateNewKeypair generates a new REALITY keypair using curve25519
func (s *RealityKeyVerificationService) generateNewKeypair() (string, string, string, error) {
	// Generate private key (32 bytes)
	var privateKey [32]byte
	if _, err := io.ReadFull(rand.Reader, privateKey[:]); err != nil {
		return "", "", "", err
	}

	// Clamp private key for curve25519
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	// Derive public key
	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return "", "", "", err
	}

	// Generate short ID (8 hex characters = 4 bytes)
	var shortIDBytes [4]byte
	if _, err := io.ReadFull(rand.Reader, shortIDBytes[:]); err != nil {
		return "", "", "", err
	}

	// Use base64url encoding WITHOUT padding (sing-box standard)
	// This ensures consistency between node .env, database, and subscriptions
	privateKeyB64 := base64.RawURLEncoding.EncodeToString(privateKey[:])
	publicKeyB64 := base64.RawURLEncoding.EncodeToString(publicKey)
	return privateKeyB64, publicKeyB64, hex.EncodeToString(shortIDBytes[:]), nil
}

// hashPrivateKey returns SHA256 hash of private key for audit purposes
func (s *RealityKeyVerificationService) hashPrivateKey(privateKey string) string {
	sum := sha256.Sum256([]byte(privateKey))
	return hex.EncodeToString(sum[:])
}

// updatePanelDB updates the panel database with new keys
func (s *RealityKeyVerificationService) updatePanelDB(nodeID uint, publicKey, shortID, privateKeyHash string) error {
	return s.db.Model(&models.Node{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
		"reality_public_key":       publicKey,
		"reality_short_id":         shortID,
		"reality_private_key_hash": privateKeyHash,
	}).Error
}

// pushKeysToNodeViaSSH pushes new keys to the node.
// Prefer the node API so the running node process can update its in-memory
// config and rewrite the live sing-box config before reload. Fall back to SSH
// only for older nodes that do not expose the update endpoint correctly.
func (s *RealityKeyVerificationService) pushKeysToNodeViaSSH(node models.Node, privateKey, publicKey, shortID string) error {
	if err := s.pushKeysToNodeViaAPI(node, privateKey, publicKey, shortID); err == nil {
		return nil
	}

	if node.SSHPrivateKey == "" || node.SSHUsername == "" {
		return errors.New("SSH credentials not available for this node")
	}

	sshHost := node.SSHHost
	if sshHost == "" {
		sshHost = extractSSHHost(node.BaseURL)
	}
	if sshHost == "" {
		return errors.New("SSH host information missing")
	}

	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	client, err := s.nodeService.openSSHClientWithPrivateKey(sshHost, sshPort, node.SSHUsername, node.SSHPrivateKey)
	if err != nil {
		return err
	}
	defer client.Close()

	// Update .env file
	envPath := "/opt/meimei-node/.env"
	// Use | as delimiter to avoid conflicts with / in base64 encoded keys
	commands := []string{
		fmt.Sprintf("sed -i 's|^VLESS_REALITY_PRIVATE_KEY=.*|VLESS_REALITY_PRIVATE_KEY=%s|' %s", shellQuoteString(privateKey), shellQuoteString(envPath)),
		fmt.Sprintf("sed -i 's|^VLESS_REALITY_PUBLIC_KEY=.*|VLESS_REALITY_PUBLIC_KEY=%s|' %s", shellQuoteString(publicKey), shellQuoteString(envPath)),
		fmt.Sprintf("sed -i 's|^VLESS_REALITY_SHORT_ID=.*|VLESS_REALITY_SHORT_ID=%s|' %s", shellQuoteString(shortID), shellQuoteString(envPath)),
		"systemctl restart meimei-sing-box.service",
	}

	for _, cmd := range commands {
		if _, err := runSimpleRemoteCommand(client, cmd); err != nil {
			return fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return nil
}

func (s *RealityKeyVerificationService) pushKeysToNodeViaAPI(node models.Node, privateKey, publicKey, shortID string) error {
	payload, err := json.Marshal(map[string]string{
		"privateKey": privateKey,
		"publicKey":  publicKey,
		"shortId":    shortID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/update-reality-keys", node.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+node.ProtocolToken)
	req.Header.Set("X-Control-Plane-Token", s.sharedToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = fmt.Sprintf("node returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("node API key update failed: %s", message)
	}

	return nil
}

// fetchNodeStatus fetches node status via HTTP API
func (s *RealityKeyVerificationService) fetchNodeStatus(node models.Node) (*nodeStatusResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/status", node.BaseURL), nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var status nodeStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// GetKeyStatus returns the key verification status for a node
func (s *RealityKeyVerificationService) GetKeyStatus(nodeID uint) (map[string]interface{}, error) {
	var node models.Node
	if err := s.db.First(&node, "id = ?", nodeID).Error; err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	status := map[string]interface{}{
		"nodeId":                 node.ID,
		"nodeName":               node.Name,
		"realityPublicKey":       node.RealityPublicKey,
		"realityShortId":         node.RealityShortID,
		"realityPrivateKeyHash":  node.RealityPrivateKeyHash,
		"lastKeyVerificationAt":  node.LastKeyVerificationAt,
		"keyMismatchDetectedAt":  node.KeyMismatchDetectedAt,
		"keyMismatchAutoFixedAt": node.KeyMismatchAutoFixedAt,
	}

	// Try to fetch current node status for real-time comparison
	nodeStatus, err := s.fetchNodeStatus(node)
	if err == nil {
		status["nodePublicKey"] = nodeStatus.RealityPublicKey
		status["nodeShortId"] = nodeStatus.RealityShortID
		status["publicKeyMatch"] = (node.RealityPublicKey == nodeStatus.RealityPublicKey)
		status["shortIdMatch"] = (node.RealityShortID == nodeStatus.RealityShortID)
	}

	return status, nil
}

// ForceRotateKeys generates new keys and pushes them to the node (regardless of mismatch)
func (s *RealityKeyVerificationService) ForceRotateKeys(nodeID uint) error {
	var node models.Node
	if err := s.db.First(&node, "id = ?", nodeID).Error; err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	// Generate new keypair
	privateKey, publicKey, shortID, err := s.generateNewKeypair()
	if err != nil {
		return fmt.Errorf("failed to generate keypair: %w", err)
	}

	privateKeyHash := s.hashPrivateKey(privateKey)

	// Update panel DB
	if err := s.updatePanelDB(node.ID, publicKey, shortID, privateKeyHash); err != nil {
		return fmt.Errorf("failed to update panel DB: %w", err)
	}

	// Push keys to node via SSH
	if err := s.pushKeysToNodeViaSSH(node, privateKey, publicKey, shortID); err != nil {
		return fmt.Errorf("failed to push keys to node: %w", err)
	}

	// Wait for node to come back online and restart sing-box
	time.Sleep(5 * time.Second)

	// Verify rotation
	result, err := s.VerifySingleNode(nodeID)
	if err != nil {
		return fmt.Errorf("failed to verify rotation: %w", err)
	}

	if result.Status == "verified" {
		rotatedAt := time.Now()
		s.db.Model(&node).Updates(map[string]interface{}{
			"last_key_verification_at":   rotatedAt,
			"key_mismatch_detected_at":   nil,
			"key_mismatch_auto_fixed_at": &rotatedAt,
		})
		return nil
	}

	return errors.New("force rotation completed but verification failed")
}

// shellQuoteString safely quotes a string for shell usage
func shellQuoteString(s string) string {
	// Simple shell escaping - replace single quotes with '\''
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
