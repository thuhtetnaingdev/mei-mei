package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"node_backend/internal/config"
	"node_backend/internal/singbox"
)

type ApplyConfigRequest struct {
	NodeName             string   `json:"nodeName"`
	RealitySNIs          []string `json:"realitySnis"`
	Hysteria2Masquerades []string `json:"hysteria2Masquerades"`
	Users                []struct {
		UUID             string `json:"uuid"`
		Email            string `json:"email"`
		Enabled          bool   `json:"enabled"`
		BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
	} `json:"users"`
}

type ConfigService struct {
	cfg               config.Config
	mu                sync.RWMutex
	lastReload        time.Time
	lastError         string
	activeUsers       int
	activeListenPorts []int
	bandwidthTracker  *BandwidthTracker
}

func NewConfigService(cfg config.Config) *ConfigService {
	tracker := NewBandwidthTracker()
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
	users := make([]singbox.User, 0, len(req.Users))
	singboxUsers := make([]struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}, 0, len(req.Users))

	for _, user := range req.Users {
		singboxUser := singbox.User{
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
		s.cfg.NodeName,
		s.cfg.PublicHost,
		s.cfg.VLESSRealityPrivateKey,
		s.cfg.VLESSRealityServerName,
		s.cfg.VLESSRealityShortID,
		s.cfg.VLESSRealityHandshakeServer,
		s.cfg.VLESSRealityHandshakePort,
		s.cfg.TLSCertificatePath,
		s.cfg.TLSKeyPath,
		s.cfg.TLSServerName,
		req.RealitySNIs,
		req.Hysteria2Masquerades,
		users,
	)
	if err != nil {
		s.setError(err.Error())
		return err
	}

	if err := os.WriteFile(s.cfg.SingboxConfigPath, payload, 0o644); err != nil {
		s.setError(err.Error())
		return err
	}

	if err := s.ensureFirewallPorts(req); err != nil {
		log.Printf("[config-service] firewall sync warning: %v", err)
	}

	if err := s.reload(); err != nil {
		s.setError(err.Error())
		return err
	}

	layout := singbox.BuildTransportLayout(s.cfg.NodeName, s.cfg.PublicHost, req.RealitySNIs, req.Hysteria2Masquerades)
	activePorts := make([]int, 0, len(layout.VLESS)+len(layout.Hysteria2)+1)
	for _, plan := range layout.VLESS {
		activePorts = append(activePorts, plan.Port)
	}
	if layout.TUIC.Port > 0 {
		activePorts = append(activePorts, layout.TUIC.Port)
	}
	for _, plan := range layout.Hysteria2 {
		activePorts = append(activePorts, plan.Port)
	}

	s.mu.Lock()
	s.lastReload = time.Now()
	s.lastError = ""
	s.activeUsers = countEnabledUsers(req.Users)
	s.activeListenPorts = activePorts
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
		"nodeName":           s.cfg.NodeName,
		"publicHost":         s.cfg.PublicHost,
		"configPath":         s.cfg.SingboxConfigPath,
		"lastReload":         s.lastReload,
		"lastError":          s.lastError,
		"activeUsers":        s.activeUsers,
		"bandwidthUsedBytes": bandwidthUsedBytes,
		"status":             "ok",
		"bandwidthTracker":   trackerStatus,
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

func (s *ConfigService) ensureFirewallPorts(req ApplyConfigRequest) error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return nil
	}

	ports := make([]string, 0, 4)
	layout := singbox.BuildTransportLayout(s.cfg.NodeName, s.cfg.PublicHost, req.RealitySNIs, req.Hysteria2Masquerades)
	ports = appendPorts(ports, "tcp", layout.VLESS)
	if layout.TUIC.Port > 0 {
		ports = append(ports, fmt.Sprintf("%d/udp", layout.TUIC.Port))
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

func (s *ConfigService) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = message
}

func countEnabledUsers(users []struct {
	UUID             string `json:"uuid"`
	Email            string `json:"email"`
	Enabled          bool   `json:"enabled"`
	BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
}) int {
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
