package services

import (
	"testing"
	"time"

	"panel_backend/internal/models"
)

func TestProcessReportSyncsNodesWhenUsageDisablesUser(t *testing.T) {
	nodeService, conn := newTestNodeService(t, func(captured *SyncPayloadWithLimits) nodeStatusResponse {
		return nodeStatusResponse{
			Status:                 "ok",
			LastAppliedConfigHash:  hashSyncPayload(*captured),
			AppliedUserCount:       countEnabledSyncUsers(captured.Users),
			SyncVerificationStatus: "applied",
		}
	})

	userService := NewUserService(conn)
	user := models.User{
		UUID:    "user-1",
		Email:   "one@example.com",
		Enabled: true,
	}
	if err := conn.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	allocation := models.UserBandwidthAllocation{
		UserID:                  user.ID,
		TotalBandwidthBytes:     bytesPerGB,
		RemainingBandwidthBytes: bytesPerGB,
		TokenAmount:             1,
		RemainingTokens:         1,
		UsagePoolTotal:          1,
		SettlementStatus:        "pending",
	}
	if err := conn.Create(&allocation).Error; err != nil {
		t.Fatalf("create allocation: %v", err)
	}

	reportService := NewBandwidthReportServiceWithSync(conn, userService, nodeService)
	err := reportService.ProcessReport(BandwidthReport{
		NodeName:     "node-1",
		ReportPeriod: time.Now(),
		TotalBytes:   bytesPerGB + 1,
		UserUsage: []UserUsageReport{
			{UUID: user.UUID, BytesUsed: bytesPerGB + 1},
		},
	}, "node-1")
	if err != nil {
		t.Fatalf("ProcessReport() error = %v", err)
	}

	var storedUser models.User
	if err := conn.First(&storedUser, user.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if storedUser.Enabled {
		t.Fatal("expected user to be disabled after exhausting bandwidth")
	}

	var storedNode models.Node
	if err := conn.First(&storedNode).Error; err != nil {
		t.Fatalf("load node: %v", err)
	}
	if storedNode.SyncVerificationStatus != "verified" {
		t.Fatalf("expected node sync verification to be verified, got %q", storedNode.SyncVerificationStatus)
	}
	if storedNode.AppliedUserCount != 0 {
		t.Fatalf("expected disabled user to be removed from node config, got %d applied users", storedNode.AppliedUserCount)
	}
}
