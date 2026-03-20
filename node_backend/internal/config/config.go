package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                        string
	NodeName                    string
	NodeToken                   string
	ControlPlaneSharedToken     string
	SingboxConfigPath           string
	SingboxReloadCommand        string
	NodeBinaryPath              string
	NodeRestartCommand          string
	PublicHost                  string
	VLESSRealityPrivateKey      string
	VLESSRealityPublicKey       string
	VLESSRealityShortID         string
	VLESSRealityServerName      string
	VLESSRealityHandshakeServer string
	VLESSRealityHandshakePort   int
	TLSCertificatePath          string
	TLSKeyPath                  string
	TLSServerName               string
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:                        getEnv("PORT", "9090"),
		NodeName:                    mustEnv("NODE_NAME"),
		NodeToken:                   mustEnv("NODE_TOKEN"),
		ControlPlaneSharedToken:     mustEnv("CONTROL_PLANE_SHARED_TOKEN"),
		SingboxConfigPath:           getEnv("SINGBOX_CONFIG_PATH", "./sing-box.generated.json"),
		SingboxReloadCommand:        getEnv("SINGBOX_RELOAD_COMMAND", "echo reload-sing-box"),
		NodeBinaryPath:              getEnv("NODE_BINARY_PATH", "/opt/meimei-node/node_backend"),
		NodeRestartCommand:          getEnv("NODE_RESTART_COMMAND", "systemctl restart meimei-node"),
		PublicHost:                  getEnv("PUBLIC_HOST", "node.example.com"),
		VLESSRealityPrivateKey:      getEnv("VLESS_REALITY_PRIVATE_KEY", ""),
		VLESSRealityPublicKey:       getEnv("VLESS_REALITY_PUBLIC_KEY", ""),
		VLESSRealityShortID:         getEnv("VLESS_REALITY_SHORT_ID", ""),
		VLESSRealityServerName:      getEnv("VLESS_REALITY_SERVER_NAME", "www.cloudflare.com"),
		VLESSRealityHandshakeServer: getEnv("VLESS_REALITY_HANDSHAKE_SERVER", "www.cloudflare.com"),
		VLESSRealityHandshakePort:   getEnvAsInt("VLESS_REALITY_HANDSHAKE_PORT", 443),
		TLSCertificatePath:          getEnv("TLS_CERTIFICATE_PATH", "/opt/meimei-node/tls.crt"),
		TLSKeyPath:                  getEnv("TLS_KEY_PATH", "/opt/meimei-node/tls.key"),
		TLSServerName:               getEnv("TLS_SERVER_NAME", getEnv("PUBLIC_HOST", "node.example.com")),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func mustEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return value
}

func getEnvAsInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
