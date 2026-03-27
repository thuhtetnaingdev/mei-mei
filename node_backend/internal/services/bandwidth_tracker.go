package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"node_backend/internal/singbox/stats"
)

// UserBandwidthUsage tracks bandwidth usage for a specific user
type UserBandwidthUsage struct {
	UUID        string
	Email       string
	BytesUsed   int64
	ActiveConns int
}

// BandwidthTracker monitors and tracks bandwidth usage per user using sing-box v2ray_api stats
type BandwidthTracker struct {
	mu sync.RWMutex

	// Stats API client
	statsClient *stats.StatsClient
	apiAddress  string
	v2rayAPIEnabled bool

	// User tracking
	currentPeriodUsage map[string]*UserBandwidthUsage // UUID -> usage
	activeUsers        map[string]string              // UUID -> Email mapping
	emailToUUID        map[string]string              // Email -> UUID mapping

	// Stats tracking for delta calculation
	lastUserStats map[string]userStatsSnapshot // Email -> last RX/TX snapshot

	// Rate limiting for stats queries (security hardening)
	lastQueryTime    time.Time
	minQueryInterval time.Duration // Minimum 100ms between queries to prevent DoS

	// Connection tracking (kept for API compatibility, always 0 in stats-based mode)
	totalConnections int

	// Exponential backoff for gRPC failures
	consecutiveFailures int
	lastFailureTime     time.Time
	currentBackoff      time.Duration // Current backoff duration
	maxBackoff          time.Duration // Maximum backoff (5 minutes)
}

// userStatsSnapshot holds a point-in-time snapshot of user traffic stats
type userStatsSnapshot struct {
	RX int64 // Cumulative received bytes from sing-box start
	TX int64 // Cumulative sent bytes from sing-box start
}

// NewBandwidthTracker creates a new bandwidth tracker
func NewBandwidthTracker(apiAddress string) *BandwidthTracker {
	tracker := &BandwidthTracker{
		currentPeriodUsage: make(map[string]*UserBandwidthUsage),
		activeUsers:        make(map[string]string),
		emailToUUID:        make(map[string]string),
		lastUserStats:      make(map[string]userStatsSnapshot),
		apiAddress:         apiAddress,
		v2rayAPIEnabled:    apiAddress != "",
		minQueryInterval:   100 * time.Millisecond, // Rate limit: min 100ms between queries
		// Exponential backoff initialization
		consecutiveFailures: 0,
		maxBackoff:          5 * time.Minute,
		currentBackoff:      0,
	}

	// Initialize stats client if API address is provided
	if tracker.v2rayAPIEnabled {
		tracker.statsClient = stats.NewStatsClient(apiAddress)
	}

	return tracker
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

	// Build set of new user UUIDs to check for complete user set change
	newUserUUIDs := make(map[string]bool)
	for _, user := range users {
		if user.Enabled {
			newUserUUIDs[user.UUID] = true
		}
	}

	// Detect complete user set change (fresh registration or node repurposed)
	// Reset bandwidth tracking if ALL previous users are gone and we have new users
	shouldReset := false
	if len(newUserUUIDs) > 0 && len(t.activeUsers) > 0 {
		// Check if there's ANY overlap between old and new users
		hasOverlap := false
		for uuid := range t.activeUsers {
			if newUserUUIDs[uuid] {
				hasOverlap = true
				break
			}
		}
		// No overlap = completely new user set, reset tracking
		if !hasOverlap {
			shouldReset = true
		}
	} else if len(newUserUUIDs) > 0 && len(t.activeUsers) == 0 && len(t.currentPeriodUsage) > 0 {
		// First config with users, but currentPeriodUsage has stale data
		shouldReset = true
	}

	if shouldReset {
		log.Printf("[bandwidth-tracker] resetting accumulated usage for fresh node registration (detected complete user set change)")
		t.currentPeriodUsage = make(map[string]*UserBandwidthUsage)
		t.lastUserStats = make(map[string]userStatsSnapshot)
	}

	newActiveUsers := make(map[string]string)
	newEmailToUUID := make(map[string]string)

	// Update with new active users
	for _, user := range users {
		if user.Enabled {
			newActiveUsers[user.UUID] = user.Email
			newEmailToUUID[user.Email] = user.UUID

			// Initialize or update user usage record
			if _, exists := t.currentPeriodUsage[user.UUID]; !exists {
				t.currentPeriodUsage[user.UUID] = &UserBandwidthUsage{
					UUID:  user.UUID,
					Email: user.Email,
				}
			} else {
				// Update email if changed
				if t.currentPeriodUsage[user.UUID].Email != user.Email {
					// Email changed, reset last stats to avoid incorrect delta
					delete(t.lastUserStats, t.currentPeriodUsage[user.UUID].Email)
				}
				t.currentPeriodUsage[user.UUID].Email = user.Email
			}
		}
	}

	// Handle removed users
	for uuid := range t.activeUsers {
		if _, exists := newActiveUsers[uuid]; exists {
			continue
		}
		// User was removed, clean up their stats snapshot ONLY
		if email, exists := t.activeUsers[uuid]; exists {
			delete(t.lastUserStats, email)
		}
		// DON'T delete currentPeriodUsage here - let GetAndResetUsage handle it
		// This ensures final usage is reported to panel
	}

	t.activeUsers = newActiveUsers
	t.emailToUUID = newEmailToUUID

	// Aggressive cleanup: Remove lastUserStats entries for ANY email not in new active set
	// This prevents memory leak when users are disabled without traffic or re-enabled rapidly
	for email := range t.lastUserStats {
		if _, isActive := newEmailToUUID[email]; !isActive {
			delete(t.lastUserStats, email)
		}
	}
}

