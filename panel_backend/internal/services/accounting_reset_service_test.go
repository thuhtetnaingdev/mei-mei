package services

import (
	"testing"
	"time"

	"panel_backend/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResetAccounting(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	if err := database.AutoMigrate(
		&models.User{},
		&models.UserBandwidthAllocation{},
		&models.UserBandwidthNodeUsage{},
		&models.Node{},
		&models.Miner{},
		&models.MinerReward{},
		&models.MintPoolState{},
		&models.MintPoolEvent{},
		&models.MintPoolTransferEvent{},
	); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	now := time.Now()
	miner := models.Miner{Name: "miner-1", WalletAddress: "wallet-1", RewardedTokens: 11}
	if err := database.Create(&miner).Error; err != nil {
		t.Fatalf("Create miner error = %v", err)
	}

	node := models.Node{
		Name:               "node-1",
		BaseURL:            "http://127.0.0.1:8081",
		PublicHost:         "node.example.com",
		ProtocolToken:      "token",
		BandwidthUsedBytes: 1024,
		RewardedTokens:     5,
		MinerID:            &miner.ID,
	}
	if err := database.Create(&node).Error; err != nil {
		t.Fatalf("Create node error = %v", err)
	}

	user := models.User{
		UUID:                 "user-1",
		Email:                "user@example.com",
		BandwidthUsedBytes:   2048,
		LastWeekMeteredBytes: 512,
		TokenBalance:         9,
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	allocation := models.UserBandwidthAllocation{
		UserID:                  user.ID,
		TotalBandwidthBytes:     4096,
		RemainingBandwidthBytes: 3072,
		TokenAmount:             12,
		RemainingTokens:         8,
		AdminAmount:             2,
		UsagePoolTotal:          7,
		UsagePoolDistributed:    3,
		ReservePoolTotal:        3,
		ReservePoolDistributed:  1,
	}
	if err := database.Create(&allocation).Error; err != nil {
		t.Fatalf("Create allocation error = %v", err)
	}

	if err := database.Create(&models.UserBandwidthNodeUsage{
		AllocationID:   allocation.ID,
		UserID:         user.ID,
		NodeID:         node.ID,
		MinerID:        &miner.ID,
		BandwidthBytes: 128,
		RewardedTokens: 1.5,
	}).Error; err != nil {
		t.Fatalf("Create node usage error = %v", err)
	}

	if err := database.Create(&models.MinerReward{
		MinerID:        miner.ID,
		NodeID:         node.ID,
		UserID:         user.ID,
		AllocationID:   &allocation.ID,
		BandwidthBytes: 128,
		RewardedTokens: 1.5,
	}).Error; err != nil {
		t.Fatalf("Create miner reward error = %v", err)
	}

	if err := database.Create(&models.MintPoolState{
		TotalMMKReserve:         100,
		TotalMeiMinted:          100,
		MainWalletBalance:       80,
		AdminWalletBalance:      10,
		TotalTransferredToUsers: 15,
		TotalRewardedToMiners:   5,
		TotalAdminCollected:     10,
		LastMintAt:              &now,
	}).Error; err != nil {
		t.Fatalf("Create mint pool state error = %v", err)
	}

	if err := database.Create(&models.MintPoolEvent{MMKAmount: 100, MeiAmount: 100, ExchangeRate: "1:1"}).Error; err != nil {
		t.Fatalf("Create mint event error = %v", err)
	}
	if err := database.Create(&models.MintPoolTransferEvent{TransferType: "main_to_user", FromWallet: "main", ToWallet: "user", Amount: 10}).Error; err != nil {
		t.Fatalf("Create mint transfer error = %v", err)
	}

	summary, err := ResetAccounting(database)
	if err != nil {
		t.Fatalf("ResetAccounting() error = %v", err)
	}

	if summary.UsersReset != 1 || summary.NodesReset != 1 || summary.MinersReset != 1 || summary.AllocationsReset != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	var storedUser models.User
	if err := database.First(&storedUser, user.ID).Error; err != nil {
		t.Fatalf("reload user error = %v", err)
	}
	if storedUser.BandwidthUsedBytes != 0 || storedUser.LastWeekMeteredBytes != 0 || storedUser.TokenBalance != 0 {
		t.Fatalf("user not reset: %#v", storedUser)
	}

	var storedNode models.Node
	if err := database.First(&storedNode, node.ID).Error; err != nil {
		t.Fatalf("reload node error = %v", err)
	}
	if storedNode.BandwidthUsedBytes != 0 || storedNode.RewardedTokens != 0 {
		t.Fatalf("node not reset: %#v", storedNode)
	}

	var storedMiner models.Miner
	if err := database.First(&storedMiner, miner.ID).Error; err != nil {
		t.Fatalf("reload miner error = %v", err)
	}
	if storedMiner.RewardedTokens != 0 {
		t.Fatalf("miner not reset: %#v", storedMiner)
	}

	var storedAllocation models.UserBandwidthAllocation
	if err := database.First(&storedAllocation, allocation.ID).Error; err != nil {
		t.Fatalf("reload allocation error = %v", err)
	}
	if storedAllocation.TokenAmount != 0 || storedAllocation.RemainingTokens != 0 || storedAllocation.UsagePoolTotal != 0 {
		t.Fatalf("allocation not reset: %#v", storedAllocation)
	}

	var nodeUsageCount int64
	if err := database.Model(&models.UserBandwidthNodeUsage{}).Count(&nodeUsageCount).Error; err != nil {
		t.Fatalf("count node usages error = %v", err)
	}
	if nodeUsageCount != 0 {
		t.Fatalf("expected node usages deleted, got %d", nodeUsageCount)
	}

	var minerRewardCount int64
	if err := database.Model(&models.MinerReward{}).Count(&minerRewardCount).Error; err != nil {
		t.Fatalf("count miner rewards error = %v", err)
	}
	if minerRewardCount != 0 {
		t.Fatalf("expected miner rewards deleted, got %d", minerRewardCount)
	}

	var pool models.MintPoolState
	if err := database.First(&pool).Error; err != nil {
		t.Fatalf("reload mint pool error = %v", err)
	}
	if pool.TotalMeiMinted != 0 || pool.AdminWalletBalance != 0 || pool.MainWalletBalance != 0 {
		t.Fatalf("mint pool not reset: %#v", pool)
	}
	if pool.LastMintAt != nil {
		t.Fatalf("expected LastMintAt nil, got %v", *pool.LastMintAt)
	}
}
