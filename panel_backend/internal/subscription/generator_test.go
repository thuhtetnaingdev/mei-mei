package subscription

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"panel_backend/internal/models"
	"panel_backend/internal/services"
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

	credentials, err := url.QueryUnescape(parts[0])
	if err != nil {
		t.Fatalf("failed to decode credentials: %v", err)
	}

	expectedCredentials := shadowsocks2022Method + ":" + shadowsocksCombinedPassword(node, user.UUID)
	if credentials != expectedCredentials {
		t.Fatalf("unexpected shadowsocks credentials: got %q want %q", credentials, expectedCredentials)
	}

	expectedPort := shadowsocksPort(node)
	if !strings.Contains(parts[1], ":"+strconv.Itoa(expectedPort)+"#") {
		t.Fatalf("expected shared shadowsocks port in link, got %s", shadowsocksLink)
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

	expectedPort := float64(shadowsocksPort(node))
	expectedPassword := shadowsocksCombinedPassword(node, user.UUID)

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
		multiplex, ok := outbound["multiplex"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected shadowsocks outbound to include multiplex settings, got %#v", outbound["multiplex"])
		}
		if multiplex["enabled"] != false {
			t.Fatalf("expected shadowsocks outbound multiplex.enabled=false, got %v", multiplex["enabled"])
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
		alpn, ok := proxy["alpn"].([]interface{})
		if !ok || len(alpn) != 1 || alpn[0] != "h3" {
			t.Fatalf("expected TUIC proxy to include alpn [h3], got %#v", proxy["alpn"])
		}
		if proxy["udp-relay-mode"] != "native" {
			t.Fatalf("expected TUIC proxy to set udp-relay-mode=native, got %v", proxy["udp-relay-mode"])
		}
		if proxy["request-timeout"] != 8000 {
			t.Fatalf("expected TUIC proxy to set request-timeout=8000, got %v", proxy["request-timeout"])
		}
		return
	}

	t.Fatal("expected TUIC proxy to be present in clash profile")
}

func TestGenerateSingboxProfileIncludesDirectInSelectorButNotAutoUrltestGroup(t *testing.T) {
	user := models.User{ID: 10003, UUID: "user-4", Email: "four@example.com", Enabled: true}
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

	var selectorHasDirect bool
	var checkedAutoGroup bool

	for _, outbound := range profile.Outbounds {
		values, ok := outbound["outbounds"].([]interface{})
		if !ok {
			continue
		}

		if outbound["type"] == "selector" && outbound["tag"] == "proxy" {
			for _, value := range values {
				if value == "direct" {
					selectorHasDirect = true
				}
			}
		}

		if outbound["type"] == "urltest" && outbound["tag"] == "auto" {
			checkedAutoGroup = true
			for _, value := range values {
				if value == "direct" {
					t.Fatalf("expected auto urltest group to omit direct, got %#v", values)
				}
			}
		}
	}

	if !selectorHasDirect {
		t.Fatal("expected selector group to include direct outbound")
	}
	if !checkedAutoGroup {
		t.Fatal("expected auto urltest outbound to be present in sing-box profile")
	}
}

