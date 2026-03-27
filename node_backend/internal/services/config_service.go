package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"node_backend/internal/config"
	"node_backend/internal/singbox"
)

type ApplyConfigRequest struct {
	NodeName             string            `json:"nodeName"`
	RealitySNIs          []string          `json:"realitySnis"`
	Hysteria2Masquerades []string          `json:"hysteria2Masquerades"`
	Users                []ApplyConfigUser `json:"users"`
}

type ApplyConfigUser struct {
	ID               uint   `json:"id"`
	UUID             string `json:"uuid"`
	Email            string `json:"email"`
	Enabled          bool   `json:"enabled"`
	BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
}

type ConfigService struct {
	cfg                    config.Config
	mu                     sync.RWMutex
	lastReload             time.Time
	lastError              string
	activeUsers            int
	activeListenPorts      []int
	bandwidthTracker       *BandwidthTracker
	lastAppliedConfigHash  string
	appliedUserCount       int
	syncVerificationStatus string
	syncVerificationError  string
	syncVerificationAt     *time.Time

	// Debouncing for config changes
	configChangeMu   sync.Mutex
	debounceTimer    *time.Timer
	pendingRequest   *ApplyConfigRequest
	debounceInterval time.Duration // Default 5 seconds
}

func NewConfigService(cfg config.Config) *ConfigService {
	tracker := NewBandwidthTracker(cfg.SingboxV2RayAPIListen)
	service := &ConfigService{
		cfg:              cfg,
		bandwidthTracker: tracker,
	}
	service.restoreTrackedUsersFromConfig()
	return service
}

// GetBandwidthTracker returns the bandwidth tracker instance
func (s *ConfigService) GetBandwidthTracker() *BandwidthTracker {
	return s.bandwidthTracker
}

