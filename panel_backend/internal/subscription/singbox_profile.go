package subscription

import (
	"encoding/json"
	"panel_backend/internal/models"
	"panel_backend/internal/services"
)

func GenerateSingboxProfile(user models.User, nodes []models.Node, settings services.ProtocolSettings) ([]byte, error) {
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

	config := map[string]interface{}{
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

	return json.MarshalIndent(config, "", "  ")
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
