package services

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

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

func TestAddBandwidthAllocationKeepsTotalPackageLimitForProgress(t *testing.T) {
	_, userService := newTestUserService(t)

	expiresAt := time.Now().Add(24 * time.Hour)
	user := &models.User{
		BandwidthUsedBytes: 49 * bytesPerGB,
		BandwidthAllocations: []models.UserBandwidthAllocation{
			{
				TotalBandwidthBytes:     100 * bytesPerGB,
				RemainingBandwidthBytes: 51 * bytesPerGB,
				RemainingTokens:         51,
				ExpiresAt:               &expiresAt,
			},
		},
	}

	userService.hydrateUserSummary(user)

	if user.BandwidthLimitGB != 100 {
		t.Fatalf("expected summarized bandwidth limit to stay at total package size, got %d", user.BandwidthLimitGB)
	}
	if user.TokenBalance != 51 {
		t.Fatalf("expected remaining token balance to stay based on remaining package amount, got %f", user.TokenBalance)
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

func TestUserServicePersistsSelectedNodes(t *testing.T) {
	conn, userService := newTestUserService(t)

	node1 := models.Node{Name: "n1", BaseURL: "http://n1", PublicHost: "n1.example.com", ProtocolToken: "t1", Enabled: true}
	node2 := models.Node{Name: "n2", BaseURL: "http://n2", PublicHost: "n2.example.com", ProtocolToken: "t2", Enabled: true}
	if err := conn.Create(&node1).Error; err != nil {
		t.Fatalf("create node1: %v", err)
	}
	if err := conn.Create(&node2).Error; err != nil {
		t.Fatalf("create node2: %v", err)
	}

	selectedCount := func(userID uint) int64 {
		var count int64
		conn.Model(&models.UserSelectedNode{}).Where("user_id = ?", userID).Count(&count)
		return count
	}

	t.Run("premium user persists selection on create", func(t *testing.T) {
		created, err := userService.Create(CreateUserInput{
			Email:           "premium@example.com",
			Premium:         boolPtr(true),
			SelectedNodeIDs: &[]uint{node1.ID, node2.ID},
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if !created.Premium {
			t.Fatal("expected premium user")
		}
		if count := selectedCount(created.ID); count != 2 {
			t.Fatalf("expected 2 selection rows, got %d", count)
		}
	})

	t.Run("update replaces selection", func(t *testing.T) {
		user, err := userService.Create(CreateUserInput{Email: "replace@example.com", Premium: boolPtr(true)})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if _, err := userService.Update(fmt.Sprint(user.ID), UpdateUserInput{SelectedNodeIDs: &[]uint{node2.ID}}); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if count := selectedCount(user.ID); count != 1 {
			t.Fatalf("expected 1 selection row after replace, got %d", count)
		}
		var row models.UserSelectedNode
		if err := conn.First(&row, "user_id = ?", user.ID).Error; err != nil {
			t.Fatalf("load selection row: %v", err)
		}
		if row.NodeID != node2.ID {
			t.Fatalf("expected replaced selection to be node2, got %d", row.NodeID)
		}
	})

	t.Run("regular user selection is ignored", func(t *testing.T) {
		created, err := userService.Create(CreateUserInput{
			Email:           "regular@example.com",
			SelectedNodeIDs: &[]uint{node1.ID},
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if count := selectedCount(created.ID); count != 0 {
			t.Fatalf("expected no selection rows for regular user, got %d", count)
		}
	})

	t.Run("demoting premium user clears selection", func(t *testing.T) {
		user, err := userService.Create(CreateUserInput{Email: "demote@example.com", Premium: boolPtr(true), SelectedNodeIDs: &[]uint{node1.ID}})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if _, err := userService.Update(fmt.Sprint(user.ID), UpdateUserInput{Premium: boolPtr(false)}); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if count := selectedCount(user.ID); count != 0 {
			t.Fatalf("expected selection cleared after demote, got %d", count)
		}
	})

	t.Run("deleting node cleans its selection rows", func(t *testing.T) {
		node3 := models.Node{Name: "n3", BaseURL: "http://n3", PublicHost: "n3.example.com", ProtocolToken: "t3", Enabled: true}
		if err := conn.Create(&node3).Error; err != nil {
			t.Fatalf("create node3: %v", err)
		}
		user, err := userService.Create(CreateUserInput{Email: "del@example.com", Premium: boolPtr(true), SelectedNodeIDs: &[]uint{node3.ID}})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if count := selectedCount(user.ID); count != 1 {
			t.Fatalf("expected 1 selection row, got %d", count)
		}
		nodeService := NewNodeService(conn, "shared-token", 5*time.Second, NewUserService(conn))
		if err := nodeService.Delete(fmt.Sprint(node3.ID)); err != nil {
			t.Fatalf("delete node: %v", err)
		}
		if count := selectedCount(user.ID); count != 0 {
			t.Fatalf("expected selection rows cleaned after node delete, got %d", count)
		}
	})
}