func (s *ConfigService) Apply(req ApplyConfigRequest) error {
	// Compute expected hash from request for change detection
	// This uses canonical JSON marshaling of the request for consistency
	expectedHash, err := canonicalApplyConfigHash(req)
	if err != nil {
		s.recordApplyVerification("error", err.Error(), "", 0, nil)
		return fmt.Errorf("failed to compute config hash: %w", err)
	}

	// Check if config is unchanged to skip redundant apply
	s.mu.RLock()
	if s.lastAppliedConfigHash == expectedHash {
		s.mu.RUnlock()
		log.Printf("[config-service] skipping redundant apply (config unchanged)")
		return nil
	}
	s.mu.RUnlock()

	effectiveNodeName := s.effectiveNodeName(req)

	users := make([]singbox.User, 0, len(req.Users))
	singboxUsers := make([]struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}, 0, len(req.Users))

	for _, user := range req.Users {
		singboxUser := singbox.User{
			ID:               user.ID,
			UUID:             user.UUID,
			Email:            user.Email,
			Enabled:          user.Enabled,
			BandwidthLimitGB: user.BandwidthLimitGB,
		}
		users = append(users, singboxUser)

		singboxUsers = append(singboxUsers, struct {
			UUID             string
			Email            string
			Enabled          bool
			BandwidthLimitGB int64
		}{
			UUID:             user.UUID,
			Email:            user.Email,
			Enabled:          user.Enabled,
			BandwidthLimitGB: user.BandwidthLimitGB,
		})
	}

	payload, err := singbox.Generate(
		effectiveNodeName,
		s.cfg.PublicHost,
		s.cfg.VLESSRealityPrivateKey,
		s.cfg.VLESSRealityServerName,
		s.cfg.VLESSRealityShortID,
		s.cfg.VLESSRealityHandshakeServer,
		s.cfg.VLESSRealityHandshakePort,
		s.cfg.TLSCertificatePath,
		s.cfg.TLSKeyPath,
		s.cfg.TLSServerName,
		s.cfg.SingboxV2RayAPIListen,
		req.RealitySNIs,
		req.Hysteria2Masquerades,
		users,
		s.cfg.DNSServers,
		s.cfg.DNSStrategy,
		s.cfg.DNSDisableCache,
		s.cfg.DNSDisableExpire,
		s.cfg.DNSIndependentCache,
		s.cfg.DNSReverseMapping,
	)
	if err != nil {
		s.setError(err.Error())
		s.recordApplyVerification("error", err.Error(), expectedHash, countEnabledUsers(req.Users), nil)
		return err
	}

	if err := os.WriteFile(s.cfg.SingboxConfigPath, payload, 0o644); err != nil {
		s.setError(err.Error())
		s.recordApplyVerification("error", err.Error(), expectedHash, countEnabledUsers(req.Users), nil)
		return err
	}
	// Note: .env sync removed - this should only happen once at install time, not per-config-change
	if err := s.validateSingboxConfig(); err != nil {
		if fallbackPayload, fallbackErr := s.buildV2RayAPIFallbackPayload(req, users); fallbackErr == nil && fallbackPayload != nil {
			log.Printf("[config-service] sing-box validation failed with v2ray api enabled, retrying without v2ray api: %v", err)
			if writeErr := os.WriteFile(s.cfg.SingboxConfigPath, fallbackPayload, 0o644); writeErr != nil {
				s.setError(writeErr.Error())
				s.recordApplyVerification("error", writeErr.Error(), expectedHash, countEnabledUsers(req.Users), nil)
				return writeErr
			}
			if validateErr := s.validateSingboxConfig(); validateErr != nil {
				s.setError(validateErr.Error())
				s.recordApplyVerification("error", validateErr.Error(), expectedHash, countEnabledUsers(req.Users), nil)
				return validateErr
			}
			s.bandwidthTracker.DisableV2RayAPI()
		} else {
			s.setError(err.Error())
			s.recordApplyVerification("error", err.Error(), expectedHash, countEnabledUsers(req.Users), nil)
			return err
		}
	}

	if err := s.ensureFirewallPorts(req); err != nil {
		log.Printf("[config-service] firewall sync warning: %v", err)
	}

	if err := s.reload(); err != nil {
		s.setError(err.Error())
		s.recordApplyVerification("error", err.Error(), expectedHash, countEnabledUsers(req.Users), nil)
		return err
	}

	layout := singbox.BuildTransportLayout(effectiveNodeName, s.cfg.PublicHost, req.RealitySNIs, req.Hysteria2Masquerades)
	shadowsocksPlans := singbox.BuildShadowsocksInboundPlans(effectiveNodeName, s.cfg.PublicHost, users)
	activePorts := make([]int, 0, len(layout.VLESS)+len(layout.Hysteria2)+len(shadowsocksPlans)+1)
	for _, plan := range layout.VLESS {
		activePorts = append(activePorts, plan.Port)
	}
	if layout.TUIC.Port > 0 {
		activePorts = append(activePorts, layout.TUIC.Port)
	}
	for _, plan := range shadowsocksPlans {
		activePorts = append(activePorts, plan.Port)
	}
	for _, plan := range layout.Hysteria2 {
		activePorts = append(activePorts, plan.Port)
	}

	appliedAt := time.Now()
	s.mu.Lock()
	s.lastReload = appliedAt
	s.lastError = ""
	s.activeUsers = countEnabledUsers(req.Users)
	s.activeListenPorts = activePorts
	s.lastAppliedConfigHash = expectedHash
	s.appliedUserCount = countEnabledUsers(req.Users)
	s.syncVerificationStatus = "applied"
	s.syncVerificationError = ""
	s.syncVerificationAt = &appliedAt
	s.mu.Unlock()

	// Update bandwidth tracker with active users
	s.bandwidthTracker.UpdateActiveUsers(singboxUsers)

	return nil
}

func (s *ConfigService) Status() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trackerStatus := s.bandwidthTracker.GetStatus()

	// Read current keys from .env file (not in-memory config) to ensure
	// we return the actual persisted state, even after external updates
	_, publicKey, shortID, _ := s.GetRealityKeysFromEnvFile()

	return map[string]interface{}{
		"nodeName":               s.cfg.NodeName,
		"publicHost":             s.cfg.PublicHost,
		"configPath":             s.cfg.SingboxConfigPath,
		"lastReload":             s.lastReload,
		"lastError":              s.lastError,
		"activeUsers":            s.activeUsers,
		"realityPublicKey":       publicKey,
		"realityShortId":         shortID,
		"realityServerName":      s.cfg.VLESSRealityServerName,
		"status":                 "ok",
		"bandwidthTracker":       trackerStatus,
		"lastAppliedConfigHash":  s.lastAppliedConfigHash,
		"appliedUserCount":       s.appliedUserCount,
		"syncVerificationStatus": valueOrDefault(s.syncVerificationStatus, "unknown"),
		"syncVerificationError":  s.syncVerificationError,
		"syncVerificationAt":     s.syncVerificationAt,
	}
}

