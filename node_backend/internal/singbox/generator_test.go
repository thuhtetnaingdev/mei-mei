package singbox

import (
	"encoding/json"
	"testing"
)

func TestGenerateBuildsSingleUserShadowsocksInbounds(t *testing.T) {
	payload, err := Generate(
		"webdock",
		"92.113.148.36",
		"private-key",
		"www.cloudflare.com",
		"abcd1234",
		"www.cloudflare.com",
		443,
		"/tmp/tls.crt",
		"/tmp/tls.key",
		"92.113.148.36",
		[]string{"cdn.akamai.steamstatic.com"},
		[]string{"https://www.cloudflare.com"},
		[]User{
			{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true},
			{ID: 2, UUID: "user-2", Email: "two@example.com", Enabled: true},
			{ID: 3, UUID: "user-3", Email: "three@example.com", Enabled: false},
		},
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	var cfg struct {
		Inbounds []map[string]interface{} `json:"inbounds"`
	}
	if err := json.Unmarshal(payload, &cfg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	var shadowsocksInbounds []map[string]interface{}
	for _, inbound := range cfg.Inbounds {
		if inbound["type"] == "shadowsocks" {
			shadowsocksInbounds = append(shadowsocksInbounds, inbound)
		}
	}

	if len(shadowsocksInbounds) != 2 {
		t.Fatalf("expected 2 shadowsocks inbounds, got %d", len(shadowsocksInbounds))
	}

	expectedPorts := map[float64]string{
		10000: ShadowsocksUserPassword("webdock", "92.113.148.36", "user-1"),
		10001: ShadowsocksUserPassword("webdock", "92.113.148.36", "user-2"),
	}

	for _, inbound := range shadowsocksInbounds {
		port, ok := inbound["listen_port"].(float64)
		if !ok {
			t.Fatalf("listen_port missing or invalid: %#v", inbound["listen_port"])
		}

		password, ok := inbound["password"].(string)
		if !ok {
			t.Fatalf("password missing or invalid: %#v", inbound["password"])
		}

		expectedPassword, exists := expectedPorts[port]
		if !exists {
			t.Fatalf("unexpected shadowsocks port %v", port)
		}
		if password != expectedPassword {
			t.Fatalf("unexpected password for port %v: got %q want %q", port, password, expectedPassword)
		}
		if _, hasUsers := inbound["users"]; hasUsers {
			t.Fatalf("shadowsocks inbound should not include users: %#v", inbound)
		}
	}
}
