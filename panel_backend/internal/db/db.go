package db

import (
	"errors"
	"fmt"
	"math"
	"os"
	"panel_backend/internal/models"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const bytesPerGB int64 = 1024 * 1024 * 1024

func Connect(databasePath string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil && filepath.Dir(databasePath) != "." {
		return nil, err
	}

	conn, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := conn.AutoMigrate(
		&models.User{},
		&models.UserBandwidthAllocation{},
		&models.UserBandwidthNodeUsage{},
		&models.UserRecord{},
		&models.Miner{},
		&models.MinerReward{},
		&models.Node{},
		&models.AdminSetting{},
		&models.MintPoolState{},
		&models.MintPoolEvent{},
		&models.MintPoolTransferEvent{},
	); err != nil {
		return nil, err
	}

	if err := backfillUserSummaries(conn); err != nil {
		return nil, err
	}
	if err := backfillAllocationDistribution(conn); err != nil {
		return nil, err
	}
	if err := backfillMintPoolTreasury(conn); err != nil {
		return nil, err
	}
	if err := backfillMintPoolTransfers(conn); err != nil {
		return nil, err
	}
	if err := collapseAggregatedUserMinerTransfers(conn); err != nil {
		return nil, err
	}

	return conn, nil
}

func backfillUserSummaries(conn *gorm.DB) error {
	var users []models.User
	if err := conn.Preload("BandwidthAllocations").Find(&users).Error; err != nil {
		return err
	}

	now := time.Now()
	for _, user := range users {
		totalRemainingBytes := int64(0)
		totalRemainingTokens := 0.0
		var latestExpiry *time.Time

		for _, allocation := range user.BandwidthAllocations {
			if allocation.RemainingBandwidthBytes <= 0 {
				continue
			}
			if allocation.ExpiresAt != nil && !allocation.ExpiresAt.After(now) {
				continue
			}

			totalRemainingBytes += allocation.RemainingBandwidthBytes
			totalRemainingTokens += allocation.RemainingTokens
			if allocation.ExpiresAt != nil && (latestExpiry == nil || allocation.ExpiresAt.After(*latestExpiry)) {
				expiry := *allocation.ExpiresAt
				latestExpiry = &expiry
			}
		}

		bandwidthLimitGB := int64(0)
		if totalRemainingBytes > 0 {
			bandwidthLimitGB = int64(math.Ceil(float64(totalRemainingBytes) / float64(bytesPerGB)))
		}

		updates := map[string]any{
			"bandwidth_limit_gb": bandwidthLimitGB,
			"token_balance":      totalRemainingTokens,
			"expires_at":         latestExpiry,
		}
		if err := conn.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}

func backfillMintPoolTreasury(conn *gorm.DB) error {
	var state models.MintPoolState
	err := conn.First(&state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return conn.Create(&models.MintPoolState{}).Error
		}
		return err
	}

	var totalTransferredToUsers float64
	if err := conn.Model(&models.UserBandwidthAllocation{}).
		Select("coalesce(sum(token_amount), 0)").
		Scan(&totalTransferredToUsers).Error; err != nil {
		return err
	}

	var totalRewardedToMiners float64
	if err := conn.Model(&models.MinerReward{}).
		Select("coalesce(sum(rewarded_tokens), 0)").
		Scan(&totalRewardedToMiners).Error; err != nil {
		return err
	}

	state.TotalTransferredToUsers = totalTransferredToUsers
	state.TotalRewardedToMiners = totalRewardedToMiners
	state.MainWalletBalance = float64(state.TotalMeiMinted) - totalTransferredToUsers
	if state.AdminWalletBalance < 0 {
		state.AdminWalletBalance = 0
	}

	return conn.Save(&state).Error
}

func backfillAllocationDistribution(conn *gorm.DB) error {
	var allocations []models.UserBandwidthAllocation
	if err := conn.Find(&allocations).Error; err != nil {
		return err
	}

	for _, allocation := range allocations {
		if allocation.AdminPercent != 0 || allocation.UsagePoolPercent != 0 || allocation.ReservePoolPercent != 0 {
			continue
		}

		usageDistributed := allocation.TokenAmount - allocation.RemainingTokens
		if usageDistributed < 0 {
			usageDistributed = 0
		}

		updates := map[string]any{
			"admin_percent":           0.0,
			"usage_pool_percent":      100.0,
			"reserve_pool_percent":    0.0,
			"admin_amount":            0.0,
			"usage_pool_total":        allocation.TokenAmount,
			"usage_pool_distributed":  usageDistributed,
			"reserve_pool_total":      0.0,
			"reserve_pool_distributed": 0.0,
			"settlement_status":       "legacy",
			"remaining_tokens":        allocation.RemainingTokens,
		}
		if err := conn.Model(&models.UserBandwidthAllocation{}).Where("id = ?", allocation.ID).Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}

func backfillMintPoolTransfers(conn *gorm.DB) error {
	var transferCount int64
	if err := conn.Model(&models.MintPoolTransferEvent{}).Count(&transferCount).Error; err != nil {
		return err
	}
	if transferCount > 0 {
		return nil
	}

	var allocations []models.UserBandwidthAllocation
	if err := conn.Order("created_at asc").Find(&allocations).Error; err != nil {
		return err
	}

	var rewards []models.MinerReward
	if err := conn.Order("created_at asc").Find(&rewards).Error; err != nil {
		return err
	}

	return conn.Transaction(func(tx *gorm.DB) error {
		for _, allocation := range allocations {
			userID := allocation.UserID
			event := models.MintPoolTransferEvent{
				TransferType: "main_to_user",
				FromWallet:   "main_wallet",
				ToWallet:     fmt.Sprintf("user:%d", allocation.UserID),
				Amount:       allocation.TokenAmount,
				UserID:       &userID,
				Note:         "Legacy backfill: user allocation",
				CreatedAt:    allocation.CreatedAt,
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
		}

		for _, reward := range rewards {
			userID := reward.UserID
			minerID := reward.MinerID
			nodeID := reward.NodeID
			event := models.MintPoolTransferEvent{
				TransferType: "user_to_miner",
				FromWallet:   fmt.Sprintf("user:%d", reward.UserID),
				ToWallet:     fmt.Sprintf("miner:%d", reward.MinerID),
				Amount:       reward.RewardedTokens,
				UserID:       &userID,
				MinerID:      &minerID,
				NodeID:       &nodeID,
				Note:         "Legacy backfill: bandwidth reward",
				CreatedAt:    reward.CreatedAt,
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func collapseAggregatedUserMinerTransfers(conn *gorm.DB) error {
	var transfers []models.MintPoolTransferEvent
	if err := conn.
		Where("transfer_type = ? AND user_id IS NOT NULL AND miner_id IS NOT NULL", "user_to_miner").
		Order("created_at asc, id asc").
		Find(&transfers).Error; err != nil {
		return err
	}

	type transferKey struct {
		userID  uint
		minerID uint
	}

	aggregated := make(map[transferKey]*models.MintPoolTransferEvent)
	duplicateIDs := make([]uint, 0)

	for index := range transfers {
		transfer := transfers[index]
		if transfer.UserID == nil || transfer.MinerID == nil {
			continue
		}

		key := transferKey{userID: *transfer.UserID, minerID: *transfer.MinerID}
		existing := aggregated[key]
		if existing == nil {
			copyTransfer := transfer
			if copyTransfer.Note == "" {
				copyTransfer.Note = fmt.Sprintf("Aggregated bandwidth rewards via miner %d", *copyTransfer.MinerID)
			}
			aggregated[key] = &copyTransfer
			continue
		}

		existing.Amount += transfer.Amount
		if transfer.NodeID != nil {
			existing.NodeID = transfer.NodeID
		}
		if transfer.UpdatedAt.After(existing.UpdatedAt) {
			existing.UpdatedAt = transfer.UpdatedAt
		}
		if transfer.CreatedAt.Before(existing.CreatedAt) {
			existing.CreatedAt = transfer.CreatedAt
		}
		existing.Note = fmt.Sprintf("Aggregated bandwidth rewards via miner %d", *existing.MinerID)
		duplicateIDs = append(duplicateIDs, transfer.ID)
	}

	if len(duplicateIDs) == 0 {
		return nil
	}

	return conn.Transaction(func(tx *gorm.DB) error {
		for _, transfer := range aggregated {
			if err := tx.Model(&models.MintPoolTransferEvent{}).
				Where("id = ?", transfer.ID).
				Updates(map[string]any{
					"amount":     transfer.Amount,
					"node_id":    transfer.NodeID,
					"note":       transfer.Note,
					"created_at": transfer.CreatedAt,
					"updated_at": transfer.UpdatedAt,
				}).Error; err != nil {
				return err
			}
		}

		return tx.Delete(&models.MintPoolTransferEvent{}, duplicateIDs).Error
	})
}