func (s *ConfigService) StartBandwidthMonitoring(ctx context.Context, interval time.Duration) {
	go func() {
		tracker := s.GetBandwidthTracker()

		// Connect to sing-box stats API
		if err := tracker.ConnectStatsClient(); err != nil {
			log.Printf("[bandwidth-monitor] stats API connection failed: %v", err)
		} else {
			log.Printf("[bandwidth-monitor] connected to sing-box stats API")
		}

		// Initial collection to establish baseline
		if _, _, err := tracker.CollectUsage(); err != nil {
			log.Printf("[bandwidth-monitor] initial stats collection failed: %v", err)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("[bandwidth-monitor] stopped")
				return
			case <-ticker.C:
				delta, _, err := tracker.CollectUsage()
				if err != nil {
					log.Printf("[bandwidth-monitor] stats collection failed: %v", err)
					continue
				}
				if delta > 0 {
					log.Printf("[bandwidth-monitor] collected %d bytes from stats API", delta)
				}
			}
		}
	}()
}

func (s *ConfigService) reload() error {
	cmd := exec.Command("sh", "-c", s.cfg.SingboxReloadCommand)
	return cmd.Run()
}

func (s *ConfigService) validateSingboxConfig() error {
	singboxPath, err := exec.LookPath("sing-box")
	if err != nil {
		return nil
	}

	cmd := exec.Command(singboxPath, "check", "-c", s.cfg.SingboxConfigPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("sing-box config validation failed: %s", message)
	}
	return nil
}

func (s *ConfigService) buildV2RayAPIFallbackPayload(req ApplyConfigRequest, users []singbox.User) ([]byte, error) {
	if strings.TrimSpace(s.cfg.SingboxV2RayAPIListen) == "" {
		return nil, nil
	}

	return singbox.Generate(
		s.effectiveNodeName(req),
		s.cfg.PublicHost,
		s.cfg.VLESSRealityPrivateKey,
		s.cfg.VLESSRealityServerName,
		s.cfg.VLESSRealityShortID,
		s.cfg.VLESSRealityHandshakeServer,
		s.cfg.VLESSRealityHandshakePort,
		s.cfg.TLSCertificatePath,
		s.cfg.TLSKeyPath,
		s.cfg.TLSServerName,
		"",
		req.RealitySNIs,
		req.Hysteria2Masquerades,
		users,
		s.cfg.DNSServers,
		s.cfg.DNSStrategy,
		s.cfg.DNSDisableCache,
		s.cfg.DNSDisableExpire,
		s.cfg.DNSIndependentCache,
		s.cfg.DNSReverseMapping,
	)
}

func (s *ConfigService) ensureFirewallPorts(req ApplyConfigRequest) error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return nil
	}

	ports := make([]string, 0, 4)
	effectiveNodeName := s.effectiveNodeName(req)
	layout := singbox.BuildTransportLayout(effectiveNodeName, s.cfg.PublicHost, req.RealitySNIs, req.Hysteria2Masquerades)
	shadowsocksPlans := singbox.BuildShadowsocksInboundPlans(effectiveNodeName, s.cfg.PublicHost, usersFromApplyRequest(req.Users))
	ports = appendPorts(ports, "tcp", layout.VLESS)
	if layout.TUIC.Port > 0 {
		ports = append(ports, fmt.Sprintf("%d/udp", layout.TUIC.Port))
	}
	for _, plan := range shadowsocksPlans {
		ports = append(ports, fmt.Sprintf("%d/tcp", plan.Port))
	}
	ports = appendHy2Ports(ports, layout.Hysteria2)

	for _, port := range ports {
		cmd := exec.Command("ufw", "allow", port)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ufw allow %s failed: %v (%s)", port, err, strings.TrimSpace(string(output)))
		}
	}

	return nil
}

