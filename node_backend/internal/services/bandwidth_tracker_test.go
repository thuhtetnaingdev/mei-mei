package services

import (
	"testing"
)

func TestBandwidthTracker_UpdateActiveUsers(t *testing.T) {
	tracker := NewBandwidthTracker("127.0.0.1:10085")

	users := []struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}{
		{UUID: "uuid-1", Email: "user1@example.com", Enabled: true, BandwidthLimitGB: 10},
		{UUID: "uuid-2", Email: "user2@example.com", Enabled: true, BandwidthLimitGB: 20},
		{UUID: "uuid-3", Email: "user3@example.com", Enabled: false, BandwidthLimitGB: 5},
	}

	tracker.UpdateActiveUsers(users)

	// Check active users
	if len(tracker.activeUsers) != 2 {
		t.Fatalf("expected 2 active users, got %d", len(tracker.activeUsers))
	}

	// Check email to UUID mapping
	if uuid, exists := tracker.emailToUUID["user1@example.com"]; !exists || uuid != "uuid-1" {
		t.Fatalf("expected user1@example.com to map to uuid-1, got %s", uuid)
	}

	if uuid, exists := tracker.emailToUUID["user2@example.com"]; !exists || uuid != "uuid-2" {
		t.Fatalf("expected user2@example.com to map to uuid-2, got %s", uuid)
	}

	// Check disabled user is not in active users
	if _, exists := tracker.activeUsers["uuid-3"]; exists {
		t.Fatal("disabled user should not be in active users")
	}
}

func TestBandwidthTracker_UpdateActiveUsers_RemovesDisabledUsers(t *testing.T) {
	tracker := NewBandwidthTracker("127.0.0.1:10085")

	// First, add two users
	users1 := []struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}{
		{UUID: "uuid-1", Email: "user1@example.com", Enabled: true, BandwidthLimitGB: 10},
		{UUID: "uuid-2", Email: "user2@example.com", Enabled: true, BandwidthLimitGB: 20},
	}
	tracker.UpdateActiveUsers(users1)

	// Simulate some stats
	tracker.lastUserStats["user1@example.com"] = userStatsSnapshot{RX: 1000, TX: 500}
	tracker.lastUserStats["user2@example.com"] = userStatsSnapshot{RX: 2000, TX: 1000}

	// Now disable user2
	users2 := []struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}{
		{UUID: "uuid-1", Email: "user1@example.com", Enabled: true, BandwidthLimitGB: 10},
		{UUID: "uuid-2", Email: "user2@example.com", Enabled: false, BandwidthLimitGB: 20},
	}
	tracker.UpdateActiveUsers(users2)

	// Check user2 is removed from active users
	if _, exists := tracker.activeUsers["uuid-2"]; exists {
		t.Fatal("disabled user should be removed from active users")
	}

	// Check user2's stats snapshot is cleaned up
	if _, exists := tracker.lastUserStats["user2@example.com"]; exists {
		t.Fatal("disabled user's stats snapshot should be cleaned up")
	}

	// Check user1 is still active
	if _, exists := tracker.activeUsers["uuid-1"]; !exists {
		t.Fatal("enabled user should remain in active users")
	}
}

func TestBandwidthTracker_GetAndResetUsage(t *testing.T) {
	tracker := NewBandwidthTracker("127.0.0.1:10085")

	users := []struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}{
		{UUID: "uuid-1", Email: "user1@example.com", Enabled: true, BandwidthLimitGB: 10},
	}
	tracker.UpdateActiveUsers(users)

	// Simulate some usage
	tracker.currentPeriodUsage["uuid-1"].BytesUsed = 5000

	// Get and reset
	usage := tracker.GetAndResetUsage()

	if len(usage) != 1 {
		t.Fatalf("expected 1 user in usage, got %d", len(usage))
	}

	if usage[0].UUID != "uuid-1" {
		t.Fatalf("expected uuid-1, got %s", usage[0].UUID)
	}

	if usage[0].BytesUsed != 5000 {
		t.Fatalf("expected 5000 bytes, got %d", usage[0].BytesUsed)
	}

	// Check usage was reset
	if tracker.currentPeriodUsage["uuid-1"].BytesUsed != 0 {
		t.Fatalf("expected usage to be reset to 0, got %d", tracker.currentPeriodUsage["uuid-1"].BytesUsed)
	}
}

