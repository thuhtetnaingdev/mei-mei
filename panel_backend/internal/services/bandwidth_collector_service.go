package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"
	"panel_backend/internal/models"
)

// NodeBandwidthUsage represents bandwidth usage response from a node
type NodeBandwidthUsage struct {
	NodeName   string `json:"nodeName"`
	ReportTime string `json:"reportTime"`
	TotalUsers int    `json:"totalUsers"`
	Users      []struct {
		UUID  string `json:"uuid"`
		Bytes int64  `json:"bytes"`
	} `json:"users"`
}

// BandwidthCollectorService polls nodes for bandwidth usage data
type BandwidthCollectorService struct {
	db              *gorm.DB
	httpClient      *http.Client
	collectInterval time.Duration
	requestTimeout  time.Duration
	nodeSharedToken string
	userService     *UserService
	nodeService     *NodeService

	mu              sync.RWMutex
	lastCollectTime time.Time
	lastCollectErr  error
	consecutiveErrs int
	isRunning       bool
}

// BandwidthCollectorConfig holds configuration for the collector
type BandwidthCollectorConfig struct {
	DB              *gorm.DB
	NodeSharedToken string
	CollectInterval time.Duration
	RequestTimeout  time.Duration
	UserService     *UserService
	NodeService     *NodeService
}

// NewBandwidthCollectorService creates a new bandwidth collector service
func NewBandwidthCollectorService(cfg BandwidthCollectorConfig) *BandwidthCollectorService {
	return &BandwidthCollectorService{
		db:              cfg.DB,
		nodeSharedToken: cfg.NodeSharedToken,
		collectInterval: cfg.CollectInterval,
		requestTimeout:  cfg.RequestTimeout,
		userService:     cfg.UserService,
		nodeService:     cfg.NodeService,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

// Start begins the periodic collection loop
func (s *BandwidthCollectorService) Start(ctx context.Context) {
	s.mu.Lock()
	s.isRunning = true
	s.mu.Unlock()

	go func() {
		log.Printf("[bandwidth-collector] started with interval %v", s.collectInterval)

		// Initial delay to allow system to stabilize
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return
		}

		ticker := time.NewTicker(s.collectInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("[bandwidth-collector] context cancelled, stopping")
				s.mu.Lock()
				s.isRunning = false
				s.mu.Unlock()
				return
			case <-ticker.C:
				s.collectFromAllNodes()
			}
		}
	}()
}

// collectFromAllNodes fetches bandwidth usage from all registered nodes
func (s *BandwidthCollectorService) collectFromAllNodes() {
	log.Printf("[bandwidth-collector] starting bandwidth collection from all nodes")

	// Get all registered nodes from DB
	var nodes []models.Node
	if err := s.db.Where("health_status != ?", "offline").Find(&nodes).Error; err != nil {
		log.Printf("[bandwidth-collector] failed to get nodes: %v", err)
		s.recordError(err)
		return
	}

	if len(nodes) == 0 {
		log.Printf("[bandwidth-collector] no active nodes to poll")
		s.recordSuccess()
		return
	}

	log.Printf("[bandwidth-collector] polling %d nodes", len(nodes))

	// Poll each node concurrently
	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		go func(node models.Node) {
			defer wg.Done()
			if err := s.collectFromNode(&node); err != nil {
				log.Printf("[bandwidth-collector] failed to collect from node %s: %v", node.Name, err)
			}
		}(nodes[i])
	}

	wg.Wait()
	s.recordSuccess()
	log.Printf("[bandwidth-collector] completed bandwidth collection")
}

