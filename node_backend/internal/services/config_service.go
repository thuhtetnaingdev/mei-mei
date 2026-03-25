package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	expectedHash, err := canonicalApplyConfigHash(req)
	if err != nil {
		s.recordApplyVerification("error", err.Error(), "", 0, nil)
		return err
	}
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
	if err := s.ensureV2RayAPIEnvDefault(); err != nil {
		log.Printf("[config-service] env sync warning: %v", err)
	}
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

	bandwidthUsedBytes := readBandwidthUsageBytes()
	trackerStatus := s.bandwidthTracker.GetStatus()

	return map[string]interface{}{
		"nodeName":               s.cfg.NodeName,
		"publicHost":             s.cfg.PublicHost,
		"configPath":             s.cfg.SingboxConfigPath,
		"lastReload":             s.lastReload,
		"lastError":              s.lastError,
		"activeUsers":            s.activeUsers,
		"bandwidthUsedBytes":     bandwidthUsedBytes,
		"realityPublicKey":       s.cfg.VLESSRealityPublicKey,
		"realityShortId":         s.cfg.VLESSRealityShortID,
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

		if _, _, err := tracker.CollectUsage(); err != nil {
			log.Printf("[bandwidth-monitor] initial sample failed: %v", err)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("[bandwidth-monitor] stopped")
				return
			case <-ticker.C:
				s.mu.RLock()
				ports := append([]int(nil), s.activeListenPorts...)
				s.mu.RUnlock()
				tracker.UpdateConnectionCounts(ports...)
				delta, duration, err := tracker.CollectUsage()
				if err != nil {
					log.Printf("[bandwidth-monitor] sample failed: %v", err)
					continue
				}
				if delta > 0 {
					log.Printf("[bandwidth-monitor] sampled %d bytes over %s", delta, duration.Round(time.Second))
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