func (s *ConfigService) effectiveNodeName(req ApplyConfigRequest) string {
	if strings.TrimSpace(req.NodeName) != "" {
		return strings.TrimSpace(req.NodeName)
	}
	return s.cfg.NodeName
}

func (s *ConfigService) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = message
}

func (s *ConfigService) ensureV2RayAPIEnvDefault() error {
	listenAddress := strings.TrimSpace(s.cfg.SingboxV2RayAPIListen)
	if listenAddress == "" || s.cfg.NodeBinaryPath == "" {
		return nil
	}

	envPath := filepath.Join(filepath.Dir(s.cfg.NodeBinaryPath), ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	found := false
	changed := false
	for index, line := range lines {
		if !strings.HasPrefix(line, "SINGBOX_V2RAY_API_LISTEN=") {
			continue
		}

		found = true
		if strings.TrimSpace(strings.TrimPrefix(line, "SINGBOX_V2RAY_API_LISTEN=")) != "" {
			return nil
		}

		lines[index] = "SINGBOX_V2RAY_API_LISTEN=" + listenAddress
		changed = true
		break
	}

	if !found {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, "SINGBOX_V2RAY_API_LISTEN="+listenAddress)
		changed = true
	}

	if !changed {
		return nil
	}

	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return os.WriteFile(envPath, []byte(updated), 0o644)
}

func countEnabledUsers(users []ApplyConfigUser) int {
	total := 0
	for _, user := range users {
		if user.Enabled {
			total++
		}
	}
	return total
}

func appendPorts[T interface{ GetPort() int }](ports []string, protocol string, plans []T) []string {
	for _, plan := range plans {
		ports = append(ports, strconv.Itoa(plan.GetPort())+"/"+protocol)
	}
	return ports
}

func appendHy2Ports(ports []string, plans []singbox.Hysteria2InboundPlan) []string {
	for _, plan := range plans {
		ports = append(ports, strconv.Itoa(plan.GetPort())+"/udp")
	}
	return ports
}

func usersFromApplyRequest(users []ApplyConfigUser) []singbox.User {
	result := make([]singbox.User, 0, len(users))
	for _, user := range users {
		result = append(result, singbox.User{
			ID:               user.ID,
			UUID:             user.UUID,
			Email:            user.Email,
			Enabled:          user.Enabled,
			BandwidthLimitGB: user.BandwidthLimitGB,
		})
	}
	return result
}