// collectFromNode fetches bandwidth usage from a single node
func (s *BandwidthCollectorService) collectFromNode(node *models.Node) error {
	// Build request URL
	url := fmt.Sprintf("%s/bandwidth-usage", node.BaseURL)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set authentication headers
	req.Header.Set("Authorization", "Bearer "+node.ProtocolToken)
	req.Header.Set("X-Control-Plane-Token", s.nodeSharedToken)

	// Execute request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.markNodeOffline(node.Name)
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		log.Printf("[bandwidth-collector] authentication failed for node %s", node.Name)
		s.markNodeOffline(node.Name)
		return fmt.Errorf("authentication failed")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.markNodeOffline(node.Name)
		return fmt.Errorf("node returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var usage NodeBandwidthUsage
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Process user usage data
	totalBytes := int64(0)
	disabledAnyUser := false
	for _, user := range usage.Users {
		if user.Bytes > 0 {
			disabled, err := s.updateUserUsage(user.UUID, user.Bytes)
			if err != nil {
				log.Printf("[bandwidth-collector] failed to update user %s usage: %v", user.UUID, err)
			}
			if disabled {
				disabledAnyUser = true
			}
			totalBytes += user.Bytes
		}
	}

	// Update node's total bandwidth and status
	if err := s.updateNodeUsage(node.Name, totalBytes); err != nil {
		log.Printf("[bandwidth-collector] failed to update node %s usage: %v", node.Name, err)
	}

	// Mark node as online
	s.markNodeOnline(node.Name)

	log.Printf("[bandwidth-collector] collected %d bytes from %d users on node %s",
		totalBytes, len(usage.Users), node.Name)

	if disabledAnyUser {
		if err := s.syncNodesAfterLimitEnforcement(); err != nil {
			log.Printf("[bandwidth-collector] failed to sync nodes after limit enforcement: %v", err)
		}
	}

	return nil
}

// updateUserUsage updates the bandwidth usage for a specific user
func (s *BandwidthCollectorService) updateUserUsage(uuid string, bytes int64) (bool, error) {
	if uuid == "" || bytes <= 0 {
		return false, nil
	}

	if s.userService != nil {
		disabled, err := s.userService.AddUsageAndDisableIfLimitReached(uuid, bytes)
		if err != nil {
			return false, fmt.Errorf("database update failed: %w", err)
		}
		return disabled, nil
	}

	result := s.db.Model(&models.User{}).
		Where("uuid = ?", uuid).
		UpdateColumn("bandwidth_used_bytes", gorm.Expr("bandwidth_used_bytes + ?", bytes))
	if result.Error != nil {
		return false, fmt.Errorf("database update failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		log.Printf("[bandwidth-collector] user %s not found, skipping usage update", uuid)
		return false, nil
	}

	return false, nil
}

// updateNodeUsage updates the total bandwidth usage for a node
func (s *BandwidthCollectorService) updateNodeUsage(nodeName string, bytes int64) error {
	if nodeName == "" || bytes <= 0 {
		return nil
	}

	result := s.db.Model(&models.Node{}).
		Where("name = ?", nodeName).
		UpdateColumn("bandwidth_used_bytes", gorm.Expr("bandwidth_used_bytes + ?", bytes))

	if result.Error != nil {
		return fmt.Errorf("database update failed: %w", result.Error)
	}

	return nil
}

// markNodeOnline marks a node as online with current timestamp
func (s *BandwidthCollectorService) markNodeOnline(nodeName string) {
	now := time.Now()
	s.db.Model(&models.Node{}).
		Where("name = ?", nodeName).
		Updates(map[string]interface{}{
			"health_status":  "online",
			"last_heartbeat": &now,
			"last_sync_at":   &now,
		})
}

// markNodeOffline marks a node as offline
func (s *BandwidthCollectorService) markNodeOffline(nodeName string) {
	s.db.Model(&models.Node{}).
		Where("name = ?", nodeName).
		Update("health_status", "offline")
}

// recordSuccess records a successful collection cycle
func (s *BandwidthCollectorService) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCollectTime = time.Now()
	s.lastCollectErr = nil
	s.consecutiveErrs = 0
}

// recordError records a failed collection cycle
func (s *BandwidthCollectorService) recordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCollectTime = time.Now()
	s.lastCollectErr = err
	s.consecutiveErrs++
}

// ForceCollect triggers an immediate collection (useful for manual trigger or testing)
func (s *BandwidthCollectorService) ForceCollect() error {
	log.Printf("[bandwidth-collector] triggering forced collection")
	s.collectFromAllNodes()
	return nil
}

func (s *BandwidthCollectorService) syncNodesAfterLimitEnforcement() error {
	if s.userService == nil || s.nodeService == nil {
		return nil
	}

	activeUsers, err := s.userService.ActiveUsers()
	if err != nil {
		return err
	}

	_, err = s.nodeService.SyncAllUsers(activeUsers)
	return err
}

// GetStatus returns the current status of the collector
func (s *BandwidthCollectorService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"isRunning":       s.isRunning,
		"collectInterval": s.collectInterval.String(),
		"lastCollectTime": s.lastCollectTime,
		"lastCollectErr":  s.lastCollectErr,
		"consecutiveErrs": s.consecutiveErrs,
	}
}

// Stop stops the collector (sets isRunning to false)
func (s *BandwidthCollectorService) Stop() {
	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()
	log.Printf("[bandwidth-collector] stopped")
}
