package subscription

import (
	"encoding/json"
	"fmt"
	"net"
	"panel_backend/internal/models"
	"panel_backend/internal/services"

	"gopkg.in/yaml.v3"
)

func GenerateSingboxProfile(user models.User, nodes []models.Node, settings services.ProtocolSettings, extraOutbounds []map[string]interface{}) ([]byte, error) {
	config := buildSingboxProfileConfig(user, nodes, settings, extraOutbounds)
	return json.MarshalIndent(config, "", "  ")
}

func GenerateClashProfile(user models.User, nodes []models.Node, settings services.ProtocolSettings, extraOutbounds []map[string]interface{}) ([]byte, error) {
	config := buildClashProfileConfig(user, nodes, settings, extraOutbounds)
	return yaml.Marshal(config)
}

func buildSingboxProfileConfig(user models.User, nodes []models.Node, settings services.ProtocolSettings, extraOutbounds []map[string]interface{}) map[string]interface{} {
	availableNodes := filterAvailableNodes(user, nodes)
	nodeTags := collectOutboundTags(user, availableNodes, settings)

	extraTags := make([]string, 0, len(extraOutbounds))
	for _, ob := range extraOutbounds {
		if tag, ok := ob["tag"].(string); ok {
			extraTags = append(extraTags, tag)
		}
	}

	proxyOutbounds := make([]string, 0, len(nodeTags)+len(extraTags))
	proxyOutbounds = append(proxyOutbounds, nodeTags...)
	proxyOutbounds = append(proxyOutbounds, extraTags...)

	urltestOutbounds := make([]string, 0, len(nodeTags)+len(extraTags))
	urltestOutbounds = append(urltestOutbounds, nodeTags...)
	urltestOutbounds = append(urltestOutbounds, extraTags...)

	outbounds := []map[string]interface{}{
		{
			"type":      "selector",
			"tag":       "proxy",
			"outbounds": append([]string{"auto", "direct"}, proxyOutbounds...),
		},
		{
			"type":      "urltest",
			"tag":       "auto",
			"outbounds": urltestOutbounds,
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  "3m",
			"tolerance": 50,
		},
		{
			"type": "direct",
			"tag":  "direct",
		},
		{
			"type": "block",
			"tag":  "block",
		},
	}

	for _, node := range availableNodes {
		plan := buildNodeTransportPlan(node, settings)
		for _, variant := range plan.Reality {
			outbounds = append(outbounds, map[string]interface{}{
				"type":        "vless",
				"tag":         variant.Tag,
				"server":      node.PublicHost,
				"server_port": variant.Port,
				"uuid":        user.UUID,
				"flow":        "xtls-rprx-vision",
				"network":     "tcp",
				"tls": map[string]interface{}{
					"enabled":     true,
					"insecure":    true,
					"server_name": variant.ServerName,
					"utls": map[string]interface{}{
						"enabled":     true,
						"fingerprint": randomizedUTLSFingerprint,
					},
					"reality": map[string]interface{}{
						"enabled":    node.RealityPublicKey != "",
						"public_key": toBase64URL(node.RealityPublicKey),
						"short_id":   node.RealityShortID,
					},
				},
				"transport": map[string]interface{}{},
			})
		}
		if plan.TUIC.Port > 0 {
			outbounds = append(outbounds, map[string]interface{}{
				"type":               "tuic",
				"tag":                plan.TUIC.Tag,
				"server":             node.PublicHost,
				"server_port":        plan.TUIC.Port,
				"uuid":               user.UUID,
				"password":           user.UUID,
				"congestion_control": "bbr",
				"tls": map[string]interface{}{
					"enabled":     true,
					"insecure":    true,
					"server_name": node.PublicHost,
				},
			})
		}
		shadowsocks := buildShadowsocksVariant(node, user)
		if shadowsocks.Port > 0 {
			outbounds = append(outbounds, map[string]interface{}{
				"type":        "shadowsocks",
				"tag":         shadowsocks.Tag,
				"server":      node.PublicHost,
				"server_port": shadowsocks.Port,
				"method":      shadowsocks2022Method,
				"password":    shadowsocks.Password,
				"network":     "tcp",
				"multiplex": map[string]interface{}{
					"enabled": false,
				},
			})
		}
		for _, variant := range plan.Hysteria2 {
			outbound := map[string]interface{}{
				"type":        "hysteria2",
				"tag":         variant.Tag,
				"server":      node.PublicHost,
				"server_port": variant.Port,
				"password":    user.UUID,
				"tls": map[string]interface{}{
					"enabled":     true,
					"insecure":    true,
					"server_name": node.PublicHost,
				},
			}
			if variant.ObfsPassword != "" {
				outbound["obfs"] = map[string]interface{}{
					"type":     "salamander",
					"password": variant.ObfsPassword,
				}
			}
			outbounds = append(outbounds, map[string]interface{}{
				"type":        outbound["type"],
				"tag":         outbound["tag"],
				"server":      outbound["server"],
				"server_port": outbound["server_port"],
				"password":    outbound["password"],
				"tls":         outbound["tls"],
				"obfs":        outbound["obfs"],
			})
		}

	}

	for _, ob := range extraOutbounds {
		outbounds = append(outbounds, ob)
	}

	return map[string]interface{}{
		"dns": map[string]interface{}{
			"final":          "proxy-dns",
			"strategy":       "prefer_ipv4",
			"cache_capacity": 4096,
			"servers": []map[string]interface{}{
				{
					"tag":  "local-dns",
					"type": "local",
				},
				{
					"tag":              "proxy-dns",
					"type":             "https",
					"server":           "dns.quad9.net",
					"server_port":      443,
					"detour":           "proxy",
					"domain_resolver":  "local-dns",
				},
				{
					"tag":              "block-dns",
					"type":             "https",
					"server":           "dns.quad9.net",
					"server_port":      443,
					"detour":           "block",
					"domain_resolver":  "local-dns",
				},
				{
					"tag":         "fakeip-dns",
					"type":        "fakeip",
					"inet4_range": "198.18.0.0/15",
					"inet6_range": "fc00::/18",
				},
			},
			"rules": []map[string]interface{}{
				{
					"domain":        []string{"+.lan", "+.local", "localhost", "*.msftncsi.com", "*.msftconnecttest.com", "time.*.com", "ntp.*.com"},
					"server":        "local-dns",
					"disable_cache": true,
				},
				{
					"rule_set": []string{"geosite-cn"},
					"server":   "local-dns",
				},
				{
					"rule_set":      []string{"geosite-category-ads"},
					"server":        "block-dns",
					"disable_cache": true,
				},
	
			},
		},
			"inbounds": []map[string]interface{}{
			{
				"type":                     "tun",
				"address":                  []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"auto_route":               true,
				"endpoint_independent_nat": false,
				"mtu":                      1400,
				"stack":                    "system",
				"strict_route":             true,
				"platform": map[string]interface{}{
					"http_proxy": map[string]interface{}{
						"enabled":     true,
						"server":      "127.0.0.1",
						"server_port": 2080,
					},
				},
			},
			{
				"type":        "mixed",
				"listen":      "127.0.0.1",
				"listen_port": 2080,
				"users":       []interface{}{},
			},
		},
		"outbounds": outbounds,
			"route": map[string]interface{}{
			"auto_detect_interface":   true,
			"default_domain_resolver": "local-dns",
			"final":                   "proxy",
			"rule_set": []map[string]interface{}{
				{
					"type":   "remote",
					"tag":    "geosite-cn",
					"format": "binary",
					"url":    "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs",
				},
				{
					"type":   "remote",
					"tag":    "geosite-private",
					"format": "binary",
					"url":    "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-private.srs",
				},
				{
					"type":   "remote",
					"tag":    "geosite-category-ads",
					"format": "binary",
					"url":    "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads.srs",
				},
			},
			"rules":                   buildSingboxRouteRules(settings),
		},
		"experimental": map[string]interface{}{
			"cache_file": map[string]interface{}{
				"enabled":    true,
				"path":       "cache.db",
				"cache_id":   "singbox",
			},
		},
	}
}

