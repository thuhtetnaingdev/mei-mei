package subscription

import (
	"encoding/json"
	"fmt"
	"panel_backend/internal/models"
)

func GenerateSingboxProfile(user models.User, nodes []models.Node) ([]byte, error) {
	availableNodes := filterAvailableNodes(nodes)
	proxyOutbounds := collectOutboundTags(availableNodes)
	urltestOutbounds := collectOutboundTags(availableNodes)

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
		if node.VLESSPort > 0 {
			serverName := node.RealityServerName
			if serverName == "" {
				serverName = node.PublicHost
			}
			outbounds = append(outbounds, map[string]interface{}{
				"type":        "vless",
				"tag":         fmt.Sprintf("%s-vless", node.Name),
				"server":      node.PublicHost,
				"server_port": node.VLESSPort,
				"uuid":        user.UUID,
				"flow":        "xtls-rprx-vision",
				"network":     "tcp",
				"tls": map[string]interface{}{
					"enabled":     true,
					"insecure":    true,
					"server_name": serverName,
					"utls": map[string]interface{}{
						"enabled":     true,
						"fingerprint": "chrome",
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
		if node.TUICPort > 0 {
			outbounds = append(outbounds, map[string]interface{}{
				"type":               "tuic",
				"tag":                fmt.Sprintf("%s-tuic", node.Name),
				"server":             node.PublicHost,
				"server_port":        node.TUICPort,
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
		if node.Hysteria2Port > 0 {
			outbounds = append(outbounds, map[string]interface{}{
				"type":        "hysteria2",
				"tag":         fmt.Sprintf("%s-hy2", node.Name),
				"server":      node.PublicHost,
				"server_port": node.Hysteria2Port,
				"password":    user.UUID,
				"tls": map[string]interface{}{
					"enabled":     true,
					"insecure":    true,
					"server_name": node.PublicHost,
				},
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

func collectOutboundTags(nodes []models.Node) []string {
	tags := []string{}
	for _, node := range nodes {
		if node.VLESSPort > 0 {
			tags = append(tags, fmt.Sprintf("%s-vless", node.Name))
		}
		if node.TUICPort > 0 {
			tags = append(tags, fmt.Sprintf("%s-tuic", node.Name))
		}
		if node.Hysteria2Port > 0 {
			tags = append(tags, fmt.Sprintf("%s-hy2", node.Name))
		}
	}
	if len(tags) == 0 {
		return []string{"direct"}
	}
	return tags
}
