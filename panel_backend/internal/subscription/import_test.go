package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func vmessLink(t *testing.T, config map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(raw)
}

func TestParseVMessAcceptsStringPortAndAid(t *testing.T) {
	link := vmessLink(t, map[string]any{
		"add":  "67.220.95.49",
		"aid":  "0",
		"host": "",
		"sni":  "xmfhlpy.com",
		"id":   "5c040f1d-45db-43bd-8d46-e4950e433735",
		"net":  "tcp",
		"port": "18000",
		"ps":   "US | provider",
		"tls":  "tls",
	})

	p := ParseURI(link)
	if p == nil {
		t.Fatal("vmess link with string port/aid was not parsed")
	}
	if p.Port != 18000 {
		t.Fatalf("port = %d, want 18000", p.Port)
	}
	if p.SNI != "xmfhlpy.com" {
		t.Fatalf("sni = %q, want xmfhlpy.com", p.SNI)
	}
	if p.Security != "tls" || p.Network != "tcp" {
		t.Fatalf("security/network = %q/%q, want tls/tcp", p.Security, p.Network)
	}
	if p.UUID != "5c040f1d-45db-43bd-8d46-e4950e433735" || p.Remark != "US | provider" {
		t.Fatalf("unexpected metadata: %+v", p)
	}
}

func TestParseVMessFallsBackToHostForSNI(t *testing.T) {
	link := vmessLink(t, map[string]any{
		"add":  "1.2.3.4",
		"host": "cdn.example.com",
		"id":   "5c040f1d-45db-43bd-8d46-e4950e433735",
		"port": 1270,
		"tls":  "tls",
	})

	p := ParseURI(link)
	if p == nil {
		t.Fatal("vmess link was not parsed")
	}
	if p.SNI != "cdn.example.com" {
		t.Fatalf("sni = %q, want cdn.example.com", p.SNI)
	}
}

func TestBuildXrayStreamSettingsDefaults(t *testing.T) {
	cfg := buildXrayConfig(ParsedProxy{
		Protocol: "shadowsocks",
		Host:     "1.2.3.4",
		Port:     443,
		Method:   "aes-256-gcm",
		Password: "secret",
	}, 19999)
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var decoded struct {
		Outbounds []struct {
			StreamSettings map[string]any `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	ss := decoded.Outbounds[0].StreamSettings
	if ss["network"] != "tcp" {
		t.Fatalf("network = %v, want tcp", ss["network"])
	}
	if ss["security"] != "none" {
		t.Fatalf("security = %v, want none", ss["security"])
	}
	if strings.Contains(string(raw), `"network":""`) {
		t.Fatalf("empty network must not reach xray: %s", raw)
	}
}
