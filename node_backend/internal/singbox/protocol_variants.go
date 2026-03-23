package singbox

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const (
	portRangeStart                = 20000
	portRangeEnd                  = 59999
	shadowsocks2022Method         = "2022-blake3-aes-128-gcm"
	shadowsocks2022KeyLengthBytes = 16
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

type ShadowsocksInboundPlan struct {
	Tag            string
	Port           int
	ServerPassword string
	Users          []ShadowsocksInboundUser
}

func (p ShadowsocksInboundPlan) GetPort() int { return p.Port }

type ShadowsocksInboundUser struct {
	Name     string
	Password string
}

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

func BuildShadowsocksInboundPlans(nodeName, publicHost string, users []User) []ShadowsocksInboundPlan {
	plan := ShadowsocksInboundPlan{
		Tag:            "shadowsocks-in",
		Port:           ShadowsocksPort(nodeName, publicHost),
		ServerPassword: ShadowsocksServerPassword(nodeName, publicHost),
		Users:          make([]ShadowsocksInboundUser, 0, len(users)),
	}
	for _, user := range users {
		if !user.Enabled {
			continue
		}

		plan.Users = append(plan.Users, ShadowsocksInboundUser{
			Name:     user.Email,
			Password: ShadowsocksUserPassword(nodeName, publicHost, user.UUID),
		})
	}

	if len(plan.Users) == 0 {
		return nil
	}
	return []ShadowsocksInboundPlan{plan}
}

func ShadowsocksServerPassword(nodeName, publicHost string) string {
	return deriveShadowsocks2022Key("server", nodeName, publicHost, "shared")
}

func ShadowsocksPort(nodeName, publicHost string) int {
	return stableRandomPort(nodeName, publicHost, "shadowsocks", map[int]struct{}{})
}

func ShadowsocksUserPassword(nodeName, publicHost, userUUID string) string {
	return deriveShadowsocks2022Key("user", nodeName, publicHost, userUUID)
}

func deriveShadowsocks2022Key(scope, nodeName, publicHost, identity string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("ss2022|%s|%s|%s|%s", scope, nodeName, publicHost, identity)))
	return base64.StdEncoding.EncodeToString(sum[:shadowsocks2022KeyLengthBytes])
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
