package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"panel_backend/internal/models"
	"panel_backend/internal/services"
	"gopkg.in/yaml.v3"
)

func TestGenerateNodeLinksIncludesUserSpecificShadowsocksLink(t *testing.T) {
	user := models.User{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true}
	node := models.Node{Name: "webdock", PublicHost: "92.113.148.36", Enabled: true}

	links := GenerateNodeLinks(user, []models.Node{node}, services.ProtocolSettings{})

	var shadowsocksLink string
	for _, link := range links {
		if link.Protocol == "shadowsocks" {
			shadowsocksLink = link.URL
			break
		}
	}

	if shadowsocksLink == "" {
		t.Fatal("expected shadowsocks link to be generated")
	}

	payload := strings.TrimPrefix(shadowsocksLink, "ss://")
	parts := strings.SplitN(payload, "@", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid shadowsocks link: %s", shadowsocksLink)
	}

	credentials, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("failed to decode credentials: %v", err)
	}

	expectedCredentials := shadowsocks2022Method + ":" + shadowsocksUserPassword(node, user.UUID)
	if string(credentials) != expectedCredentials {
		t.Fatalf("unexpected shadowsocks credentials: got %q want %q", string(credentials), expectedCredentials)
	}

	if !strings.Contains(parts[1], ":10000#") {
		t.Fatalf("expected user-specific shadowsocks port in link, got %s", shadowsocksLink)
	}
}

func TestGenerateSingboxProfileIncludesMatchingShadowsocksOutbound(t *testing.T) {
	user := models.User{ID: 10002, UUID: "user-2", Email: "two@example.com", Enabled: true}
	node := models.Node{Name: "webdock", PublicHost: "92.113.148.36", Enabled: true}

	payload, err := GenerateSingboxProfile(user, []models.Node{node}, services.ProtocolSettings{})
	if err != nil {
		t.Fatalf("GenerateSingboxProfile() error = %v", err)
	}

	var profile struct {
		Outbounds []map[string]interface{} `json:"outbounds"`
	}
	if err := json.Unmarshal(payload, &profile); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	expectedPort := float64(60001)
	expectedPassword := shadowsocksUserPassword(node, user.UUID)

	for _, outbound := range profile.Outbounds {
		if outbound["type"] != "shadowsocks" {
			continue
		}

		if outbound["server_port"] != expectedPort {
			t.Fatalf("unexpected shadowsocks port: got %v want %v", outbound["server_port"], expectedPort)
		}
		if outbound["password"] != expectedPassword {
			t.Fatalf("unexpected shadowsocks password: got %v want %v", outbound["password"], expectedPassword)
		}
		return
	}

	t.Fatal("expected shadowsocks outbound to be present in sing-box profile")
}

func TestGenerateClashProfileUsesDisableSNIForTUICIPHosts(t *testing.T) {
	user := models.User{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true}
	node := models.Node{Name: "webdock", PublicHost: "92.113.148.36", Enabled: true}

	payload, err := GenerateClashProfile(user, []models.Node{node}, services.ProtocolSettings{})
	if err != nil {
		t.Fatalf("GenerateClashProfile() error = %v", err)
	}

	var profile struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(payload, &profile); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	for _, proxy := range profile.Proxies {
		if proxy["type"] != "tuic" {
			continue
		}

		if proxy["sni"] != nil {
			t.Fatalf("expected TUIC proxy to omit sni for IP hosts, got %v", proxy["sni"])
		}
		if proxy["disable-sni"] != true {
			t.Fatalf("expected TUIC proxy to set disable-sni=true, got %v", proxy["disable-sni"])
		}
		return
	}

	t.Fatal("expected TUIC proxy to be present in clash profile")
}
