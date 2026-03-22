package services

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"
	"panel_backend/internal/db"
	"panel_backend/internal/models"
)

func TestCreateTestingUserWithoutAllocationsSucceeds(t *testing.T) {
	conn, userService := newTestUserService(t)

	created, err := userService.Create(CreateUserInput{
		Email:     "testing@example.com",
		Enabled:   boolPtr(true),
		IsTesting: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !created.IsTesting {
		t.Fatal("expected created user to be marked testing")
	}
	if len(created.BandwidthAllocations) != 0 {
		t.Fatalf("expected no allocations for testing user, got %d", len(created.BandwidthAllocations))
	}

	var stored models.User
	if err := conn.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("load stored user: %v", err)
	}
	if !stored.IsTesting {
		t.Fatal("expected stored user to persist is_testing")
	}
}

func TestActiveUsersIncludesTestingUsersWithoutAllocations(t *testing.T) {
	conn, userService := newTestUserService(t)

	testingUser := models.User{UUID: "testing-user", Email: "testing@example.com", Enabled: true, IsTesting: true}
	normalUser := models.User{UUID: "normal-user", Email: "normal@example.com", Enabled: true, IsTesting: false}
	if err := conn.Create(&testingUser).Error; err != nil {
		t.Fatalf("create testing user: %v", err)
	}
	if err := conn.Create(&normalUser).Error; err != nil {
		t.Fatalf("create normal user: %v", err)
	}

	activeUsers, err := userService.ActiveUsers()
	if err != nil {
		t.Fatalf("ActiveUsers() error = %v", err)
	}

	if len(activeUsers) != 1 {
		t.Fatalf("expected 1 active user, got %d", len(activeUsers))
	}
	if activeUsers[0].UUID != testingUser.UUID {
		t.Fatalf("expected testing user to be active, got %s", activeUsers[0].UUID)
	}
}

func TestRecordUsageOnNodeForTestingUserSkipsDistribution(t *testing.T) {
	conn, userService := newTestUserService(t)

	user := models.User{UUID: "testing-user", Email: "testing@example.com", Enabled: true, IsTesting: true}
	if err := conn.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	disabled, rewardedTokens, err := userService.RecordUsageOnNode(user.UUID, "node-a", 4096)
	if err != nil {
		t.Fatalf("RecordUsageOnNode() error = %v", err)
	}
	if disabled {
		t.Fatal("expected testing user to stay enabled")
	}
	if rewardedTokens != 0 {
		t.Fatalf("expected no rewarded tokens, got %f", rewardedTokens)
	}

	var stored models.User
	if err := conn.First(&stored, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if stored.BandwidthUsedBytes != 4096 {
		t.Fatalf("expected bandwidth used to increment, got %d", stored.BandwidthUsedBytes)
	}

	var nodeUsageCount int64
	if err := conn.Model(&models.UserBandwidthNodeUsage{}).Count(&nodeUsageCount).Error; err != nil {
		t.Fatalf("count node usages: %v", err)
	}
	if nodeUsageCount != 0 {
		t.Fatalf("expected no node usage reward rows, got %d", nodeUsageCount)
	}

	var rewardCount int64
	if err := conn.Model(&models.MinerReward{}).Count(&rewardCount).Error; err != nil {
		t.Fatalf("count miner rewards: %v", err)
	}
	if rewardCount != 0 {
		t.Fatalf("expected no miner rewards, got %d", rewardCount)
	}
}

func newTestUserService(t *testing.T) (*gorm.DB, *UserService) {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "test.sqlite")
	conn, err := db.Connect(databasePath)
	if err != nil {
		t.Fatalf("db.Connect() error = %v", err)
	}
	return conn, NewUserService(conn)
}

func boolPtr(value bool) *bool {
	return &value
}