func TestBandwidthTracker_GetAndResetUsage_FiltersZeroUsage(t *testing.T) {
	tracker := NewBandwidthTracker("127.0.0.1:10085")

	users := []struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}{
		{UUID: "uuid-1", Email: "user1@example.com", Enabled: true, BandwidthLimitGB: 10},
		{UUID: "uuid-2", Email: "user2@example.com", Enabled: true, BandwidthLimitGB: 20},
	}
	tracker.UpdateActiveUsers(users)

	// Only user1 has usage
	tracker.currentPeriodUsage["uuid-1"].BytesUsed = 5000
	tracker.currentPeriodUsage["uuid-2"].BytesUsed = 0

	// Get and reset
	usage := tracker.GetAndResetUsage()

	if len(usage) != 1 {
		t.Fatalf("expected 1 user in usage (only non-zero), got %d", len(usage))
	}

	if usage[0].UUID != "uuid-1" {
		t.Fatalf("expected uuid-1, got %s", usage[0].UUID)
	}
}

func TestBandwidthTracker_GetTotalBandwidthUsed(t *testing.T) {
	tracker := NewBandwidthTracker("127.0.0.1:10085")

	users := []struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}{
		{UUID: "uuid-1", Email: "user1@example.com", Enabled: true, BandwidthLimitGB: 10},
		{UUID: "uuid-2", Email: "user2@example.com", Enabled: true, BandwidthLimitGB: 20},
	}
	tracker.UpdateActiveUsers(users)

	tracker.currentPeriodUsage["uuid-1"].BytesUsed = 3000
	tracker.currentPeriodUsage["uuid-2"].BytesUsed = 7000

	total := tracker.GetTotalBandwidthUsed()

	if total != 10000 {
		t.Fatalf("expected 10000 total bytes, got %d", total)
	}
}

func TestBandwidthTracker_GetStatus(t *testing.T) {
	tracker := NewBandwidthTracker("127.0.0.1:10085")

	users := []struct {
		UUID             string
		Email            string
		Enabled          bool
		BandwidthLimitGB int64
	}{
		{UUID: "uuid-1", Email: "user1@example.com", Enabled: true, BandwidthLimitGB: 10},
	}
	tracker.UpdateActiveUsers(users)
	tracker.currentPeriodUsage["uuid-1"].BytesUsed = 5000

	status := tracker.GetStatus()

	if status["apiAddress"] != "127.0.0.1:10085" {
		t.Fatalf("expected apiAddress to be 127.0.0.1:10085, got %v", status["apiAddress"])
	}

	if status["v2rayAPIEnabled"] != true {
		t.Fatalf("expected v2rayAPIEnabled to be true, got %v", status["v2rayAPIEnabled"])
	}

	if status["activeUsers"] != 1 {
		t.Fatalf("expected 1 active user, got %v", status["activeUsers"])
	}

	if status["periodUsageBytes"] != int64(5000) {
		t.Fatalf("expected 5000 period usage bytes, got %v", status["periodUsageBytes"])
	}
}

func TestBandwidthTracker_DisableV2RayAPI(t *testing.T) {
	tracker := NewBandwidthTracker("127.0.0.1:10085")

	if !tracker.IsV2RayAPIEnabled() {
		t.Fatal("expected v2ray API to be enabled initially")
	}

	tracker.DisableV2RayAPI()

	if tracker.IsV2RayAPIEnabled() {
		t.Fatal("expected v2ray API to be disabled after DisableV2RayAPI")
	}

	if tracker.apiAddress != "" {
		t.Fatalf("expected apiAddress to be empty after disable, got %s", tracker.apiAddress)
	}
}
