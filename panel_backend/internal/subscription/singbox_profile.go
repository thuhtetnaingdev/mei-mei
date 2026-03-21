package subscription

import (
	"encoding/json"
	"panel_backend/internal/models"
	"panel_backend/internal/services"

	"gopkg.in/yaml.v3"
)

func GenerateSingboxProfile(user models.User, nodes []models.Node, settings services.ProtocolSettings) ([]byte, error) {
	config := buildSingboxProfileConfig(user, nodes, settings)
	return json.MarshalIndent(config, "", "  ")
}

func GenerateClashProfile(user models.User, nodes []models.Node, settings services.ProtocolSettings) ([]byte, error) {
	config := buildClashProfileConfig(user, nodes, settings)
	return yaml.Marshal(config)
}

func buildSingboxProfileConfig(user models.User, nodes []models.Node, settings services.ProtocolSettings) map[string]interface{} {
	availableNodes := filterAvailableNodes(nodes)
	proxyOutbounds := collectOutboundTags(user, availableNodes, settings)
	urltestOutbounds := collectOutboundTags(user, availableNodes, settings)

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
						"public_key": node.RealityPublicKey,
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
					"enabled": true,
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

	return map[string]interface{}{
		"inbounds": []map[string]interface{}{
			{
				"type":                     "tun",
				"address":                  []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"auto_route":               true,
				"endpoint_independent_nat": false,
				"mtu":                      9000,
				"platform": map[string]interface{}{
					"http_proxy": map[string]interface{}{
						"enabled":     true,
						"server":      "127.0.0.1",
						"server_port": 2080,
					},
				},
				"stack":        "system",
				"strict_route": false,
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
			"auto_detect_interface": true,
			"final":                 "proxy",
			"rules": []map[string]interface{}{
				{
					"action": "sniff",
				},
				{
					"action":     "route",
					"clash_mode": "Direct",
					"outbound":   "direct",
				},
			},
		},
	}
}

func buildClashProfileConfig(user models.User, nodes []models.Node, settings services.ProtocolSettings) map[string]interface{} {
	availableNodes := filterAvailableNodes(nodes)
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
					"public-key": node.RealityPublicKey,
					"short-id":   node.RealityShortID,
				},
			})
		}

		if plan.TUIC.Port > 0 {
			name := plan.TUIC.Tag
			proxyNames = append(proxyNames, name)
			proxies = append(proxies, map[string]interface{}{
				"name":                  name,
				"type":                  "tuic",
				"server":                node.PublicHost,
				"port":                  plan.TUIC.Port,
				"uuid":                  user.UUID,
				"password":              user.UUID,
				"udp":                   true,
				"sni":                   node.PublicHost,
				"skip-cert-verify":      true,
				"congestion-controller": "bbr",
			})
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
