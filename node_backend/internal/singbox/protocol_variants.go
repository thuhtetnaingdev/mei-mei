package singbox

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const (
	portRangeStart = 20000
	portRangeEnd   = 59999
)

type VLESSInboundPlan struct {
	Tag        string
	ServerName string
	Port       int
}

func (p VLESSInboundPlan) GetPort() int { return p.Port }

type TUICInboundPlan struct {
	Tag  string
	Port int
}

func (p TUICInboundPlan) GetPort() int { return p.Port }

type Hysteria2InboundPlan struct {
	Tag           string
	Port          int
	MasqueradeURL string
	ObfsPassword  string
}

func (p Hysteria2InboundPlan) GetPort() int { return p.Port }

type TransportLayout struct {
	VLESS     []VLESSInboundPlan
	TUIC      TUICInboundPlan
	Hysteria2 []Hysteria2InboundPlan
}

func BuildTransportLayout(nodeName, publicHost string, realitySNIs, masquerades []string) TransportLayout {
	usedPorts := make(map[int]struct{})
	layout := TransportLayout{
		TUIC: TUICInboundPlan{
			Tag:  "tuic-in",
			Port: stableRandomPort(nodeName, publicHost, "tuic", usedPorts),
		},
	}

	serverNames := append([]string(nil), realitySNIs...)
	layout.VLESS = make([]VLESSInboundPlan, 0, len(serverNames))
	for index, serverName := range serverNames {
		layout.VLESS = append(layout.VLESS, VLESSInboundPlan{
			Tag:        fmt.Sprintf("%s-vless-in-%d", nodeName, index+1),
			ServerName: serverName,
			Port:       stableRandomPort(nodeName, publicHost, "reality:"+serverName, usedPorts),
		})
	}

	layout.Hysteria2 = make([]Hysteria2InboundPlan, 0, len(masquerades))
	for index, masquerade := range masquerades {
		layout.Hysteria2 = append(layout.Hysteria2, Hysteria2InboundPlan{
			Tag:           fmt.Sprintf("%s-hy2-in-%d", nodeName, index+1),
			Port:          stableRandomPort(nodeName, publicHost, "hy2:"+masquerade, usedPorts),
			MasqueradeURL: masquerade,
			ObfsPassword:  obfuscationPassword(nodeName, publicHost, masquerade, index),
		})
	}

	return layout
}

func obfuscationPassword(nodeName, publicHost, target string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s", nodeName, publicHost, index, target)))
	return hex.EncodeToString(sum[:12])
}

func stableRandomPort(nodeName, publicHost, key string, usedPorts map[int]struct{}) int {
	rangeSize := portRangeEnd - portRangeStart + 1
	sum := sha256.Sum256([]byte(nodeName + "|" + publicHost + "|" + key))
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
