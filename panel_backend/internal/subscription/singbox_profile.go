package subscription

import (
	"encoding/json"
	"net"
	"panel_backend/internal/models"
	"panel_backend/internal/services"

	"gopkg.in/yaml.v3"
)

func GenerateSingboxProfile(user models.User, nodes []models.Node, settings services.ProtocolSettings, extraOutbounds []map[string]interface{}) ([]byte, error) {
	config := buildSingboxProfileConfig(user, nodes, settings, extraOutbounds)
	return json.MarshalIndent(config, "", "  ")
}

func GenerateClashProfile(user models.User, nodes []models.Node, settings services.ProtocolSettings) ([]byte, error) {
	config := buildClashProfileConfig(user, nodes, settings)
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

	urltestOutbounds := nodeTags

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
			"interval":  "10m",
			"tolerance": 50,
		},
		{
			"type": "direct",
			"tag":  "direct",
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
			"final":    "proxy-dns",
			"strategy": "prefer_ipv4",
			"servers": []map[string]interface{}{
				{
					"tag":  "local-dns",
					"type": "local",
				},
				{
					"tag":    "proxy-dns",
					"type":   "https",
					"server": "dns.quad9.net",
					"detour": "proxy",
				},
				{
					"tag":         "fakeip-dns",
					"type":        "fakeip",
					"inet4_range": "198.18.0.0/16",
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
			"domain_strategy":         "prefer_ipv4",
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
			"action":        "sniff",
			"sniff_timeout": "300ms",
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

func buildClashProfileConfig(user models.User, nodes []models.Node, settings services.ProtocolSettings) map[string]interface{} {
	availableNodes := filterAvailableNodes(user, nodes)
	proxies := make([]map[string]interface{}, 0)
	proxyNames := make([]string, 0)

	for _, node := range availableNodes {
		plan := buildNodeTransportPlan(node, settings)

		for _, variant := range plan.Reality {
			name := variant.Tag
			proxyNames = append(proxyNames, name)
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
			proxies = append(proxies, tuicProxy)
		}

		shadowsocks := buildShadowsocksVariant(node, user)
		if shadowsocks.Port > 0 {
			name := shadowsocks.Tag
			proxyNames = append(proxyNames, name)
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

	groupProxies := append([]string{"AUTO", "DIRECT"}, proxyNames...)
	autoGroupProxies := proxyNames
	if len(autoGroupProxies) == 0 {
		autoGroupProxies = []string{"DIRECT"}
	}

	return map[string]interface{}{
		"mixed-port": 7890,
		"allow-lan":  false,
		"mode":       "rule",
		"log-level":  "info",
		"ipv6":       true,
		"proxies":    proxies,
		"proxy-groups": []map[string]interface{}{
			{
				"name":      "AUTO",
				"type":      "url-test",
				"proxies":   autoGroupProxies,
				"url":       "http://www.gstatic.com/generate_204",
				"interval":  600,
				"tolerance": 50,
			},
			{
				"name":    "Proxy",
				"type":    "select",
				"proxies": groupProxies,
			},
		},
		"rules": []string{
			"MATCH,Proxy",
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