// ConnectStatsClient connects to the sing-box stats API
func (t *BandwidthTracker) ConnectStatsClient() error {
	t.mu.RLock()
	client := t.statsClient
	enabled := t.v2rayAPIEnabled
	t.mu.RUnlock()

	if !enabled || client == nil {
		return fmt.Errorf("v2ray API is disabled or not configured")
	}

	return client.Connect()
}

// querySingboxStats queries sing-box stats API for per-user traffic
// Returns map[email]UserTraffic with cumulative RX/TX bytes
func (t *BandwidthTracker) querySingboxStats() (map[string]stats.UserTraffic, error) {
	t.mu.RLock()
	client := t.statsClient
	enabled := t.v2rayAPIEnabled
	t.mu.RUnlock()

	if !enabled || client == nil {
		return nil, fmt.Errorf("v2ray API is disabled or not configured")
	}

	if !client.IsConnected() {
		// Attempt to connect if not already connected
		if err := client.Connect(); err != nil {
			log.Printf("[bandwidth-tracker] stats API connection failed: %v", err)
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	traffic, err := client.GetUserTraffic(ctx, false)
	if err != nil {
		// Try reconnecting on error
		log.Printf("[bandwidth-tracker] stats query failed: %v, attempting reconnect", err)
		if reconnectErr := client.Reconnect(); reconnectErr != nil {
			log.Printf("[bandwidth-tracker] reconnect failed: %v", reconnectErr)
		}
		return nil, err
	}

	return traffic, nil
}

// CollectUsage reads stats from sing-box and accrues the delta into the current period.
// Returns total delta bytes, duration (always 0 in stats mode), and error.
func (t *BandwidthTracker) CollectUsage() (int64, time.Duration, error) {
	return t.collectUsageFromStats()
}

func (t *BandwidthTracker) collectUsageFromStats() (int64, time.Duration, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if in backoff period
	if t.consecutiveFailures > 0 {
		backoffRemaining := t.currentBackoff - time.Since(t.lastFailureTime)
		if backoffRemaining > 0 {
			log.Printf("[bandwidth-tracker] in backoff, %v remaining", backoffRemaining)
			return 0, 0, nil // Skip this collection cycle
		}
		// Backoff period expired, reset and try
		log.Printf("[bandwidth-tracker] backoff expired, attempting reconnect")
		t.consecutiveFailures = 0
		t.currentBackoff = 0
	}

	// Security: Rate limit stats queries to prevent DoS
	// Normal polling interval is 60s, so 100ms minimum is very conservative
	if time.Since(t.lastQueryTime) < t.minQueryInterval {
		return 0, 0, nil
	}
	t.lastQueryTime = time.Now()

	// Query stats from sing-box
	traffic, err := t.querySingboxStatsLocked()
	if err != nil {
		// Log but don't fail - panel will get 0 usage this period
		// Use sanitized error to prevent potential information leakage
		log.Printf("[bandwidth-tracker] collect usage failed: %s", stats.SanitizeError(err.Error()))
		return 0, 0, err
	}

	if len(traffic) == 0 {
		return 0, 0, nil
	}

	totalDelta := int64(0)

	// Calculate delta for each user and accumulate
	for email, userTraffic := range traffic {
		uuid, exists := t.emailToUUID[email]
		if !exists {
			// Traffic for unknown user, skip
			continue
		}

		// Get last snapshot for this user
		lastStats, hasLastStats := t.lastUserStats[email]

		// Calculate delta (RX + TX)
		var delta int64
		if hasLastStats {
			// Normal case: calculate delta from last snapshot
			rxDelta := userTraffic.RX - lastStats.RX
			txDelta := userTraffic.TX - lastStats.TX

			// Handle counter reset (sing-box restart) - if current < last, assume restart
			if rxDelta < 0 {
				log.Printf("[bandwidth-tracker] counter reset detected for %s (RX: %d -> %d)", email, lastStats.RX, userTraffic.RX)
				rxDelta = userTraffic.RX // Counter reset, use current value
			}
			if txDelta < 0 {
				log.Printf("[bandwidth-tracker] counter reset detected for %s (TX: %d -> %d)", email, lastStats.TX, userTraffic.TX)
				txDelta = userTraffic.TX // Counter reset, use current value
			}

			delta = rxDelta + txDelta
		} else {
			// First sample for this user - no delta to report
			// Just record the baseline
			delta = 0
		}

		// Update snapshot for next iteration
		t.lastUserStats[email] = userStatsSnapshot{
			RX: userTraffic.RX,
			TX: userTraffic.TX,
		}

		// Accumulate delta into user's period usage
		if delta > 0 && uuid != "" {
			if usage, exists := t.currentPeriodUsage[uuid]; exists {
				usage.BytesUsed += delta
				totalDelta += delta
			}
		}
	}

	// On success: reset failure counter
	t.consecutiveFailures = 0
	t.currentBackoff = 0
	return totalDelta, 0, nil
}

// querySingboxStatsLocked queries stats while holding the lock
// Must be called with t.mu already locked
func (t *BandwidthTracker) querySingboxStatsLocked() (map[string]stats.UserTraffic, error) {
	if !t.v2rayAPIEnabled || t.statsClient == nil {
		return nil, fmt.Errorf("v2ray API is disabled or not configured")
	}

	if !t.statsClient.IsConnected() {
		if err := t.statsClient.Connect(); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	traffic, err := t.statsClient.GetUserTraffic(ctx, false)
	if err != nil {
		// Use sanitized error
		log.Printf("[bandwidth-tracker] stats query failed: %s", stats.SanitizeError(err.Error()))
		
		// Increment failure counter and calculate backoff
		t.consecutiveFailures++
		
		// Binary exponential backoff: 60s, 120s, 240s, 480s, capped at maxBackoff
		baseInterval := 60 * time.Second
		backoff := baseInterval * time.Duration(1<<uint(t.consecutiveFailures-1))
		if backoff > t.maxBackoff || backoff < baseInterval { // Handle overflow
			backoff = t.maxBackoff
		}
		t.currentBackoff = backoff
		t.lastFailureTime = time.Now()
		
		log.Printf("[bandwidth-tracker] failure #%d, backing off for %v", t.consecutiveFailures, backoff)
		
		// Try reconnecting (but don't expect success during backoff)
		if reconnectErr := t.statsClient.Reconnect(); reconnectErr != nil {
			log.Printf("[bandwidth-tracker] reconnect failed: %s", stats.SanitizeError(reconnectErr.Error()))
		}
		return nil, err
	}

	return traffic, nil
}

// SampleBandwidth is kept for API compatibility but returns zero values in stats-based mode
func (t *BandwidthTracker) SampleBandwidth() (BandwidthSample, error) {
	return BandwidthSample{}, fmt.Errorf("stats-based mode: use CollectUsage instead")
}

// UpdateConnectionCounts is a no-op in stats-based mode (kept for API compatibility)
func (t *BandwidthTracker) UpdateConnectionCounts(ports ...int) {
	// Stats-based mode doesn't need connection counting
	// Traffic is directly attributed to users via sing-box stats
	t.mu.Lock()
	t.totalConnections = 0
	t.mu.Unlock()
}

// GetAndResetUsage returns the current period usage and resets for the next period
func (t *BandwidthTracker) GetAndResetUsage() []UserBandwidthUsage {
	t.mu.Lock()
	defer t.mu.Unlock()

	usage := make([]UserBandwidthUsage, 0, len(t.currentPeriodUsage))
	toDelete := make([]string, 0)

	for uuid, userUsage := range t.currentPeriodUsage {
		// Capture usage for reporting
		if userUsage.BytesUsed > 0 {
			usage = append(usage, UserBandwidthUsage{
				UUID:        userUsage.UUID,
				Email:       userUsage.Email,
				BytesUsed:   userUsage.BytesUsed,
				ActiveConns: 0, // Stats-based mode doesn't track connections
			})
		}

		// Reset for next period
		userUsage.BytesUsed = 0
		userUsage.ActiveConns = 0

		// Mark inactive users for deletion (after reporting)
		if _, isActive := t.activeUsers[uuid]; !isActive {
			toDelete = append(toDelete, uuid)
		}
	}

	// Delete inactive users AFTER capturing their usage
	for _, uuid := range toDelete {
		delete(t.currentPeriodUsage, uuid)
		// Also clean up emailToUUID mapping
		// Find email for this UUID
		for email, u := range t.emailToUUID {
			if u == uuid {
				delete(t.emailToUUID, email)
				break
			}
		}
	}

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

	status := map[string]interface{}{
		"apiAddress":       t.apiAddress,
		"v2rayAPIEnabled":  t.v2rayAPIEnabled,
		"activeUsers":      len(t.activeUsers),
		"trackedUsers":     len(t.currentPeriodUsage),
		"periodUsageBytes": total,
	}

	if t.statsClient != nil {
		status["statsConnected"] = t.statsClient.IsConnected()
	} else {
		status["statsConnected"] = false
	}

	return status
}

// DisableV2RayAPI disables the v2ray API and falls back to disabled mode
func (t *BandwidthTracker) DisableV2RayAPI() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.v2rayAPIEnabled = false
	t.apiAddress = ""

	if t.statsClient != nil {
		t.statsClient.Close()
		t.statsClient = nil
	}

	log.Printf("[bandwidth-tracker] v2ray API disabled")
}

// IsV2RayAPIEnabled returns whether v2ray API is enabled
func (t *BandwidthTracker) IsV2RayAPIEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.v2rayAPIEnabled
}

// BandwidthSample represents a point-in-time bandwidth measurement
// Kept for backward compatibility but not used in stats-based mode
type BandwidthSample struct {
	Timestamp  time.Time
	TotalBytes int64
	RXBytes    int64
	TXBytes    int64
}
