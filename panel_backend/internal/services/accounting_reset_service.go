package services

import (
	"panel_backend/internal/models"

	"gorm.io/gorm"
)

type AccountingResetSummary struct {
	UsersReset              int64
	NodesReset              int64
	MinersReset             int64
	AllocationsReset        int64
	NodeUsageEntriesDeleted int64
	MinerRewardsDeleted     int64
	MintEventsDeleted       int64
	MintTransfersDeleted    int64
}

// ResetAccounting zeroes live accounting counters and clears derived accounting history.
func ResetAccounting(db *gorm.DB) (*AccountingResetSummary, error) {
	summary := &AccountingResetSummary{}

	err := db.Transaction(func(tx *gorm.DB) error {
		userResult := tx.Model(&models.User{}).Where("1 = 1").Updates(map[string]interface{}{
			"bandwidth_used_bytes":    0,
			"last_week_metered_bytes": 0,
			"token_balance":           0,
		})
		if userResult.Error != nil {
			return userResult.Error
		}
		summary.UsersReset = userResult.RowsAffected

		nodeResult := tx.Model(&models.Node{}).Where("1 = 1").Updates(map[string]interface{}{
			"bandwidth_used_bytes": 0,
			"rewarded_tokens":      0,
		})
		if nodeResult.Error != nil {
			return nodeResult.Error
		}
		summary.NodesReset = nodeResult.RowsAffected

		minerResult := tx.Model(&models.Miner{}).Where("1 = 1").Update("rewarded_tokens", 0)
		if minerResult.Error != nil {
			return minerResult.Error
		}
		summary.MinersReset = minerResult.RowsAffected

		allocationResult := tx.Model(&models.UserBandwidthAllocation{}).Where("1 = 1").Updates(map[string]interface{}{
			"token_amount":             0,
			"remaining_tokens":         0,
			"admin_amount":             0,
			"usage_pool_total":         0,
			"usage_pool_distributed":   0,
			"reserve_pool_total":       0,
			"reserve_pool_distributed": 0,
		})
		if allocationResult.Error != nil {
			return allocationResult.Error
		}
		summary.AllocationsReset = allocationResult.RowsAffected

		nodeUsageResult := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.UserBandwidthNodeUsage{})
		if nodeUsageResult.Error != nil {
			return nodeUsageResult.Error
		}
		summary.NodeUsageEntriesDeleted = nodeUsageResult.RowsAffected

		minerRewardResult := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.MinerReward{})
		if minerRewardResult.Error != nil {
			return minerRewardResult.Error
		}
		summary.MinerRewardsDeleted = minerRewardResult.RowsAffected

		mintEventResult := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.MintPoolEvent{})
		if mintEventResult.Error != nil {
			return mintEventResult.Error
		}
		summary.MintEventsDeleted = mintEventResult.RowsAffected

		mintTransferResult := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.MintPoolTransferEvent{})
		if mintTransferResult.Error != nil {
			return mintTransferResult.Error
		}
		summary.MintTransfersDeleted = mintTransferResult.RowsAffected

		mintPoolService := NewMintPoolService(tx)
		pool, err := mintPoolService.getOrCreatePoolStateTx(tx)
		if err != nil {
			return err
		}

		pool.TotalMMKReserve = 0
		pool.TotalMeiMinted = 0
		pool.MainWalletBalance = 0
		pool.AdminWalletBalance = 0
		pool.TotalTransferredToUsers = 0
		pool.TotalRewardedToMiners = 0
		pool.TotalAdminCollected = 0
		pool.LastMintAt = nil

		return tx.Save(pool).Error
	})
	if err != nil {
		return nil, err
	}

	return summary, nil
}
