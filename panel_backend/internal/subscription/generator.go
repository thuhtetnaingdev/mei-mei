package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"panel_backend/internal/models"
	"panel_backend/internal/services"
	"strings"
)

type NodeLink struct {
	NodeName string `json:"nodeName"`
	Protocol string `json:"protocol"`
	URL      string `json:"url"`
}

// isNodeBandwidthExceeded checks if a node has exceeded its bandwidth limit
// Returns true if bandwidth limit is set (> 0) and used bytes >= limit in bytes
func isNodeBandwidthExceeded(node models.Node) bool {
	if node.BandwidthLimitGB <= 0 {
		// No limit set (0 means unlimited)
		return false
	}
	limitBytes := node.BandwidthLimitGB * 1024 * 1024 * 1024
	return node.BandwidthUsedBytes >= limitBytes
}

// filterAvailableNodes returns only nodes that are enabled and have not exceeded their bandwidth limit.
// Testing users only receive testable nodes.
func filterAvailableNodes(user models.User, nodes []models.Node) []models.Node {
	available := make([]models.Node, 0, len(nodes))
	for _, node := range nodes {
		if user.IsTesting && !node.IsTestable {
			continue
		}
		if node.Enabled && !isNodeBandwidthExceeded(node) {
			available = append(available, node)
		}
	}
	return available
}

func Generate(user models.User, nodes []models.Node, settings services.ProtocolSettings) string {
	availableNodes := filterAvailableNodes(user, nodes)
	links := GenerateNodeLinks(user, availableNodes, settings)
	lines := make([]string, 0, len(links))
	for _, link := range links {
		lines = append(lines, link.URL)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))
}

func GenerateNodeLinks(user models.User, nodes []models.Node, settings services.ProtocolSettings) []NodeLink {
	availableNodes := filterAvailableNodes(user, nodes)
	var links []NodeLink

	for _, node := range availableNodes {
		plan := buildNodeTransportPlan(node, settings)
		for _, variant := range plan.Reality {
			vlessQuery := url.Values{}
			vlessQuery.Set("type", "tcp")
			vlessQuery.Set("encryption", "none")
			if node.RealityPublicKey != "" {
				vlessQuery.Set("security", "reality")
				vlessQuery.Set("flow", "xtls-rprx-vision")
				vlessQuery.Set("sni", variant.ServerName)
				vlessQuery.Set("pbk", node.RealityPublicKey)
				vlessQuery.Set("sid", node.RealityShortID)
				vlessQuery.Set("fp", randomizedUTLSFingerprint)
			}
			link := fmt.Sprintf(
				"vless://%s@%s:%d?%s#%s-VLESS",
				user.UUID,
				node.PublicHost,
				variant.Port,
				vlessQuery.Encode(),
				url.QueryEscape(node.Name+"-"+variant.LabelSuffix),
			)
			links = append(links, NodeLink{NodeName: node.Name, Protocol: "vless", URL: link})
		}

		if plan.TUIC.Port > 0 {
			tuicQuery := url.Values{}
			tuicQuery.Set("congestion_control", "bbr")
			tuicQuery.Set("sni", node.PublicHost)
			tuicQuery.Set("insecure", "1")
			link := fmt.Sprintf(
				"tuic://%s:%s@%s:%d?%s#%s-TUIC",
				user.UUID,
				user.UUID,
				node.PublicHost,
				plan.TUIC.Port,
				tuicQuery.Encode(),
				url.QueryEscape(node.Name),
			)
			links = append(links, NodeLink{NodeName: node.Name, Protocol: "tuic", URL: link})
		}

		shadowsocks := buildShadowsocksVariant(node, user)
		if shadowsocks.Port > 0 {
			credentials := url.QueryEscape(shadowsocks2022Method) + ":" + url.QueryEscape(shadowsocks.Password)
			link := fmt.Sprintf(
				"ss://%s@%s:%d#%s-SHADOWSOCKS",
				credentials,
				node.PublicHost,
				shadowsocks.Port,
				url.QueryEscape(node.Name),
			)
			links = append(links, NodeLink{NodeName: node.Name, Protocol: "shadowsocks", URL: link})
		}

		for _, variant := range plan.Hysteria2 {
			hy2Query := url.Values{}
			hy2Query.Set("sni", node.PublicHost)
			hy2Query.Set("insecure", "1")
			if variant.ObfsPassword != "" {
				hy2Query.Set("obfs", "salamander")
				hy2Query.Set("obfs-password", variant.ObfsPassword)
			}
			link := fmt.Sprintf(
				"hysteria2://%s@%s:%d?%s#%s-HY2",
				user.UUID,
				node.PublicHost,
				variant.Port,
				hy2Query.Encode(),
				url.QueryEscape(node.Name+"-"+variant.LabelSuffix),
			)
			links = append(links, NodeLink{NodeName: node.Name, Protocol: "hysteria2", URL: link})
		}
	}

	return links
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
