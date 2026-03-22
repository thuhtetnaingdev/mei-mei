package services

import "testing"

func TestAttributeInterfaceDeltaLockedDoesNotSplitUnknownTrafficAcrossMultipleUsers(t *testing.T) {
	tracker := NewBandwidthTracker("")
	tracker.activeUsers["user-1"] = true
	tracker.activeUsers["user-2"] = true
	tracker.currentPeriodUsage["user-1"] = &UserBandwidthUsage{UUID: "user-1", Email: "one@example.com"}
	tracker.currentPeriodUsage["user-2"] = &UserBandwidthUsage{UUID: "user-2", Email: "two@example.com"}

	tracker.attributeInterfaceDeltaLocked(1024)

	if tracker.currentPeriodUsage["user-1"].BytesUsed != 0 {
		t.Fatalf("expected user-1 usage to stay 0, got %d", tracker.currentPeriodUsage["user-1"].BytesUsed)
	}
	if tracker.currentPeriodUsage["user-2"].BytesUsed != 0 {
		t.Fatalf("expected user-2 usage to stay 0, got %d", tracker.currentPeriodUsage["user-2"].BytesUsed)
	}
}

func TestAttributeInterfaceDeltaLockedAssignsAllTrafficToSingleActiveUser(t *testing.T) {
	tracker := NewBandwidthTracker("")
	tracker.activeUsers["user-1"] = true
	tracker.currentPeriodUsage["user-1"] = &UserBandwidthUsage{UUID: "user-1", Email: "one@example.com"}

	tracker.attributeInterfaceDeltaLocked(2048)

	if tracker.currentPeriodUsage["user-1"].BytesUsed != 2048 {
		t.Fatalf("expected single active user to receive 2048 bytes, got %d", tracker.currentPeriodUsage["user-1"].BytesUsed)
	}
}

func TestAttributeInterfaceDeltaLockedUsesObservedConnectionWeights(t *testing.T) {
	tracker := NewBandwidthTracker("")
	tracker.activeUsers["user-1"] = true
	tracker.activeUsers["user-2"] = true
	tracker.currentPeriodUsage["user-1"] = &UserBandwidthUsage{UUID: "user-1", Email: "one@example.com"}
	tracker.currentPeriodUsage["user-2"] = &UserBandwidthUsage{UUID: "user-2", Email: "two@example.com"}
	tracker.connectionCounts["user-1"] = 3
	tracker.connectionCounts["user-2"] = 1

	tracker.attributeInterfaceDeltaLocked(400)

	if tracker.currentPeriodUsage["user-1"].BytesUsed != 300 {
		t.Fatalf("expected user-1 to receive 300 bytes, got %d", tracker.currentPeriodUsage["user-1"].BytesUsed)
	}
	if tracker.currentPeriodUsage["user-2"].BytesUsed != 100 {
		t.Fatalf("expected user-2 to receive 100 bytes, got %d", tracker.currentPeriodUsage["user-2"].BytesUsed)
	}
}

func TestParseConnectionCountsFromLogsMapsEmailToUUID(t *testing.T) {
	logs := `
INFO inbound/vless[wd-vless-in-1]: [thuhtet01.naing@gmail.com] inbound connection to static.xx.fbcdn.net:443
INFO inbound/vless[wd-vless-in-1]: [thuhtet01.naing@gmail.com] inbound connection to edge-chat.facebook.com:443
INFO inbound/vless[wd-vless-in-1]: [demoinerl2h@gmail.com] inbound connection to example.com:443
`

	counts := parseConnectionCountsFromLogs(logs, map[string]string{
		"thuhtet01.naing@gmail.com": "uuid-1",
		"demoinerl2h@gmail.com":     "uuid-2",
	})

	if counts["uuid-1"] != 2 {
		t.Fatalf("expected uuid-1 count to be 2, got %d", counts["uuid-1"])
	}
	if counts["uuid-2"] != 1 {
		t.Fatalf("expected uuid-2 count to be 1, got %d", counts["uuid-2"])
	}
}
