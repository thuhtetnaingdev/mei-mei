package subscription

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"panel_backend/internal/models"
	"panel_backend/internal/services"
	"strings"
)

const randomizedUTLSFingerprint = "randomized"

const (
	portRangeStart                = 20000
	portRangeEnd                  = 59999
	shadowsocks2022Method         = "2022-blake3-aes-128-gcm"
	shadowsocks2022KeyLengthBytes = 16
)

type realityVariant struct {
	ServerName  string
	Port        int
	LabelSuffix string
	Tag         string
}

type tuicVariant struct {
	Port int
	Tag  string
}

type shadowsocksVariant struct {
	Port     int
	Tag      string
	Password string
}

type hysteria2Variant struct {
	Port          int
	LabelSuffix   string
	Tag           string
	MasqueradeURL string
	ObfsPassword  string
}

type nodeTransportPlan struct {
	Reality   []realityVariant
	TUIC      tuicVariant
	Hysteria2 []hysteria2Variant
}

func buildNodeTransportPlan(node models.Node, settings services.ProtocolSettings) nodeTransportPlan {
	usedPorts := make(map[int]struct{})
	plan := nodeTransportPlan{
		TUIC: tuicVariant{
			Port: stableRandomPort(node, "tuic", usedPorts),
			Tag:  fmt.Sprintf("%s-tuic", node.Name),
		},
	}

	serverNames := append([]string(nil), settings.RealitySNIs...)
	plan.Reality = make([]realityVariant, 0, len(serverNames))
	for index, serverName := range serverNames {
		plan.Reality = append(plan.Reality, realityVariant{
			ServerName:  serverName,
			Port:        stableRandomPort(node, "reality:"+serverName, usedPorts),
			LabelSuffix: labelSuffixFromValue(serverName, index),
			Tag:         fmt.Sprintf("%s-vless-%d", node.Name, index+1),
		})
	}

	targets := append([]string(nil), settings.Hysteria2Masquerades...)
	plan.Hysteria2 = make([]hysteria2Variant, 0, len(targets))
	for index, target := range targets {
		plan.Hysteria2 = append(plan.Hysteria2, hysteria2Variant{
			Port:          stableRandomPort(node, "hy2:"+target, usedPorts),
			LabelSuffix:   labelSuffixFromValue(target, index),
			Tag:           fmt.Sprintf("%s-hy2-%d", node.Name, index+1),
			MasqueradeURL: target,
			ObfsPassword:  obfuscationPassword(node, target, index),
		})
	}

	return plan
}

func buildShadowsocksVariant(node models.Node, user models.User) shadowsocksVariant {
	return shadowsocksVariant{
		Port:     shadowsocksPort(node),
		Tag:      fmt.Sprintf("%s-shadowsocks", node.Name),
		Password: shadowsocksCombinedPassword(node, user.UUID),
	}
}

func shadowsocksServerPassword(node models.Node) string {
	return deriveShadowsocks2022Key("server", node.Name, node.PublicHost, "shared")
}

func shadowsocksUserPassword(node models.Node, userUUID string) string {
	return deriveShadowsocks2022Key("user", node.Name, node.PublicHost, userUUID)
}

func shadowsocksCombinedPassword(node models.Node, userUUID string) string {
	return shadowsocksServerPassword(node) + ":" + shadowsocksUserPassword(node, userUUID)
}

func deriveShadowsocks2022Key(scope, nodeName, publicHost, identity string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("ss2022|%s|%s|%s|%s", scope, nodeName, publicHost, identity)))
	return base64.StdEncoding.EncodeToString(sum[:shadowsocks2022KeyLengthBytes])
}

func obfuscationPassword(node models.Node, target string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s", node.Name, node.PublicHost, index, target)))
	return hex.EncodeToString(sum[:12])
}

func stableRandomPort(node models.Node, key string, usedPorts map[int]struct{}) int {
	rangeSize := portRangeEnd - portRangeStart + 1
	sum := sha256.Sum256([]byte(node.Name + "|" + node.PublicHost + "|" + key))
	startOffset := int(binary.BigEndian.Uint32(sum[:4]) % uint32(rangeSize))
	for attempt := 0; attempt < rangeSize; attempt++ {
		port := portRangeStart + ((startOffset + attempt) % rangeSize)
		if _, exists := usedPorts[port]; exists {
			continue
		}
		usedPorts[port] = struct{}{}
		return port
	}
	return portRangeStart
}

func shadowsocksPort(node models.Node) int {
	return stableRandomPort(node, "shadowsocks", map[int]struct{}{})
}

func labelSuffixFromValue(value string, index int) string {
	if parsed, err := url.Parse(value); err == nil {
		if parsed.Host != "" {
			return sanitizeLabel(parsed.Host)
		}
	}

	return sanitizeLabel(valueOrDefault(strings.TrimSpace(value), fmt.Sprintf("entry-%d", index+1)))
}

func sanitizeLabel(value string) string {
	replacer := strings.NewReplacer(
		":", "-",
		"/", "-",
		"\\", "-",
		"?", "-",
		"&", "-",
		"=", "-",
		"#", "-",
		"@", "-",
		" ", "-",
	)
	cleaned := strings.Trim(replacer.Replace(value), "-")
	if cleaned == "" {
		return "entry"
	}
	return cleaned
}
