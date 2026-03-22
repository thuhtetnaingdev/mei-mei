package services

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
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
	apiAddress         string
	lastSample         BandwidthSample
	currentPeriodUsage map[string]*UserBandwidthUsage
	activeUsers        map[string]bool // Set of active user UUIDs
	lastUserTotals     map[string]int64
	connectionCounts   map[string]int // UUID -> connection count
	totalConnections   int
}

var singboxUserLogPattern = regexp.MustCompile(`\[([^\]]+)\]\s+inbound connection`)

// NewBandwidthTracker creates a new bandwidth tracker
func NewBandwidthTracker(apiAddress string) *BandwidthTracker {
	return &BandwidthTracker{
		currentPeriodUsage: make(map[string]*UserBandwidthUsage),
		activeUsers:        make(map[string]bool),
		lastUserTotals:     make(map[string]int64),
		connectionCounts:   make(map[string]int),
		apiAddress:         strings.TrimSpace(apiAddress),
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

	newActiveUsers := make(map[string]bool)

	// Update with new active users
	for _, user := range users {
		if user.Enabled {
			newActiveUsers[user.UUID] = true

			// Initialize or update user usage record
			if _, exists := t.currentPeriodUsage[user.UUID]; !exists {
				t.currentPeriodUsage[user.UUID] = &UserBandwidthUsage{
					UUID:  user.UUID,
					Email: user.Email,
				}
			} else {
				if t.currentPeriodUsage[user.UUID].Email != user.Email {
					delete(t.lastUserTotals, user.UUID)
				}
				t.currentPeriodUsage[user.UUID].Email = user.Email
			}
		}
	}

	for uuid := range t.activeUsers {
		if newActiveUsers[uuid] {
			continue
		}
		delete(t.lastUserTotals, uuid)
		if usage, exists := t.currentPeriodUsage[uuid]; exists {
			usage.ActiveConns = 0
			usage.BytesUsed = 0
		}
	}

	t.activeUsers = newActiveUsers
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

	connCounts := t.getConnectionCountsFromLogsLocked()
	totalConnections := 0
	for _, count := range connCounts {
		totalConnections += count
	}

	if totalConnections == 0 {
		for _, port := range ports {
			count, err := getActiveConnections(port)
			if err != nil {
				continue
			}
			totalConnections += count
		}
		if totalConnections > 0 && len(t.activeUsers) == 1 {
			for uuid := range t.activeUsers {
				connCounts[uuid] = totalConnections
			}
		}
	}

	// Update connection counts
	t.connectionCounts = connCounts
	t.totalConnections = totalConnections

	// Update active connection counts in user usage records
	for uuid, count := range connCounts {
		if usage, exists := t.currentPeriodUsage[uuid]; exists {
			usage.ActiveConns = count
		}
	}
}

func (t *BandwidthTracker) getConnectionCountsFromLogsLocked() map[string]int {
	emailToUUID := make(map[string]string, len(t.currentPeriodUsage))
	for uuid, usage := range t.currentPeriodUsage {
		if usage == nil {
			continue
		}
		email := strings.TrimSpace(strings.ToLower(usage.Email))
		if email == "" {
			continue
		}
		emailToUUID[email] = uuid
	}
	if len(emailToUUID) == 0 {
		return make(map[string]int)
	}

	output, err := readRecentSingboxLogs()
	if err != nil || strings.TrimSpace(output) == "" {
		return make(map[string]int)
	}

	return parseConnectionCountsFromLogs(output, emailToUUID)
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
	return t.collectUsageFromInterfaces()
}

func (t *BandwidthTracker) collectUsageFromInterfaces() (int64, time.Duration, error) {
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

	// Ignore background server traffic if the node has no active proxy
	// connections at all during this sample window.
	if t.totalConnections <= 0 {
		return delta, duration, nil
	}

	t.attributeInterfaceDeltaLocked(delta)

	return delta, duration, nil
}

func (t *BandwidthTracker) attributeInterfaceDeltaLocked(delta int64) {
	if delta <= 0 {
		return
	}

	weightedUsers := make([]string, 0, len(t.activeUsers))
	totalWeight := 0
	for uuid := range t.activeUsers {
		if count := t.connectionCounts[uuid]; count > 0 {
			weightedUsers = append(weightedUsers, uuid)
			totalWeight += count
		}
	}

	if totalWeight > 0 {
		sort.Strings(weightedUsers)
		remaining := delta
		for index, uuid := range weightedUsers {
			weight := t.connectionCounts[uuid]
			if weight <= 0 {
				continue
			}

			userBytes := (delta * int64(weight)) / int64(totalWeight)
			if index == len(weightedUsers)-1 {
				userBytes = remaining
			}
			if userBytes < 0 {
				userBytes = 0
			}
			remaining -= userBytes

			if usage, exists := t.currentPeriodUsage[uuid]; exists {
				usage.BytesUsed += userBytes
			}
		}
		return
	}

	if len(t.activeUsers) != 1 {
		return
	}

	for uuid := range t.activeUsers {
		if usage, exists := t.currentPeriodUsage[uuid]; exists {
			usage.BytesUsed += delta
		}
		return
	}
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
		"apiAddress":       t.apiAddress,
		"lastSampleTime":   t.lastSample.Timestamp,
		"totalBytes":       t.lastSample.TotalBytes,
		"activeUsers":      len(t.activeUsers),
		"activeConns":      t.totalConnections,
		"trackedUsers":     len(t.currentPeriodUsage),
		"periodUsageBytes": total,
	}
}

func (t *BandwidthTracker) DisableV2RayAPI() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.apiAddress = ""
}

func readRecentSingboxLogs() (string, error) {
	cmd := exec.Command("journalctl", "-u", "meimei-sing-box", "--since", "30 seconds ago", "--no-pager", "-o", "cat")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func parseConnectionCountsFromLogs(output string, emailToUUID map[string]string) map[string]int {
	connCounts := make(map[string]int)
	for _, line := range strings.Split(output, "\n") {
		matches := singboxUserLogPattern.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}

		email := strings.TrimSpace(strings.ToLower(matches[1]))
		uuid, exists := emailToUUID[email]
		if !exists {
			continue
		}
		connCounts[uuid]++
	}

	return connCounts
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
	countTCP, tcpErr := countConnectionsForPort("tcp", port)
	countUDP, udpErr := countConnectionsForPort("udp", port)

	switch {
	case tcpErr != nil && udpErr != nil:
		return 0, tcpErr
	case tcpErr != nil:
		return countUDP, nil
	case udpErr != nil:
		return countTCP, nil
	default:
		return countTCP + countUDP, nil
	}
}

func countConnectionsForPort(network string, port int) (int, error) {
	if port <= 0 {
		return 0, nil
	}

	args := []string{"-H"}
	switch network {
	case "tcp":
		args = append(args, "-tn")
	case "udp":
		args = append(args, "-un")
	default:
		return 0, fmt.Errorf("unsupported network %q", network)
	}

	cmd := exec.Command("ss", args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return 0, nil
	}

	count := 0
	suffix := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(trimmed, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		localAddress := fields[3]
		if !strings.HasSuffix(localAddress, suffix) {
			continue
		}

		count++
	}

	return count, nil
}
