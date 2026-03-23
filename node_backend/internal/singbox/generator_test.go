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