func TestGenerateSingboxProfileIncludesDNSSupportForDirectMode(t *testing.T) {
	user := models.User{ID: 10004, UUID: "user-5", Email: "five@example.com", Enabled: true}
	node := models.Node{Name: "webdock", PublicHost: "92.113.148.36", Enabled: true}

	payload, err := GenerateSingboxProfile(user, []models.Node{node}, services.ProtocolSettings{})
	if err != nil {
		t.Fatalf("GenerateSingboxProfile() error = %v", err)
	}

	var profile map[string]interface{}
	if err := json.Unmarshal(payload, &profile); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	dnsConfig, ok := profile["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dns config in profile, got %#v", profile["dns"])
	}
	if dnsConfig["final"] != "local-dns" {
		t.Fatalf("expected dns.final=local-dns, got %#v", dnsConfig["final"])
	}

	inbounds, ok := profile["inbounds"].([]interface{})
	if !ok {
		t.Fatalf("expected inbounds in profile, got %#v", profile["inbounds"])
	}
	var tunStrictRoute any
	for _, inbound := range inbounds {
		item, ok := inbound.(map[string]interface{})
		if !ok || item["type"] != "tun" {
			continue
		}
		tunStrictRoute = item["strict_route"]
	}
	if tunStrictRoute != true {
		t.Fatalf("expected tun strict_route=true, got %#v", tunStrictRoute)
	}

	routeConfig, ok := profile["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected route config in profile, got %#v", profile["route"])
	}
	if routeConfig["default_domain_resolver"] != "local-dns" {
		t.Fatalf("expected route.default_domain_resolver=local-dns, got %#v", routeConfig["default_domain_resolver"])
	}

	rules, ok := routeConfig["rules"].([]interface{})
	if !ok {
		t.Fatalf("expected route rules in profile, got %#v", routeConfig["rules"])
	}
	for _, rule := range rules {
		item, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		if item["action"] == "hijack-dns" {
			return
		}
	}

	t.Fatalf("expected route rules to include hijack-dns, got %#v", rules)
}

func TestGenerateSingboxProfileIncludesConfiguredRoutingRules(t *testing.T) {
	user := models.User{ID: 3, UUID: "user-3", Email: "three@example.com", Enabled: true}
	node := models.Node{Name: "webdock", PublicHost: "92.113.148.36", Enabled: true}
	settings := services.ProtocolSettings{
		DirectPackages: []string{"com.mobile.legends", "com.mobile.legends.usa"},
		DirectDomains:  []string{"mongodb.net"},
		ProxyDomains:   []string{"facebook.com", "fbcdn.net"},
	}

	payload, err := GenerateSingboxProfile(user, []models.Node{node}, settings)
	if err != nil {
		t.Fatalf("GenerateSingboxProfile() error = %v", err)
	}

	var profile struct {
		Route struct {
			Rules []map[string]interface{} `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(payload, &profile); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	var hasPackageRule bool
	var hasDirectDomainRule bool
	var hasProxyDomainRule bool

	for _, rule := range profile.Route.Rules {
		if packageNames, ok := rule["package_name"].([]interface{}); ok && rule["outbound"] == "direct" {
			values := make([]string, 0, len(packageNames))
			for _, value := range packageNames {
				values = append(values, value.(string))
			}
			if strings.Join(values, ",") == strings.Join(settings.DirectPackages, ",") {
				hasPackageRule = true
			}
		}

		if domainSuffixes, ok := rule["domain_suffix"].([]interface{}); ok {
			values := make([]string, 0, len(domainSuffixes))
			for _, value := range domainSuffixes {
				values = append(values, value.(string))
			}
			joined := strings.Join(values, ",")
			if rule["outbound"] == "direct" && joined == strings.Join(settings.DirectDomains, ",") {
				hasDirectDomainRule = true
			}
			if rule["outbound"] == "proxy" && joined == strings.Join(settings.ProxyDomains, ",") {
				hasProxyDomainRule = true
			}
		}
	}

	if !hasPackageRule {
		t.Fatalf("expected direct package_name rule, got %#v", profile.Route.Rules)
	}
	if !hasDirectDomainRule {
		t.Fatalf("expected direct domain_suffix rule, got %#v", profile.Route.Rules)
	}
	if !hasProxyDomainRule {
		t.Fatalf("expected proxy domain_suffix rule, got %#v", profile.Route.Rules)
	}
}

func TestGenerateNodeLinksFiltersTestingUsersToTestableNodes(t *testing.T) {
	user := models.User{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true, IsTesting: true}
	nodes := []models.Node{
		{Name: "prod", PublicHost: "prod.example.com", Enabled: true, IsTestable: false},
		{Name: "test", PublicHost: "test.example.com", Enabled: true, IsTestable: true},
	}

	links := GenerateNodeLinks(user, nodes, services.ProtocolSettings{})

	if len(links) == 0 {
		t.Fatal("expected testing user to receive at least one node link")
	}
	for _, link := range links {
		if link.NodeName != "test" {
			t.Fatalf("unexpected node %q in testing user links", link.NodeName)
		}
	}
}
