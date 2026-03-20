package services

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BandwidthSample represents a point-in-time bandwidth measurement
type BandwidthSample struct {
	Timestamp  time.Time
	TotalBytes int64
	RXBytes    int64
	TXBytes    int64
}

// UserBandwidthUsage tracks bandwidth usage for a specific user
type UserBandwidthUsage struct {
	UUID        string
	Email       string
	BytesUsed   int64
	ActiveConns int
}

// BandwidthTracker monitors and tracks bandwidth usage per user
type BandwidthTracker struct {
	mu                 sync.RWMutex
	lastSample         BandwidthSample
	currentPeriodUsage map[string]*UserBandwidthUsage
	activeUsers        map[string]bool // Set of active user UUIDs
	connectionCounts   map[string]int  // UUID -> connection count
}

// NewBandwidthTracker creates a new bandwidth tracker
func NewBandwidthTracker() *BandwidthTracker {
	return &BandwidthTracker{
		currentPeriodUsage: make(map[string]*UserBandwidthUsage),
		activeUsers:        make(map[string]bool),
		connectionCounts:   make(map[string]int),
	}
}

// UpdateActiveUsers updates the list of active users for bandwidth attribution
func (t *BandwidthTracker) UpdateActiveUsers(users []struct {
	UUID             string
	Email            string
	Enabled          bool
	BandwidthLimitGB int64
}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clear old active users
	t.activeUsers = make(map[string]bool)

	// Update with new active users
	for _, user := range users {
		if user.Enabled {
			t.activeUsers[user.UUID] = true

			// Initialize or update user usage record
			if _, exists := t.currentPeriodUsage[user.UUID]; !exists {
				t.currentPeriodUsage[user.UUID] = &UserBandwidthUsage{
					UUID:  user.UUID,
					Email: user.Email,
				}
			} else {
				t.currentPeriodUsage[user.UUID].Email = user.Email
			}
		}
	}
}

func readBandwidthSample() (BandwidthSample, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return BandwidthSample{}, fmt.Errorf("failed to open /proc/net/dev: %w", err)
	}
	defer file.Close()

	var totalRX, totalTX int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		iface := strings.TrimSpace(parts[0])
		// Skip loopback interface
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		rxBytes, rxErr := strconv.ParseInt(fields[0], 10, 64)
		txBytes, txErr := strconv.ParseInt(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}

		totalRX += rxBytes
		totalTX += txBytes
	}

	if err := scanner.Err(); err != nil {
		return BandwidthSample{}, fmt.Errorf("error reading /proc/net/dev: %w", err)
	}

	sample := BandwidthSample{
		Timestamp:  time.Now(),
		TotalBytes: totalRX + totalTX,
		RXBytes:    totalRX,
		TXBytes:    totalTX,
	}

	return sample, nil
}

// SampleBandwidth takes a new bandwidth sample from system interfaces.
func (t *BandwidthTracker) SampleBandwidth() (BandwidthSample, error) {
	sample, err := readBandwidthSample()
	if err != nil {
		return BandwidthSample{}, err
	}

	t.mu.Lock()
	t.lastSample = sample
	t.mu.Unlock()

	return sample, nil
}

// UpdateConnectionCounts updates the connection count per user by parsing sing-box connections
func (t *BandwidthTracker) UpdateConnectionCounts(ports ...int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Get connection counts from sing-box ports
	connCounts := getConnectionCountsByPort(ports...)

	// Update connection counts
	t.connectionCounts = connCounts

	// Update active connection counts in user usage records
	for uuid, count := range connCounts {
		if usage, exists := t.currentPeriodUsage[uuid]; exists {
			usage.ActiveConns = count
		}
	}
}

// getConnectionCountsByPort attempts to get connection counts from sing-box
// Since sing-box doesn't expose per-user connections directly, we use connection tracking
func getConnectionCountsByPort(ports ...int) map[string]int {
	// This is a placeholder - in production, you would:
	// 1. Use eBPF to track connections per UUID
	// 2. Parse sing-box access logs if enabled
	// 3. Use netstat/ss to count connections per port and distribute

	// For now, return empty map - bandwidth will be distributed equally among active users
	return make(map[string]int)
}