func buildSingboxRouteRules(settings services.ProtocolSettings) []map[string]interface{} {
	rules := []map[string]interface{}{
		{
			"action": "sniff",
		},
		{
			"port":   53,
			"action": "hijack-dns",
		},
	}

	if len(settings.DirectPackages) > 0 {
		rules = append(rules, map[string]interface{}{
			"action":       "route",
			"package_name": settings.DirectPackages,
			"outbound":     "direct",
		})
	}
	if len(settings.DirectDomains) > 0 {
		rules = append(rules, map[string]interface{}{
			"action":        "route",
			"domain_suffix": settings.DirectDomains,
			"outbound":      "direct",
		})
	}
	if len(settings.ProxyDomains) > 0 {
		rules = append(rules, map[string]interface{}{
			"action":        "route",
			"domain_suffix": settings.ProxyDomains,
			"outbound":      "proxy",
		})
	}

	rules = append(rules, map[string]interface{}{
		"action":     "route",
		"clash_mode": "Direct",
		"outbound":   "direct",
	})

	rules = append(rules, map[string]interface{}{
		"type":   "logical",
		"mode":   "and",
		"outbound": "direct",
		"rules": []map[string]interface{}{
			{
				"rule_set": []string{"geosite-cn"},
			},
			{
				"invert":   true,
				"rule_set": []string{"geosite-private"},
			},
		},
	})

	return rules
}

