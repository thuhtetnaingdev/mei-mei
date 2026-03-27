package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                        string
	NodeName                    string
	NodeToken                   string
	ControlPlaneSharedToken     string
	SingboxConfigPath           string
	SingboxV2RayAPIListen       string
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
	DNSServers                  string
	DNSStrategy                 string
	DNSDisableCache             bool
	DNSDisableExpire            bool
	DNSIndependentCache         bool
	DNSReverseMapping           bool
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:                        getEnv("PORT", "9090"),
		NodeName:                    mustEnv("NODE_NAME"),
		NodeToken:                   mustEnv("NODE_TOKEN"),
		ControlPlaneSharedToken:     mustEnv("CONTROL_PLANE_SHARED_TOKEN"),
		SingboxConfigPath:           getEnv("SINGBOX_CONFIG_PATH", "./sing-box.generated.json"),
		SingboxV2RayAPIListen:       getEnv("SINGBOX_V2RAY_API_LISTEN", "127.0.0.1:10085"),
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
		DNSServers:                  getEnv("DNS_SERVERS", "8.8.8.8,1.1.1.1"),
		DNSStrategy:                 getEnv("DNS_STRATEGY", "prefer_ipv4"),
		DNSDisableCache:             getEnvAsBool("DNS_DISABLE_CACHE", false),
		DNSDisableExpire:            getEnvAsBool("DNS_DISABLE_EXPIRE", false),
		DNSIndependentCache:         getEnvAsBool("DNS_INDEPENDENT_CACHE", false),
		DNSReverseMapping:           getEnvAsBool("DNS_REVERSE_MAPPING", false),
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

func getEnvAsBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return b
}

// normalizeRealityPrivateKey converts base64url to standard base64 for sing-box config
// sing-box Reality expects private_key in standard base64 format
func normalizeRealityPrivateKey(key string) string {
	if key == "" {
		return ""
	}
	// Convert base64url to standard base64
	b64 := strings.ReplaceAll(key, "-", "+")
	b64 = strings.ReplaceAll(b64, "_", "/")
	// Add padding if needed
	if mod := len(b64) % 4; mod == 2 {
		b64 += "=="
	} else if mod == 3 {
		b64 += "="
	}
	return b64
}

// GetRealityPrivateKeyForSingBox returns the private key in standard base64 format for sing-box config
func (c *Config) GetRealityPrivateKeyForSingBox() string {
	return normalizeRealityPrivateKey(c.VLESSRealityPrivateKey)
}
