package services

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"node_backend/internal/config"
	"node_backend/internal/singbox"
)

type ApplyConfigRequest struct {
	NodeName string `json:"nodeName"`
	Users    []struct {
		UUID             string `json:"uuid"`
		Email            string `json:"email"`
		Enabled          bool   `json:"enabled"`
		BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
	} `json:"users"`
}

type ConfigService struct {
	cfg                config.Config
	mu                 sync.RWMutex
	lastReload         time.Time
	lastError          string
	activeUsers        int
	bandwidthTracker   *BandwidthTracker
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
		s.cfg.VLESSPort,
		s.cfg.TUICPort,
		s.cfg.Hysteria2Port,
		s.cfg.VLESSRealityPrivateKey,
		s.cfg.VLESSRealityServerName,
		s.cfg.VLESSRealityShortID,
		s.cfg.VLESSRealityHandshakeServer,
		s.cfg.VLESSRealityHandshakePort,
		s.cfg.TLSCertificatePath,
		s.cfg.TLSKeyPath,
		s.cfg.TLSServerName,
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

	if err := s.reload(); err != nil {
		s.setError(err.Error())
		return err
	}

	s.mu.Lock()
	s.lastReload = time.Now()
	s.lastError = ""
	s.activeUsers = countEnabledUsers(req.Users)
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
		"vlessPort":          s.cfg.VLESSPort,
		"tuicPort":           s.cfg.TUICPort,
		"hysteria2Port":      s.cfg.Hysteria2Port,
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
				tracker.UpdateConnectionCounts(s.cfg.VLESSPort, s.cfg.TUICPort, s.cfg.Hysteria2Port)
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