func buildClashProfileConfig(user models.User, nodes []models.Node, settings services.ProtocolSettings, extraOutbounds []map[string]interface{}) map[string]interface{} {
	availableNodes := filterAvailableNodes(user, nodes)
	proxies := make([]map[string]interface{}, 0)
	proxyNames := make([]string, 0)
	nodeProxyNames := make([]string, 0)

	for _, node := range availableNodes {
		plan := buildNodeTransportPlan(node, settings)

		for _, variant := range plan.Reality {
			name := variant.Tag
			proxyNames = append(proxyNames, name)
			nodeProxyNames = append(nodeProxyNames, name)
			proxies = append(proxies, map[string]interface{}{
				"name":               name,
				"type":               "vless",
				"server":             node.PublicHost,
				"port":               variant.Port,
				"uuid":               user.UUID,
				"network":            "tcp",
				"udp":                true,
				"tls":                true,
				"flow":               "xtls-rprx-vision",
				"servername":         variant.ServerName,
				"skip-cert-verify":   true,
				"client-fingerprint": "chrome",
				"reality-opts": map[string]interface{}{
					"public-key": toBase64URL(node.RealityPublicKey),
					"short-id":   node.RealityShortID,
				},
			})
		}

		if plan.TUIC.Port > 0 {
			name := plan.TUIC.Tag
			tuicProxy := map[string]interface{}{
				"name":                  name,
				"type":                  "tuic",
				"server":                node.PublicHost,
				"port":                  plan.TUIC.Port,
				"uuid":                  user.UUID,
				"password":              user.UUID,
				"udp":                   true,
				"skip-cert-verify":      true,
				"alpn":                  []string{"h3"},
				"congestion-controller": "bbr",
				"udp-relay-mode":        "native",
				"request-timeout":       8000,
			}
			if net.ParseIP(node.PublicHost) != nil {
				tuicProxy["disable-sni"] = true
			} else {
				tuicProxy["sni"] = node.PublicHost
			}
			proxyNames = append(proxyNames, name)
			nodeProxyNames = append(nodeProxyNames, name)
			proxies = append(proxies, tuicProxy)
		}

		shadowsocks := buildShadowsocksVariant(node, user)
		if shadowsocks.Port > 0 {
			name := shadowsocks.Tag
			proxyNames = append(proxyNames, name)
			nodeProxyNames = append(nodeProxyNames, name)
			proxies = append(proxies, map[string]interface{}{
				"name":     name,
				"type":     "ss",
				"server":   node.PublicHost,
				"port":     shadowsocks.Port,
				"cipher":   shadowsocks2022Method,
				"password": shadowsocks.Password,
				"udp":      true,
			})
		}

		for _, variant := range plan.Hysteria2 {
			name := variant.Tag
			proxyNames = append(proxyNames, name)
			nodeProxyNames = append(nodeProxyNames, name)
			proxy := map[string]interface{}{
				"name":             name,
				"type":             "hysteria2",
				"server":           node.PublicHost,
				"port":             variant.Port,
				"password":         user.UUID,
				"sni":              node.PublicHost,
				"skip-cert-verify": true,
				"udp":              true,
			}
			if variant.ObfsPassword != "" {
				proxy["obfs"] = "salamander"
				proxy["obfs-password"] = variant.ObfsPassword
			}
			proxies = append(proxies, proxy)
		}
	}

	var importedProxyNames []string
	for _, ob := range extraOutbounds {
		name, _ := ob["tag"].(string)
		if name == "" {
			if r, ok := ob["remark"].(string); ok && r != "" {
				name = r
			} else {
				name = fmt.Sprintf("imported-%04d", len(proxyNames)+1)
			}
		}
		proxy := singboxToClashProxy(ob, name)
		if proxy != nil {
			proxyNames = append(proxyNames, name)
			proxies = append(proxies, proxy)
			importedProxyNames = append(importedProxyNames, name)
		}
	}

	autoGroupProxies := proxyNames
	if user.ClashFallback {
		if user.ClashFallbackMode == "sub_integration" {
			autoGroupProxies = nodeProxyNames
		} else {
			autoGroupProxies = importedProxyNames
		}
	}
	if len(autoGroupProxies) == 0 {
		autoGroupProxies = []string{"DIRECT"}
	}

	proxyGroups := []map[string]interface{}{
		{
			"name":      "AUTO",
			"type":      "url-test",
			"proxies":   autoGroupProxies,
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  user.ClashAutoInterval,
			"tolerance": user.ClashAutoTolerance,
		},
	}

	if user.ClashFallback {
		var fbNames []string
		if user.ClashFallbackMode == "sub_integration" {
			fbNames = importedProxyNames
		} else {
			fbNames = nodeProxyNames
		}
		fbLimit := user.ClashFallbackCount
		if fbLimit > 0 && len(fbNames) > fbLimit {
			fbNames = fbNames[:fbLimit]
		}

		if len(fbNames) > 0 {
			proxyGroups = append(proxyGroups,
				map[string]interface{}{
					"name":      "Fallback-Nodes",
					"type":      "url-test",
					"proxies":   fbNames,
					"url":       "http://www.gstatic.com/generate_204",
					"interval":  user.ClashFallbackInterval,
					"tolerance": user.ClashFallbackTolerance,
				},
				map[string]interface{}{
					"name":      "FALLBACK",
					"type":      "fallback",
					"proxies":   []string{"AUTO", "Fallback-Nodes"},
					"url":       "http://www.gstatic.com/generate_204",
					"interval":  user.ClashFallbackInterval,
					"timeout":   3000,
				},
			)
		}
	}

	selectProxies := []string{"AUTO"}
	if user.ClashFallback {
		selectProxies = append(selectProxies, "FALLBACK")
	}
	selectProxies = append(selectProxies, "DIRECT")
	selectProxies = append(selectProxies, proxyNames...)

	proxyGroups = append(proxyGroups, map[string]interface{}{
		"name":    "Proxy",
		"type":    "select",
		"proxies": selectProxies,
	})

	return map[string]interface{}{
		"mixed-port":                7890,
		"allow-lan":                 false,
		"ipv6":                      false,
		"log-level":                 "info",
		"mode":                      "rule",
		"global-client-fingerprint": "chrome",
		"global-ua":                 "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
		"connection-pool": map[string]interface{}{
			"enable":      true,
			"max-streams": 2048,
		},
		"sniffer": map[string]interface{}{
			"enable":            true,
			"force-dns-mapping": true,
			"parse-pure-ip":     true,
			"sniff": map[string]interface{}{
				"TLS": map[string]interface{}{
					"ports": []int{443, 8443},
				},
				"HTTP": map[string]interface{}{
					"ports":               []interface{}{80, 8080, "8000-9000"},
					"override-destination": true,
				},
				"QUIC": map[string]interface{}{
					"ports": []int{443, 8443},
				},
			},
		},
		"profile": map[string]interface{}{
			"store-selected": true,
			"store-fake-ip":  true,
		},
		"dns": map[string]interface{}{
			"enable":        true,
			"respect-rules": true,
			"enhanced-mode": "fake-ip",
			"fake-ip-range": "198.18.0.1/16",
			"cache-size":    4096,
			"fake-ip-filter": []string{
				"*.lan",
				"*.localdomain",
				"*.local",
				"*.msftncsi.com",
				"*.msftconnecttest.com",
				"time.*.com",
				"ntp.*.com",
				"+.pool.ntp.org",
			},
			"default-nameserver": []string{
				"223.5.5.5",
				"119.29.29.29",
			},
			"proxy-server-nameserver": []string{
				"https://dns.quad9.net/dns-query",
				"https://dns.cloudflare.com/dns-query",
			},
			"direct-nameserver": []string{
				"https://dns.google/dns-query",
				"https://1.1.1.1/dns-query",
			},
		},
		"tun": map[string]interface{}{
			"enable":                true,
			"stack":                 "mixed",
			"auto-route":            true,
			"auto-detect-interface": true,
			"dns-hijack": []string{
				"any:53",
			},
		},
		"rule-providers": map[string]interface{}{
			"reject": map[string]interface{}{
				"type":     "http",
				"behavior": "domain",
				"url":      "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/reject.txt",
				"path":     "./ruleset/reject.yaml",
				"interval": 86400,
			},
			"icloud": map[string]interface{}{
				"type":     "http",
				"behavior": "domain",
				"url":      "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/icloud.txt",
				"path":     "./ruleset/icloud.yaml",
				"interval": 86400,
			},
			"apple": map[string]interface{}{
				"type":     "http",
				"behavior": "domain",
				"url":      "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/apple.txt",
				"path":     "./ruleset/apple.yaml",
				"interval": 86400,
			},
			"google": map[string]interface{}{
				"type":     "http",
				"behavior": "domain",
				"url":      "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/google.txt",
				"path":     "./ruleset/google.yaml",
				"interval": 86400,
			},
			"proxy": map[string]interface{}{
				"type":     "http",
				"behavior": "domain",
				"url":      "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/proxy.txt",
				"path":     "./ruleset/proxy.yaml",
				"interval": 86400,
			},
			"direct": map[string]interface{}{
				"type":     "http",
				"behavior": "domain",
				"url":      "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/direct.txt",
				"path":     "./ruleset/direct.yaml",
				"interval": 86400,
			},
			"youtube": map[string]interface{}{
				"type":     "http",
				"behavior": "classical",
				"url":      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/YouTube/YouTube.yaml",
				"path":     "./ruleset/youtube.yaml",
				"interval": 86400,
			},
		},
		"proxies": proxies,
		"proxy-groups": proxyGroups,
		"rules": []string{
			"DOMAIN,dns.google,REJECT",
			"DOMAIN,dns.quad9.net,REJECT",
			"DOMAIN,cloudflare-dns.com,REJECT",
			"DOMAIN,doh.opendns.com,REJECT",
			"DOMAIN,dns10.quad9.net,REJECT",
			"DOMAIN,mozilla.cloudflare-dns.com,REJECT",
			"DOMAIN-SUFFIX,.mm,DIRECT",
			"DOMAIN-SUFFIX,telegram.org,DIRECT",
			"DOMAIN,t.me,DIRECT",
			"DOMAIN,telegram.me,DIRECT",
			"DOMAIN-SUFFIX,telegra.ph,DIRECT",
			"DOMAIN-SUFFIX,tiktok.com,DIRECT",
			"DOMAIN-SUFFIX,tiktokv.com,DIRECT",
			"DOMAIN-SUFFIX,tiktokcdn.com,DIRECT",
			"DOMAIN-SUFFIX,tiktokcdn-us.com,DIRECT",
			"DOMAIN-SUFFIX,byteoversea.com,DIRECT",
			"DOMAIN-SUFFIX,ibytedtos.com,DIRECT",
			"RULE-SET,youtube,DIRECT",
			"RULE-SET,google,DIRECT",
			"RULE-SET,reject,REJECT",
			"RULE-SET,icloud,DIRECT",
			"RULE-SET,apple,DIRECT",
			"RULE-SET,proxy,Proxy",
			"RULE-SET,direct,DIRECT",
			"MATCH,Proxy",
		},
		"experimental": map[string]interface{}{
			"h2-concise": true,
		},
	}
}

