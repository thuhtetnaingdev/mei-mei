package services

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

type installedNodeMetadata struct {
	RealityPublicKey  string
	RealityShortID    string
	RealityServerName string
}

func readInstalledNodeMetadata(client *ssh.Client) (installedNodeMetadata, error) {
	output, err := runSimpleRemoteCommand(
		client,
		"sed -n '/^VLESS_REALITY_PUBLIC_KEY=/p;/^VLESS_REALITY_SHORT_ID=/p;/^VLESS_REALITY_SERVER_NAME=/p' /opt/meimei-node/.env",
	)
	if err != nil {
		return installedNodeMetadata{}, err
	}

	metadata := installedNodeMetadata{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}

		switch key {
		case "VLESS_REALITY_PUBLIC_KEY":
			metadata.RealityPublicKey = strings.TrimSpace(value)
		case "VLESS_REALITY_SHORT_ID":
			metadata.RealityShortID = strings.TrimSpace(value)
		case "VLESS_REALITY_SERVER_NAME":
			metadata.RealityServerName = strings.TrimSpace(value)
		}
	}

	if metadata.RealityPublicKey == "" || metadata.RealityShortID == "" {
		return installedNodeMetadata{}, fmt.Errorf("installed node metadata is missing reality public key or short id")
	}
	if metadata.RealityServerName == "" {
		metadata.RealityServerName = DefaultRealitySNI
	}

	return metadata, nil
}
