package singbox

import "encoding/json"

type User struct {
	UUID             string `json:"uuid"`
	Email            string `json:"email"`
	Enabled          bool   `json:"enabled"`
	BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
}

type Config struct {
	Log       map[string]interface{}   `json:"log"`
	Inbounds  []map[string]interface{} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Route     map[string]interface{}   `json:"route"`
}

func Generate(nodeName, publicHost string, vlessPort, tuicPort, hy2Port int, realityPrivateKey, realityServerName, realityShortID, handshakeServer string, handshakePort int, tlsCertPath, tlsKeyPath, tlsServerName string, users []User) ([]byte, error) {
	vlessClients := make([]map[string]interface{}, 0, len(users))
	tuicClients := make([]map[string]interface{}, 0, len(users))
	hy2Clients := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		if !user.Enabled {
			continue
		}
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

	inbounds := []map[string]interface{}{
		{
			"type":        "vless",
			"tag":         "vless-in",
			"listen":      "::",
			"listen_port": vlessPort,
			"users":       vlessClients,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": realityServerName,
				"reality": map[string]interface{}{
					"enabled": true,
					"handshake": map[string]interface{}{
						"server":      handshakeServer,
						"server_port": handshakePort,
					},
					"private_key": realityPrivateKey,
					"short_id":    []string{realityShortID},
				},
			},
		},
		{
			"type":               "tuic",
			"tag":                "tuic-in",
			"listen":             "::",
			"listen_port":        tuicPort,
			"users":              tuicClients,
			"congestion_control": "bbr",
			"tls": map[string]interface{}{
				"enabled":          true,
				"server_name":      tlsServerName,
				"certificate_path": tlsCertPath,
				"key_path":         tlsKeyPath,
			},
		},
		{
			"type":        "hysteria2",
			"tag":         "hy2-in",
			"listen":      "::",
			"listen_port": hy2Port,
			"users":       hy2Clients,
			"tls": map[string]interface{}{
				"enabled":          true,
				"server_name":      tlsServerName,
				"certificate_path": tlsCertPath,
				"key_path":         tlsKeyPath,
			},
		},
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

	_ = nodeName
	_ = publicHost

	return json.MarshalIndent(cfg, "", "  ")
}