func collectOutboundTags(user models.User, nodes []models.Node, settings services.ProtocolSettings) []string {
	tags := []string{}
	for _, node := range nodes {
		plan := buildNodeTransportPlan(node, settings)
		for _, variant := range plan.Reality {
			tags = append(tags, variant.Tag)
		}
		if plan.TUIC.Port > 0 {
			tags = append(tags, plan.TUIC.Tag)
		}
		shadowsocks := buildShadowsocksVariant(node, user)
		if shadowsocks.Port > 0 {
			tags = append(tags, shadowsocks.Tag)
		}
		for _, variant := range plan.Hysteria2 {
			tags = append(tags, variant.Tag)
		}
	}
	if len(tags) == 0 {
		return []string{"direct"}
	}
	return tags
}

func singboxToClashProxy(cfg map[string]interface{}, name string) map[string]interface{} {
	proto, _ := cfg["type"].(string)
	server, _ := cfg["server"].(string)
	serverPort, _ := cfg["server_port"].(float64)

	if server == "" || serverPort == 0 {
		return nil
	}

	switch proto {
	case "vless":
		proxy := map[string]interface{}{
			"name":             name,
			"type":             "vless",
			"server":           server,
			"port":             int(serverPort),
			"uuid":             cfg["uuid"],
			"network":          "tcp",
			"udp":              true,
			"tls":              true,
			"skip-cert-verify": true,
		}
		if flow, ok := cfg["flow"].(string); ok && flow != "" {
			proxy["flow"] = flow
		}
		if tls, ok := cfg["tls"].(map[string]interface{}); ok {
			if sn, ok := tls["server_name"].(string); ok && sn != "" {
				proxy["servername"] = sn
			}
			if utls, ok := tls["utls"].(map[string]interface{}); ok {
				if fp, ok := utls["fingerprint"].(string); ok && fp != "" {
					proxy["client-fingerprint"] = fp
				}
			}
			if reality, ok := tls["reality"].(map[string]interface{}); ok {
				realityOpts := make(map[string]interface{})
				if pk, ok := reality["public_key"].(string); ok && pk != "" {
					realityOpts["public-key"] = pk
				}
				if sid, ok := reality["short_id"].(string); ok && sid != "" {
					realityOpts["short-id"] = sid
				}
				if len(realityOpts) > 0 {
					proxy["reality-opts"] = realityOpts
				}
			}
		}
		return proxy

	case "vmess":
		proxy := map[string]interface{}{
			"name":             name,
			"type":             "vmess",
			"server":           server,
			"port":             int(serverPort),
			"uuid":             cfg["uuid"],
			"udp":              true,
			"skip-cert-verify": true,
		}
		if tls, ok := cfg["tls"].(map[string]interface{}); ok {
			proxy["tls"] = true
			if sn, ok := tls["server_name"].(string); ok && sn != "" {
				proxy["servername"] = sn
			}
		}
		return proxy

	case "trojan":
		proxy := map[string]interface{}{
			"name":             name,
			"type":             "trojan",
			"server":           server,
			"port":             int(serverPort),
			"password":         cfg["password"],
			"udp":              true,
			"skip-cert-verify": true,
		}
		if tls, ok := cfg["tls"].(map[string]interface{}); ok {
			if sn, ok := tls["server_name"].(string); ok && sn != "" {
				proxy["sni"] = sn
			}
		}
		return proxy

	case "shadowsocks":
		method, _ := cfg["method"].(string)
		if method == "" {
			method = "aes-256-gcm"
		}
		return map[string]interface{}{
			"name":     name,
			"type":     "ss",
			"server":   server,
			"port":     int(serverPort),
			"cipher":   method,
			"password": cfg["password"],
			"udp":      true,
		}

	case "hysteria2":
		proxy := map[string]interface{}{
			"name":             name,
			"type":             "hysteria2",
			"server":           server,
			"port":             int(serverPort),
			"password":         cfg["password"],
			"udp":              true,
			"skip-cert-verify": true,
		}
		if tls, ok := cfg["tls"].(map[string]interface{}); ok {
			if sn, ok := tls["server_name"].(string); ok && sn != "" {
				proxy["sni"] = sn
			}
		}
		if obfs, ok := cfg["obfs"].(map[string]interface{}); ok {
			if ot, ok := obfs["type"].(string); ok && ot == "salamander" {
				proxy["obfs"] = "salamander"
				if op, ok := obfs["password"].(string); ok && op != "" {
					proxy["obfs-password"] = op
				}
			}
		}
		return proxy

	case "tuic":
		return map[string]interface{}{
			"name":                  name,
			"type":                  "tuic",
			"server":                server,
			"port":                  int(serverPort),
			"uuid":                  cfg["uuid"],
			"password":              cfg["password"],
			"udp":                   true,
			"skip-cert-verify":      true,
			"alpn":                  []string{"h3"},
			"congestion-controller": "bbr",
			"udp-relay-mode":        "native",
		}
	}

	return nil
}
