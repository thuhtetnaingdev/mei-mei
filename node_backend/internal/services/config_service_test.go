package services

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"node_backend/internal/config"
	"node_backend/internal/singbox"
)

func TestApplyStoresVerificationMetadata(t *testing.T) {
	tempDir := t.TempDir()
	service := NewConfigService(config.Config{
		NodeName:                    "test-node",
		PublicHost:                  "node.example.com",
		SingboxConfigPath:           filepath.Join(tempDir, "sing-box.generated.json"),
		SingboxReloadCommand:        "true",
		NodeBinaryPath:              filepath.Join(tempDir, "node_backend"),
		TLSCertificatePath:          filepath.Join(tempDir, "tls.crt"),
		TLSKeyPath:                  filepath.Join(tempDir, "tls.key"),
		TLSServerName:               "node.example.com",
		VLESSRealityPrivateKey:      "private-key",
		VLESSRealityServerName:      "www.cloudflare.com",
		VLESSRealityShortID:         "abcd1234",
		VLESSRealityHandshakeServer: "www.cloudflare.com",
		VLESSRealityHandshakePort:   443,
	})

	req := ApplyConfigRequest{
		NodeName:             "test-node",
		RealitySNIs:          []string{"www.cloudflare.com"},
		Hysteria2Masquerades: []string{"https://www.cloudflare.com"},
		Users: []ApplyConfigUser{
			{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true},
			{ID: 2, UUID: "user-2", Email: "two@example.com", Enabled: false},
		},
	}

	if err := service.Apply(req); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	expectedHash, err := canonicalApplyConfigHash(req)
	if err != nil {
		t.Fatalf("canonicalApplyConfigHash() error = %v", err)
	}

	status := service.Status()
	if status["lastAppliedConfigHash"] != expectedHash {
		t.Fatalf("unexpected lastAppliedConfigHash: got %v want %v", status["lastAppliedConfigHash"], expectedHash)
	}
	if status["appliedUserCount"] != 1 {
		t.Fatalf("unexpected appliedUserCount: got %v want 1", status["appliedUserCount"])
	}
	if status["syncVerificationStatus"] != "applied" {
		t.Fatalf("unexpected syncVerificationStatus: got %v want applied", status["syncVerificationStatus"])
	}
	if status["syncVerificationAt"] == nil {
		t.Fatal("expected syncVerificationAt to be set")
	}
}

func TestApplyUsesRequestedNodeNameForGeneratedTransports(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "sing-box.generated.json")
	service := NewConfigService(config.Config{
		NodeName:                    "host-node-name",
		PublicHost:                  "node.example.com",
		SingboxConfigPath:           configPath,
		SingboxReloadCommand:        "true",
		NodeBinaryPath:              filepath.Join(tempDir, "node_backend"),
		TLSCertificatePath:          filepath.Join(tempDir, "tls.crt"),
		TLSKeyPath:                  filepath.Join(tempDir, "tls.key"),
		TLSServerName:               "node.example.com",
		VLESSRealityPrivateKey:      "private-key",
		VLESSRealityServerName:      "www.cloudflare.com",
		VLESSRealityShortID:         "abcd1234",
		VLESSRealityHandshakeServer: "www.cloudflare.com",
		VLESSRealityHandshakePort:   443,
	})

	req := ApplyConfigRequest{
		NodeName:             "panel-node-name",
		RealitySNIs:          []string{"www.cloudflare.com"},
		Hysteria2Masquerades: []string{"https://www.cloudflare.com"},
		Users: []ApplyConfigUser{
			{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true},
		},
	}

	if err := service.Apply(req); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}

	expectedLayout := singbox.BuildTransportLayout(req.NodeName, "node.example.com", req.RealitySNIs, req.Hysteria2Masquerades)
	unexpectedLayout := singbox.BuildTransportLayout("host-node-name", "node.example.com", req.RealitySNIs, req.Hysteria2Masquerades)
	expectedServerPassword := singbox.ShadowsocksServerPassword(req.NodeName, "node.example.com")
	unexpectedServerPassword := singbox.ShadowsocksServerPassword("host-node-name", "node.example.com")
	expectedUserPassword := singbox.ShadowsocksUserPassword(req.NodeName, "node.example.com", "user-1")
	unexpectedUserPassword := singbox.ShadowsocksUserPassword("host-node-name", "node.example.com", "user-1")
	configText := string(payload)

	if !strings.Contains(configText, `"listen_port": `+strconv.Itoa(expectedLayout.VLESS[0].Port)) {
		t.Fatalf("generated config did not include requested node-name port %d", expectedLayout.VLESS[0].Port)
	}
	if strings.Contains(configText, `"listen_port": `+strconv.Itoa(unexpectedLayout.VLESS[0].Port)) {
		t.Fatalf("generated config unexpectedly included host node-name port %d", unexpectedLayout.VLESS[0].Port)
	}
	if !strings.Contains(configText, expectedServerPassword) {
		t.Fatalf("generated config did not include requested node-name shadowsocks server password")
	}
	if !strings.Contains(configText, expectedUserPassword) {
		t.Fatalf("generated config did not include requested node-name shadowsocks user password")
	}
	if strings.Contains(configText, unexpectedServerPassword) {
		t.Fatalf("generated config unexpectedly included host node-name shadowsocks server password")
	}
	if strings.Contains(configText, unexpectedUserPassword) {
		t.Fatalf("generated config unexpectedly included host node-name shadowsocks user password")
	}
}
