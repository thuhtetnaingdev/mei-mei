package singbox

import "encoding/json"

type User struct {
	ID               uint   `json:"id"`
	UUID             string `json:"uuid"`
	Email            string `json:"email"`
	Enabled          bool   `json:"enabled"`
	BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
}

type Config struct {
	Log          map[string]interface{}   `json:"log"`
	Experimental map[string]interface{}   `json:"experimental,omitempty"`
	Inbounds     []map[string]interface{} `json:"inbounds"`
	Outbounds    []map[string]interface{} `json:"outbounds"`
	Route        map[string]interface{}   `json:"route"`
}

func Generate(nodeName, publicHost string, realityPrivateKey, realityServerName, realityShortID, handshakeServer string, handshakePort int, tlsCertPath, tlsKeyPath, tlsServerName, v2rayAPIListen string, realitySNIs, hysteria2Masquerades []string, users []User) ([]byte, error) {
	vlessClients := make([]map[string]interface{}, 0, len(users))
	tuicClients := make([]map[string]interface{}, 0, len(users))
	hy2Clients := make([]map[string]interface{}, 0, len(users))
	statsUsers := make([]string, 0, len(users))
	for _, user := range users {
		if !user.Enabled {
			continue
		}
		statsUsers = append(statsUsers, user.Email)
		vlessClients = append(vlessClients, map[string]interface{}{
			"uuid": user.UUID,
			"name": user.Email,
			"flow": "xtls-rprx-vision",
		})
		tuicClients = append(tuicClients, map[string]interface{}{
			"uuid":     user.UUID,
			"password": user.UUID,
			"name":     user.Email,
		})
		hy2Clients = append(hy2Clients, map[string]interface{}{
			"password": user.UUID,
			"name":     user.Email,
		})
	}

	layout := BuildTransportLayout(nodeName, publicHost, realitySNIs, hysteria2Masquerades)
	shadowsocksPlans := BuildShadowsocksInboundPlans(nodeName, publicHost, users)
	inbounds := make([]map[string]interface{}, 0, len(layout.VLESS)+len(layout.Hysteria2)+len(shadowsocksPlans)+1)

	for _, plan := range layout.VLESS {
		handshakeTarget := handshakeServer
		if plan.ServerName != "" {
			handshakeTarget = plan.ServerName
		}

		inbounds = append(inbounds, map[string]interface{}{
			"type":        "vless",
			"tag":         plan.Tag,
			"listen":      "::",
			"listen_port": plan.Port,
			"users":       vlessClients,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": plan.ServerName,
				"reality": map[string]interface{}{
					"enabled": true,
					"handshake": map[string]interface{}{
						"server":      handshakeTarget,
						"server_port": handshakePort,
					},
					"private_key": realityPrivateKey,
					"short_id":    []string{realityShortID},
				},
			},
		})
	}

	if layout.TUIC.Port > 0 {
		inbounds = append(inbounds, map[string]interface{}{
			"type":               "tuic",
			"tag":                layout.TUIC.Tag,
			"listen":             "::",
			"listen_port":        layout.TUIC.Port,
			"users":              tuicClients,
			"congestion_control": "bbr",
			"tls": map[string]interface{}{
				"enabled":          true,
				"server_name":      tlsServerName,
				"certificate_path": tlsCertPath,
				"key_path":         tlsKeyPath,
			},
		})
	}

	for _, plan := range shadowsocksPlans {
		inbounds = append(inbounds, map[string]interface{}{
			"type":        "shadowsocks",
			"tag":         plan.Tag,
			"listen":      "::",
			"listen_port": plan.Port,
			"network":     "tcp",
			"method":      shadowsocks2022Method,
			"password":    plan.Password,
			"multiplex": map[string]interface{}{
				"enabled": true,
			},
		})
	}

	for _, plan := range layout.Hysteria2 {
		inbound := map[string]interface{}{
			"type":        "hysteria2",
			"tag":         plan.Tag,
			"listen":      "::",
			"listen_port": plan.Port,
			"users":       hy2Clients,
			"tls": map[string]interface{}{
				"enabled":          true,
				"server_name":      tlsServerName,
				"certificate_path": tlsCertPath,
				"key_path":         tlsKeyPath,
			},
		}
		if plan.MasqueradeURL != "" {
			inbound["masquerade"] = map[string]interface{}{
				"type": "proxy",
				"url":  plan.MasqueradeURL,
			}
			inbound["obfs"] = map[string]interface{}{
				"type":     "salamander",
				"password": plan.ObfsPassword,
			}
		}
		inbounds = append(inbounds, inbound)
	}

	cfg := Config{
		Log: map[string]interface{}{
			"level": "info",
		},
		Inbounds: inbounds,
		Outbounds: []map[string]interface{}{
			{
				"type": "direct",
				"tag":  "direct",
			},
		},
		Route: map[string]interface{}{
			"final":                 "direct",
			"auto_detect_interface": true,
		},
	}
	if v2rayAPIListen != "" {
		cfg.Experimental = map[string]interface{}{
			"v2ray_api": map[string]interface{}{
				"listen": v2rayAPIListen,
				"stats": map[string]interface{}{
					"enabled": true,
					"users":   statsUsers,
				},
			},
		}
	}

	_ = nodeName
	_ = publicHost

	return json.MarshalIndent(cfg, "", "  ")
}