func (s *ConfigService) restoreTrackedUsersFromConfig() {
	payload, err := os.ReadFile(s.cfg.SingboxConfigPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[config-service] failed to read existing sing-box config: %v", err)
		}
		return
	}

	type inboundUser struct {
		UUID     string `json:"uuid"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	type inbound struct {
		Type  string        `json:"type"`
		Users []inboundUser `json:"users"`
	}
	type generatedConfig struct {
		Inbounds []inbound `json:"inbounds"`
	}

	var cfg generatedConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		log.Printf("[config-service] failed to parse existing sing-box config: %v", err)
		return
	}

	seen := make(map[string]bool)
	trackedUsers := make([]struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}, 0)

	for _, inbound := range cfg.Inbounds {
		for _, user := range inbound.Users {
			uuid := user.UUID
			if uuid == "" {
				uuid = user.Password
			}
			if uuid == "" || seen[uuid] {
				continue
			}

			seen[uuid] = true
			trackedUsers = append(trackedUsers, struct {
				UUID             string
				Email            string
				Enabled          bool
				BandwidthLimitGB int64
			}{
				UUID:             uuid,
				Email:            user.Name,
				Enabled:          true,
				BandwidthLimitGB: 0,
			})
		}
	}

	if len(trackedUsers) == 0 {
		return
	}

	s.bandwidthTracker.UpdateActiveUsers(trackedUsers)

	s.mu.Lock()
	s.activeUsers = len(trackedUsers)
	s.mu.Unlock()

	log.Printf("[config-service] restored %d tracked users from existing sing-box config", len(trackedUsers))
}

func (s *ConfigService) recordApplyVerification(status, message, configHash string, appliedUserCount int, verifiedAt *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAppliedConfigHash = configHash
	s.appliedUserCount = appliedUserCount
	s.syncVerificationStatus = status
	s.syncVerificationError = message
	s.syncVerificationAt = verifiedAt
}

// computeConfigHash computes hash of a config request without applying
// This generates the sing-box config payload and computes its SHA256 hash
func (s *ConfigService) computeConfigHash(req ApplyConfigRequest) (string, error) {
	effectiveNodeName := s.effectiveNodeName(req)

	users := make([]singbox.User, 0, len(req.Users))
	for _, user := range req.Users {
		users = append(users, singbox.User{
			ID:               user.ID,
			UUID:             user.UUID,
			Email:            user.Email,
			Enabled:          user.Enabled,
			BandwidthLimitGB: user.BandwidthLimitGB,
		})
	}

	payload, err := singbox.Generate(
		effectiveNodeName,
		s.cfg.PublicHost,
		s.cfg.VLESSRealityPrivateKey,
		s.cfg.VLESSRealityServerName,
		s.cfg.VLESSRealityShortID,
		s.cfg.VLESSRealityHandshakeServer,
		s.cfg.VLESSRealityHandshakePort,
		s.cfg.TLSCertificatePath,
		s.cfg.TLSKeyPath,
		s.cfg.TLSServerName,
		s.cfg.SingboxV2RayAPIListen,
		req.RealitySNIs,
		req.Hysteria2Masquerades,
		users,
		s.cfg.DNSServers,
		s.cfg.DNSStrategy,
		s.cfg.DNSDisableCache,
		s.cfg.DNSDisableExpire,
		s.cfg.DNSIndependentCache,
		s.cfg.DNSReverseMapping,
	)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}

// ApplyDebounced schedules a config apply with debouncing to coalesce rapid changes
// Multiple calls within debounceInterval will be coalesced into a single apply
// This reduces CPU, I/O, and sing-box reloads when the panel sends rapid config updates
func (s *ConfigService) ApplyDebounced(req ApplyConfigRequest) error {
	s.configChangeMu.Lock()
	defer s.configChangeMu.Unlock()

	// Store the latest request
	s.pendingRequest = &req

	// Cancel existing timer if any
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}

	// Start new timer
	s.debounceTimer = time.AfterFunc(s.debounceInterval, func() {
		s.configChangeMu.Lock()
		if s.pendingRequest == nil {
			s.configChangeMu.Unlock()
			return
		}
		pendingReq := *s.pendingRequest
		s.pendingRequest = nil
		s.configChangeMu.Unlock()

		// Apply the debounced request
		log.Printf("[config-service] applying debounced config change")
		if err := s.Apply(pendingReq); err != nil {
			log.Printf("[config-service] debounced apply failed: %v", err)
		}
	})

	log.Printf("[config-service] config change debounced (will apply in %v)", s.debounceInterval)
	return nil
}

func canonicalApplyConfigHash(req ApplyConfigRequest) (string, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// UpdateRealityKeys updates the REALITY keys and reloads sing-box
// Validates key formats, updates in-memory config, updates .env file, and reloads sing-box
func (s *ConfigService) UpdateRealityKeys(privateKey, publicKey, shortID string) error {
	// Validate key formats (accept both standard base64 and base64url)
	if !isValidBase64(privateKey) {
		return errors.New("invalid private key format (must be base64 encoded)")
	}
	if !isValidBase64(publicKey) {
		return errors.New("invalid public key format (must be base64 encoded)")
	}
	if !isValidHex(shortID) || len(shortID) != 8 {
		return errors.New("invalid short ID format (expected 8 hex characters)")
	}

	// Update in-memory config
	s.cfg.VLESSRealityPrivateKey = privateKey
	s.cfg.VLESSRealityPublicKey = publicKey
	s.cfg.VLESSRealityShortID = shortID

	// Update .env file
	envPath := s.getEnvFilePath()
	if err := updateEnvFile(envPath, map[string]string{
		"VLESS_REALITY_PRIVATE_KEY": privateKey,
		"VLESS_REALITY_PUBLIC_KEY":  publicKey,
		"VLESS_REALITY_SHORT_ID":    shortID,
	}); err != nil {
		return fmt.Errorf("failed to update .env file: %w", err)
	}

	if err := updateSingboxRealityKeys(s.cfg.SingboxConfigPath, privateKey, shortID); err != nil {
		return fmt.Errorf("failed to update sing-box config: %w", err)
	}

	// Reload sing-box
	if err := s.reloadSingbox(); err != nil {
		return fmt.Errorf("failed to reload sing-box: %w", err)
	}

	log.Printf("[config-service] reality keys updated successfully")
	return nil
}

// getEnvFilePath returns the path to the .env file
// Defaults to /opt/meimei-node/.env if node binary path is set
func (s *ConfigService) getEnvFilePath() string {
	if s.cfg.NodeBinaryPath != "" {
		return filepath.Join(filepath.Dir(s.cfg.NodeBinaryPath), ".env")
	}
	return "/opt/meimei-node/.env"
}

// reloadSingbox reloads the sing-box service
func (s *ConfigService) reloadSingbox() error {
	cmd := exec.Command("sh", "-c", s.cfg.SingboxReloadCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload command failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	log.Printf("[config-service] sing-box reloaded successfully")
	return nil
}

// updateEnvFile updates environment variables in the .env file
func updateEnvFile(path string, updates map[string]string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new file with the updates
			lines := make([]string, 0, len(updates))
			for key, value := range updates {
				lines = append(lines, key+"="+value)
			}
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	updated := make(map[string]bool)

	for i, line := range lines {
		for key, value := range updates {
			if strings.HasPrefix(line, key+"=") {
				lines[i] = key + "=" + value
				updated[key] = true
			}
		}
	}

	// Add missing keys
	for key, value := range updates {
		if !updated[key] {
			lines = append(lines, key+"="+value)
		}
	}

	// Ensure file ends with newline
	result := strings.Join(lines, "\n")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return os.WriteFile(path, []byte(result), 0644)
}

func updateSingboxRealityKeys(path, privateKey, shortID string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return err
	}

	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return errors.New("sing-box config missing inbounds array")
	}

	updated := 0
	for _, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]interface{})
		if !ok {
			continue
		}
		if inbound["type"] != "vless" {
			continue
		}

		tlsConfig, ok := inbound["tls"].(map[string]interface{})
		if !ok {
			continue
		}
		realityConfig, ok := tlsConfig["reality"].(map[string]interface{})
		if !ok {
			continue
		}

		realityConfig["private_key"] = privateKey
		realityConfig["short_id"] = []string{shortID}
		updated++
	}

	if updated == 0 {
		return errors.New("no vless reality inbounds found in sing-box config")
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}

// GetRealityKeysFromEnvFile reads the current REALITY keys from the .env file
// This ensures we always return the actual persisted state, even if the
// in-memory config is stale (e.g., after external updates)
func (s *ConfigService) GetRealityKeysFromEnvFile() (privateKey, publicKey, shortID string, err error) {
	envPath := s.getEnvFilePath()
	content, err := os.ReadFile(envPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read .env file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "VLESS_REALITY_PRIVATE_KEY=") {
			privateKey = strings.TrimPrefix(line, "VLESS_REALITY_PRIVATE_KEY=")
		} else if strings.HasPrefix(line, "VLESS_REALITY_PUBLIC_KEY=") {
			publicKey = strings.TrimPrefix(line, "VLESS_REALITY_PUBLIC_KEY=")
		} else if strings.HasPrefix(line, "VLESS_REALITY_SHORT_ID=") {
			shortID = strings.TrimPrefix(line, "VLESS_REALITY_SHORT_ID=")
		}
	}
	return privateKey, publicKey, shortID, nil
}

// isValidBase64 validates that a string is valid standard base64 encoding
// Accepts both StdEncoding (with +/) and RawURLEncoding (with -_)
func isValidBase64(s string) bool {
	if s == "" {
		return false
	}
	// Try standard base64 first (what sing-box expects)
	_, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return true
	}
	// Also accept base64url for backwards compatibility
	_, err = base64.RawURLEncoding.DecodeString(s)
	return err == nil
}

// isValidHex validates that a string contains only hexadecimal characters
func isValidHex(s string) bool {
	if s == "" {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
