package stats

import (
	"context"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Security Assumptions:
// - sing-box instance is trusted and runs on the same host
// - localhost network namespace is secure (not shared with untrusted containers)
// - v2ray_api endpoint must NOT be exposed to external networks
// - File permissions on sing-box binary and config should restrict access

// Security constants for input validation
const (
	MaxStatNameLength = 512  // Maximum stat name length to prevent DoS
	MaxEmailLength    = 254  // RFC 5321 maximum email length
	MaxErrorLength    = 200  // Maximum error message length to prevent DoS
)

// sanitizeError removes sensitive information from error messages
// - Removes file paths (replaces with [REDACTED])
// - Removes potential stack traces
// - Limits error message length to MaxErrorLength
//
// Exported as SanitizeError for use by other packages
func sanitizeError(errMsg string) string {
	// Remove file paths (e.g., /path/to/file.go:123 or C:\path\to\file.go:123)
	pathRegex := regexp.MustCompile(`[a-zA-Z]:\\[^:\s]*\.[a-zA-Z]+:\d+|/[\w./]+/[\w.]+:\d+`)
	sanitized := pathRegex.ReplaceAllString(errMsg, "[REDACTED]")

	// Remove stack trace patterns (goroutine numbers, hex addresses, etc.)
	stackTraceRegex := regexp.MustCompile(`(?m)^goroutine\s+\d+\s+\[.*\]$|0x[0-9a-fA-F]+`)
	sanitized = stackTraceRegex.ReplaceAllString(sanitized, "")

	// Remove multiple consecutive newlines (potential stack trace indicator)
	newlineRegex := regexp.MustCompile(`\n{2,}`)
	sanitized = newlineRegex.ReplaceAllString(sanitized, " ")

	// Trim whitespace and limit length
	sanitized = strings.TrimSpace(sanitized)
	if len(sanitized) > MaxErrorLength {
		sanitized = sanitized[:MaxErrorLength] + "..."
	}

	return sanitized
}

// SanitizeError removes sensitive information from error messages.
// Exported wrapper for internal sanitizeError function.
func SanitizeError(errMsg string) string {
	return sanitizeError(errMsg)
}

// UserTraffic represents traffic statistics for a single user
type UserTraffic struct {
	Email string
	UUID  string
	RX    int64 // Received bytes (cumulative since sing-box start)
	TX    int64 // Sent bytes (cumulative since sing-box start)
}

// StatsClient provides gRPC client for sing-box v2ray_api stats service
type StatsClient struct {
	mu           sync.RWMutex
	conn         *grpc.ClientConn
	statsService StatsServiceClient
	address      string
	connected    bool
}

// StatsServiceClient is the gRPC client interface for stats service
type StatsServiceClient interface {
	GetStats(ctx context.Context, in *GetStatsRequest, opts ...grpc.CallOption) (*GetStatsResponse, error)
	QueryStats(ctx context.Context, in *QueryStatsRequest, opts ...grpc.CallOption) (*QueryStatsResponse, error)
}

// GetStatsRequest requests stats for a specific pattern
type GetStatsRequest struct {
	Name   string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Reset_ bool   `protobuf:"varint,2,opt,name=reset,proto3" json:"reset,omitempty"`
}

// GetStatsResponse contains stats for a single pattern
type GetStatsResponse struct {
	Stat *Stat `protobuf:"bytes,1,opt,name=stat,proto3" json:"stat,omitempty"`
}

// Stat represents a single statistic value
type Stat struct {
	Name  string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Value int64  `protobuf:"varint,2,opt,name=value,proto3" json:"value,omitempty"`
}

// QueryStatsRequest requests stats matching patterns
type QueryStatsRequest struct {
	Patterns []string `protobuf:"bytes,1,rep,name=patterns,proto3" json:"patterns,omitempty"`
	Reset_   bool     `protobuf:"varint,2,opt,name=reset,proto3" json:"reset,omitempty"`
}

// QueryStatsResponse contains multiple stats
type QueryStatsResponse struct {
	Stat []*Stat `protobuf:"bytes,1,rep,name=stat,proto3" json:"stat,omitempty"`
}

// statsServiceClient implements StatsServiceClient
type statsServiceClient struct {
	cc grpc.ClientConnInterface
}

// NewStatsServiceClient creates a new stats service client
func NewStatsServiceClient(cc grpc.ClientConnInterface) StatsServiceClient {
	return &statsServiceClient{cc: cc}
}

// GetStats fetches a single stat by name
func (c *statsServiceClient) GetStats(ctx context.Context, in *GetStatsRequest, opts ...grpc.CallOption) (*GetStatsResponse, error) {
	out := new(GetStatsResponse)
	err := c.cc.Invoke(ctx, "/com.github.xtls.xray_core.app.stats.StatsService/GetStats", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// QueryStats fetches multiple stats by pattern
func (c *statsServiceClient) QueryStats(ctx context.Context, in *QueryStatsRequest, opts ...grpc.CallOption) (*QueryStatsResponse, error) {
	out := new(QueryStatsResponse)
	err := c.cc.Invoke(ctx, "/com.github.xtls.xray_core.app.stats.StatsService/QueryStats", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// NewStatsClient creates a new stats client connected to the specified address
func NewStatsClient(address string) *StatsClient {
	return &StatsClient{
		address: address,
	}
}

// Connect establishes gRPC connection to sing-box stats API
func (c *StatsClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected && c.conn != nil {
		return nil
	}

	// Security validation: ensure only localhost addresses are accepted
	if err := c.validateLocalhostAddress(); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		c.address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to stats API at %s: %w", c.address, err)
	}

	c.conn = conn
	c.statsService = NewStatsServiceClient(conn)
	c.connected = true

	log.Printf("[stats-client] connected to sing-box stats API at %s", c.address)
	return nil
}

// validateLocalhostAddress validates that the address is localhost-only
func (c *StatsClient) validateLocalhostAddress() error {
	// Parse the address to extract host
	host, _, err := net.SplitHostPort(c.address)
	if err != nil {
		// If SplitHostPort fails, the address might not have a port
		// Try to parse as-is (could be a Unix socket or bare address)
		host = c.address
	}

	// If host is empty, it means localhost (default binding)
	if host == "" {
		log.Printf("[stats-client] security validation passed: localhost address (empty host)")
		return nil
	}

	// Normalize IPv6 addresses
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}

	// Check if host is a localhost address
	localhostAddresses := map[string]bool{
		"127.0.0.1": true,
		"localhost": true,
		"::1":       true,
	}

	if localhostAddresses[host] {
		log.Printf("[stats-client] security validation passed: localhost address %s", host)
		return nil
	}

	return fmt.Errorf("non-localhost address rejected: %s (only 127.0.0.1, localhost, or ::1 allowed)", c.address)
}

// Close closes the gRPC connection
func (c *StatsClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.connected = false
		c.statsService = nil
		return err
	}
	return nil
}

// IsConnected returns whether the client is connected
func (c *StatsClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetUserTraffic fetches traffic statistics for all users
// Returns map[email]UserTraffic with cumulative RX/TX bytes
func (c *StatsClient) GetUserTraffic(ctx context.Context, reset bool) (map[string]UserTraffic, error) {
	c.mu.RLock()
	conn := c.statsService
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("stats client not connected")
	}

	// Query all user traffic stats
	// sing-box uses pattern: "user>>>email>>>traffic>>>downlink" and "user>>>email>>>traffic>>>uplink"
	resp, err := conn.QueryStats(ctx, &QueryStatsRequest{
		Patterns: []string{"user>>>"},
		Reset_:   reset,
	})
	if err != nil {
		return nil, fmt.Errorf("query stats failed: %w", err)
	}

	// Parse stats into per-user traffic
	traffic := make(map[string]UserTraffic)
	for _, stat := range resp.Stat {
		if stat == nil {
			continue
		}

		// Parse stat name: "user>>>email>>>traffic>>>downlink" or "uplink"
		email, direction := parseStatName(stat.Name)
		if email == "" {
			continue
		}

		user := traffic[email]
		user.Email = email

		switch direction {
		case "downlink":
			user.RX = stat.Value
		case "uplink":
			user.TX = stat.Value
		}

		traffic[email] = user
	}

	return traffic, nil
}

// parseStatName extracts email and direction from stat name
// Format: "user>>>email>>>traffic>>>downlink" or "user>>>email>>>traffic>>>uplink"
func parseStatName(name string) (email, direction string) {
	// Security validation: check stat name length to prevent DoS
	if len(name) > MaxStatNameLength {
		log.Printf("[stats-client] validation failed: stat name exceeds maximum length (%d > %d)", len(name), MaxStatNameLength)
		return "", ""
	}

	// Expected format: user>>>email>>>traffic>>>downlink
	// Split by >>>
	parts := splitByTripleGreater(name)
	if len(parts) < 4 {
		return "", ""
	}

	if parts[0] != "user" || parts[2] != "traffic" {
		return "", ""
	}

	email = parts[1]
	direction = parts[3]

	// Security validation: check email length (RFC 5321)
	if email == "" || len(email) > MaxEmailLength {
		log.Printf("[stats-client] validation failed: invalid email length (empty or > %d chars)", MaxEmailLength)
		return "", ""
	}

	// Security validation: strict direction validation
	// Only accept exactly "uplink" or "downlink"
	if direction != "uplink" && direction != "downlink" {
		log.Printf("[stats-client] validation failed: invalid direction (must be 'uplink' or 'downlink')")
		return "", ""
	}

	return email, direction
}

// splitByTripleGreater splits string by ">>>" delimiter
func splitByTripleGreater(s string) []string {
	var parts []string
	current := ""
	i := 0
	for i < len(s) {
		if i+3 <= len(s) && s[i:i+3] == ">>>" {
			parts = append(parts, current)
			current = ""
			i += 3
			continue
		}
		current += string(s[i])
		i++
	}
	parts = append(parts, current)
	return parts
}

// Reconnect attempts to reconnect to the stats API
func (c *StatsClient) Reconnect() error {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.connected = false
		c.statsService = nil
	}
	c.mu.Unlock()

	return c.Connect()
}
