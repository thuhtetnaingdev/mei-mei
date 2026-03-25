package singbox

import (
	"encoding/json"
	"testing"
)

func TestGenerateBuildsMultiUserShadowsocksInbound(t *testing.T) {
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
		"",
		[]string{"cdn.akamai.steamstatic.com"},
		[]string{"https://www.cloudflare.com"},
		[]User{
			{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true},
			{ID: 2, UUID: "user-2", Email: "two@example.com", Enabled: true},
			{ID: 3, UUID: "user-3", Email: "three@example.com", Enabled: false},
		},
		"8.8.8.8,1.1.1.1",
		"prefer_ipv4",
		false,
		false,
		false,
		false,
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

	if len(shadowsocksInbounds) != 1 {
		t.Fatalf("expected 1 shadowsocks inbound, got %d", len(shadowsocksInbounds))
	}

	inbound := shadowsocksInbounds[0]
	expectedPort := float64(ShadowsocksPort("webdock", "92.113.148.36"))
	if inbound["listen_port"] != expectedPort {
		t.Fatalf("unexpected shadowsocks port: got %v want %v", inbound["listen_port"], expectedPort)
	}
	if inbound["password"] != ShadowsocksServerPassword("webdock", "92.113.148.36") {
		t.Fatalf("unexpected shadowsocks server password: %#v", inbound["password"])
	}

	users, ok := inbound["users"].([]interface{})
	if !ok {
		t.Fatalf("shadowsocks inbound users missing or invalid: %#v", inbound["users"])
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 enabled shadowsocks users, got %d", len(users))
	}

	expectedUsers := map[string]string{
		"one@example.com": ShadowsocksUserPassword("webdock", "92.113.148.36", "user-1"),
		"two@example.com": ShadowsocksUserPassword("webdock", "92.113.148.36", "user-2"),
	}
	for _, rawUser := range users {
		entry, ok := rawUser.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected shadowsocks user payload: %#v", rawUser)
		}
		name, _ := entry["name"].(string)
		password, _ := entry["password"].(string)
		if expectedUsers[name] != password {
			t.Fatalf("unexpected shadowsocks user entry: %#v", entry)
		}
	}
}

func TestBuildDNSConfig(t *testing.T) {
	t.Run("returns nil for empty servers", func(t *testing.T) {
		result := buildDNSConfig("", "prefer_ipv4", false, false, false, false)
		if result != nil {
			t.Fatalf("expected nil for empty servers, got %#v", result)
		}
	})

	t.Run("builds basic DNS config", func(t *testing.T) {
		result := buildDNSConfig("8.8.8.8,1.1.1.1", "prefer_ipv4", false, false, false, false)
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		servers, ok := result["servers"].([]map[string]interface{})
		if !ok {
			t.Fatalf("expected servers to be []map[string]interface{}, got %#v", result["servers"])
		}

		if len(servers) != 2 {
			t.Fatalf("expected 2 DNS servers, got %d", len(servers))
		}

		if servers[0]["tag"] != "dns1" || servers[0]["server"] != "8.8.8.8" {
			t.Fatalf("unexpected first server: %#v", servers[0])
		}

		if servers[1]["tag"] != "dns2" || servers[1]["server"] != "1.1.1.1" {
			t.Fatalf("unexpected second server: %#v", servers[1])
		}

		if result["strategy"] != "prefer_ipv4" {
			t.Fatalf("unexpected strategy: %#v", result["strategy"])
		}

		if result["final"] != "dns1" {
			t.Fatalf("unexpected final: %#v", result["final"])
		}
	})

	t.Run("builds config with cache disabled", func(t *testing.T) {
		result := buildDNSConfig("8.8.8.8", "", true, true, true, false)
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		if result["disable_cache"] != true {
			t.Fatalf("expected disable_cache to be true")
		}

		if result["disable_expire"] != true {
			t.Fatalf("expected disable_expire to be true")
		}

		if result["independent_cache"] != true {
			t.Fatalf("expected independent_cache to be true")
		}
	})

	t.Run("handles whitespace in server list", func(t *testing.T) {
		result := buildDNSConfig(" 8.8.8.8 , 1.1.1.1 ", "", false, false, false, false)
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		servers, ok := result["servers"].([]map[string]interface{})
		if !ok || len(servers) != 2 {
			t.Fatalf("expected 2 DNS servers, got %#v", result["servers"])
		}
	})
}