// CollectUsage reads a new sample and accrues the delta into the current period.
func (t *BandwidthTracker) CollectUsage() (int64, time.Duration, error) {
	currentSample, err := readBandwidthSample()
	if err != nil {
		return 0, 0, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.lastSample.Timestamp.IsZero() {
		t.lastSample = currentSample
		return 0, 0, nil
	}

	delta := currentSample.TotalBytes - t.lastSample.TotalBytes
	duration := currentSample.Timestamp.Sub(t.lastSample.Timestamp)
	t.lastSample = currentSample

	if delta <= 0 || len(t.activeUsers) == 0 {
		if delta < 0 {
			delta = 0
		}
		return delta, duration, nil
	}

	// Calculate total weight based on active connections.
	totalWeight := 0
	for uuid := range t.activeUsers {
		if count, exists := t.connectionCounts[uuid]; exists && count > 0 {
			totalWeight += count
		} else {
			totalWeight++
		}
	}

	if totalWeight == 0 {
		totalWeight = len(t.activeUsers)
	}

	for uuid := range t.activeUsers {
		weight := 1
		if count, exists := t.connectionCounts[uuid]; exists && count > 0 {
			weight = count
		}

		userBytes := (delta * int64(weight)) / int64(totalWeight)
		if usage, exists := t.currentPeriodUsage[uuid]; exists {
			usage.BytesUsed += userBytes
		}
	}

	return delta, duration, nil
}

// GetAndResetUsage returns the current period usage and resets for the next period
func (t *BandwidthTracker) GetAndResetUsage() []UserBandwidthUsage {
	t.mu.Lock()
	defer t.mu.Unlock()

	usage := make([]UserBandwidthUsage, 0, len(t.currentPeriodUsage))
	for _, userUsage := range t.currentPeriodUsage {
		if userUsage.BytesUsed > 0 {
			usage = append(usage, *userUsage)
		}
		// Reset bytes for next period but keep the record
		userUsage.BytesUsed = 0
		userUsage.ActiveConns = 0
	}

	// Reset connection counts
	t.connectionCounts = make(map[string]int)

	return usage
}

// GetTotalBandwidthUsed returns total bandwidth used in current period
func (t *BandwidthTracker) GetTotalBandwidthUsed() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total int64
	for _, usage := range t.currentPeriodUsage {
		total += usage.BytesUsed
	}
	return total
}

// GetStatus returns current tracker status
func (t *BandwidthTracker) GetStatus() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total int64
	for _, usage := range t.currentPeriodUsage {
		total += usage.BytesUsed
	}

	return map[string]interface{}{
		"lastSampleTime":   t.lastSample.Timestamp,
		"totalBytes":       t.lastSample.TotalBytes,
		"activeUsers":      len(t.activeUsers),
		"trackedUsers":     len(t.currentPeriodUsage),
		"periodUsageBytes": total,
	}
}

// readBandwidthUsageBytes reads total bandwidth from /proc/net/dev
// This is the legacy function kept for compatibility
func readBandwidthUsageBytes() int64 {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0
	}
	defer file.Close()

	var total int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		rxBytes, rxErr := strconv.ParseInt(fields[0], 10, 64)
		txBytes, txErr := strconv.ParseInt(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}

		total += rxBytes + txBytes
	}

	return total
}

// parseSSOutput parses ss command output to extract connection information
func parseSSOutput(output string) map[string]int {
	connCounts := make(map[string]int)
	lines := strings.Split(output, "\n")

	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// Look for UUID patterns in the connection info
		// sing-box uses UUID in various formats in connection details
		for _, field := range fields {
			if isUUID(field) {
				connCounts[field]++
			}
		}
	}

	return connCounts
}

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	// Remove common prefixes/suffixes
	s = strings.TrimPrefix(s, "uuid:")
	s = strings.TrimSuffix(s, ",")

	// Standard UUID format: 8-4-4-4-12 hex characters
	if len(s) != 36 {
		return false
	}

	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}

	expected := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expected[i] {
			return false
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}

	return true
}

// getActiveConnections attempts to get active connections using ss command
func getActiveConnections(port int) (int, error) {
	cmd := exec.Command("ss", "-tn", fmt.Sprintf("dport = :%d", port))
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(output), "\n")
	// Subtract 1 for header line
	count := len(lines) - 1
	if count < 0 {
		count = 0
	}

	return count, nil
}
