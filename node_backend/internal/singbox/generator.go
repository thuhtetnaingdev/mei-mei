package singbox

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

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
	DNS          map[string]interface{}   `json:"dns,omitempty"`
}

func Generate(
	nodeName, publicHost string,
	realityPrivateKey, realityServerName, realityShortID, handshakeServer string,
	handshakePort int,
	tlsCertPath, tlsKeyPath, tlsServerName, v2rayAPIListen string,
	realitySNIs, hysteria2Masquerades []string,
	users []User,
	dnsServers, dnsStrategy string,
	dnsDisableCache, dnsDisableExpire, dnsIndependentCache, dnsReverseMapping bool,
) ([]byte, error) {
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
		ssUsers := make([]map[string]interface{}, 0, len(plan.Users))
		for _, user := range plan.Users {
			ssUsers = append(ssUsers, map[string]interface{}{
				"name":     user.Name,
				"password": user.Password,
			})
		}
		inbounds = append(inbounds, map[string]interface{}{
			"type":        "shadowsocks",
			"tag":         plan.Tag,
			"listen":      "::",
			"listen_port": plan.Port,
			"network":     "tcp",
			"method":      shadowsocks2022Method,
			"password":    plan.ServerPassword,
			"users":       ssUsers,
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
		DNS: buildDNSConfig(dnsServers, dnsStrategy, dnsDisableCache, dnsDisableExpire, dnsIndependentCache, dnsReverseMapping),
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

func buildDNSConfig(servers string, strategy string, disableCache, disableExpire, independentCache, reverseMapping bool) map[string]interface{} {
	if strings.TrimSpace(servers) == "" {
		return nil
	}

	serverList := strings.Split(servers, ",")
	dnsServers := make([]map[string]interface{}, 0, len(serverList))

	for i, srv := range serverList {
		srv = strings.TrimSpace(srv)
		if srv == "" {
			continue
		}

		tag := fmt.Sprintf("dns%d", i+1)
		dnsServers = append(dnsServers, map[string]interface{}{
			"tag":         tag,
			"type":        "udp",
			"server":      srv,
			"server_port": 53,
		})
	}

	if len(dnsServers) == 0 {
		return nil
	}

	dns := map[string]interface{}{
		"servers": dnsServers,
		"final":   "dns1",
	}

	if strategy != "" {
		dns["strategy"] = strategy
	}
	if disableCache {
		dns["disable_cache"] = true
	}
	if disableExpire {
		dns["disable_expire"] = true
	}
	if independentCache {
		dns["independent_cache"] = true
	}
	if reverseMapping {
		dns["reverse_mapping"] = true
	}

	return dns
}

func parseDNSServerAddress(address string, index int) map[string]interface{} {
	address = strings.TrimSpace(address)
	tag := fmt.Sprintf("dns%d", index)

	if strings.HasPrefix(address, "tcp://") {
		return map[string]interface{}{
			"tag":     tag,
			"type":    "tcp",
			"address": address,
		}
	}

	if strings.HasPrefix(address, "tls://") {
		return map[string]interface{}{
			"tag":     tag,
			"type":    "tls",
			"address": address,
		}
	}

	if strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "h3://") {
		return map[string]interface{}{
			"tag":     tag,
			"type":    "https",
			"address": address,
		}
	}

	if strings.HasPrefix(address, "quic://") {
		return map[string]interface{}{
			"tag":     tag,
			"type":    "quic",
			"address": address,
		}
	}

	if strings.HasPrefix(address, "udp://") {
		address = strings.TrimPrefix(address, "udp://")
	}

	host, portStr, err := splitHostPort(address)
	if err != nil {
		return map[string]interface{}{
			"tag":    tag,
			"type":   "udp",
			"server": address,
			"port":   53,
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 53
	}

	return map[string]interface{}{
		"tag":    tag,
		"type":   "udp",
		"server": host,
		"port":   port,
	}
}

func splitHostPort(address string) (string, string, error) {
	lastColon := strings.LastIndex(address, ":")
	if lastColon < 0 {
		return address, "53", nil
	}

	host := address[:lastColon]
	port := address[lastColon+1:]

	if port == "" {
		return host, "53", nil
	}

	return host, port, nil
}
