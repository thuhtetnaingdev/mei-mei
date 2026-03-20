package services

import (
	"errors"
	"fmt"
	"math"
	"panel_backend/internal/models"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const bytesPerGB int64 = 1024 * 1024 * 1024
const walletFloatTolerance = 1e-9

type UserService struct {
	db *gorm.DB
}

type UserBandwidthAllocationInput struct {
	BandwidthGB int64      `json:"bandwidthGb" binding:"required,min=1"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	TokenAmount float64    `json:"tokenAmount"`
}

type UserBandwidthReductionInput struct {
	BandwidthGB int64  `json:"bandwidthGb" binding:"required,min=1"`
	Note        string `json:"note"`
}

type UserBandwidthAllocationAdjustmentInput struct {
	Action      string `json:"action" binding:"required,oneof=increase reduce"`
	BandwidthGB int64  `json:"bandwidthGb" binding:"required,min=1"`
	Note        string `json:"note"`
}

type UserBandwidthAllocationUpdateInput struct {
	ExpiresAt *time.Time `json:"expiresAt" binding:"required"`
}

type CreateUserInput struct {
	Email                string                         `json:"email" binding:"required,email"`
	Enabled              *bool                          `json:"enabled"`
	ExpiresAt            *time.Time                     `json:"expiresAt"`
	BandwidthLimitGB     int64                          `json:"bandwidthLimitGb"`
	Notes                string                         `json:"notes"`
	BandwidthAllocations []UserBandwidthAllocationInput `json:"bandwidthAllocations"`
}

type UpdateUserInput struct {
	Email   *string `json:"email"`
	Enabled *bool   `json:"enabled"`
	Notes   *string `json:"notes"`
}

type allocationDistributionSnapshot struct {
	AdminPercent       float64
	UsagePoolPercent   float64
	ReservePoolPercent float64
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) Create(input CreateUserInput) (*models.User, error) {
	if err := validateAllocationInputs(input.BandwidthAllocations); err != nil {
		return nil, err
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	user := models.User{
		UUID:    uuid.NewString(),
		Email:   input.Email,
		Enabled: enabled,
		Notes:   input.Notes,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		allocations := normalizeAllocationInputs(input.BandwidthAllocations, input.BandwidthLimitGB, input.ExpiresAt)
		if err := s.createAllocations(tx, user.ID, allocations, "initial user allocation"); err != nil {
			return err
		}

		if err := s.refreshUserSummaryTx(tx, &user); err != nil {
			return err
		}
		if err := s.createUserRecordTx(tx, user.ID, "created", "User created", s.describeCreateRecord(input, &user)); err != nil {
			return err
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(uintToString(user.ID))
}

func (s *UserService) List() ([]models.User, error) {
	if err := s.settleExpiredAllocations(); err != nil {
		return nil, err
	}

	var users []models.User
	err := s.db.
		Preload("BandwidthAllocations", preloadUserAllocations).
		Preload("BandwidthAllocations.NodeUsages").
		Order("created_at desc").
		Find(&users).Error
	if err == nil {
		for index := range users {
			s.hydrateUserSummary(&users[index])
		}
	}
	return users, err
}

func (s *UserService) GetByID(id string) (*models.User, error) {
	if err := s.settleExpiredAllocations(); err != nil {
		return nil, err
	}

	var user models.User
	err := s.db.
		Preload("BandwidthAllocations", preloadUserAllocations).
		Preload("BandwidthAllocations.NodeUsages").
		First(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	s.hydrateUserSummary(&user)
	return &user, nil
}

func (s *UserService) GetByUUID(uuid string) (*models.User, error) {
	if err := s.settleExpiredAllocations(); err != nil {
		return nil, err
	}

	var user models.User
	err := s.db.
		Preload("BandwidthAllocations", preloadUserAllocations).
		Preload("BandwidthAllocations.NodeUsages").
		First(&user, "uuid = ?", uuid).Error
	if err != nil {
		return nil, err
	}
	s.hydrateUserSummary(&user)
	return &user, nil
}

func (s *UserService) Delete(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", id).Error; err != nil {
			return err
		}

		var allocations []models.UserBandwidthAllocation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", user.ID).
			Find(&allocations).Error; err != nil {
			return err
		}

		refundableTokens := 0.0
		for _, allocation := range allocations {
			if allocation.RemainingTokens > 0 {
				refundableTokens += allocation.RemainingTokens
			}
		}
		if err := s.refundUserTokensToMainWalletTx(tx, user.ID, refundableTokens, "user deleted refund"); err != nil {
			return err
		}
		if err := s.createUserRecordTx(tx, user.ID, "deleted", "User deleted", s.describeDeleteRecord(&user, refundableTokens)); err != nil {
			return err
		}

		return tx.Delete(&models.User{}, "id = ?", id).Error
	})
}

func (s *UserService) Update(id string, input UpdateUserInput) (*models.User, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.First(&user, "id = ?", id).Error; err != nil {
			return err
		}

		if input.Email != nil && *input.Email != "" {
			user.Email = *input.Email
		}
		if input.Enabled != nil {
			user.Enabled = *input.Enabled
		}
		if input.Notes != nil {
			user.Notes = *input.Notes
		}

		if err := tx.Save(&user).Error; err != nil {
			return err
		}

		if err := s.refreshUserSummaryTx(tx, &user); err != nil {
			return err
		}
		if err := s.createUserRecordTx(tx, user.ID, "updated", "User updated", s.describeUpdateRecord(input, &user)); err != nil {
			return err
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

func (s *UserService) AddBandwidthAllocation(userID string, input UserBandwidthAllocationInput) (*models.User, error) {
	if input.BandwidthGB <= 0 {
		return nil, errors.New("bandwidth must be greater than zero")
	}
	if err := validateAllocationExpiry(input); err != nil {
		return nil, err
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			return err
		}

		if err := s.createAllocations(tx, user.ID, []UserBandwidthAllocationInput{input}, "bandwidth top-up"); err != nil {
			return err
		}

		user.Enabled = true
		if err := s.refreshUserSummaryTx(tx, &user); err != nil {
			return err
		}
		if err := s.createUserRecordTx(tx, user.ID, "bandwidth_added", "Bandwidth added", s.describeBandwidthRecord(input)); err != nil {
			return err
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(userID)
}

func (s *UserService) ReduceBandwidthAllocation(userID string, input UserBandwidthReductionInput) (*models.User, error) {
	if input.BandwidthGB <= 0 {
		return nil, errors.New("bandwidth must be greater than zero")
	}

	reductionBytes := input.BandwidthGB * bytesPerGB
	if reductionBytes <= 0 {
		return nil, errors.New("bandwidth must be greater than zero")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			return err
		}

		var allocations []models.UserBandwidthAllocation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", user.ID).
			Find(&allocations).Error; err != nil {
			return err
		}

		sortUserAllocations(allocations)

		now := time.Now()
		availableBytes := int64(0)
		for index := range allocations {
			allocation := allocations[index]
			if allocation.RemainingBandwidthBytes <= 0 {
				continue
			}
			if allocation.ExpiresAt != nil && !allocation.ExpiresAt.After(now) {
				continue
			}
			availableBytes += allocation.RemainingBandwidthBytes
		}

		if availableBytes < reductionBytes {
			return fmt.Errorf("user only has %s remaining bandwidth available to reduce", formatBandwidthBytes(availableBytes))
		}

		remainingReductionBytes := reductionBytes
		refundedTokens := 0.0

		for index := range allocations {
			allocation := &allocations[index]
			if allocation.RemainingBandwidthBytes <= 0 {
				continue
			}
			if allocation.ExpiresAt != nil && !allocation.ExpiresAt.After(now) {
				continue
			}
			if remainingReductionBytes <= 0 {
				break
			}

			reduceBytes := minInt64(remainingReductionBytes, allocation.RemainingBandwidthBytes)
			refund := 0.0
			if allocation.RemainingBandwidthBytes > 0 && allocation.RemainingTokens > 0 {
				refund = allocation.RemainingTokens * (float64(reduceBytes) / float64(allocation.RemainingBandwidthBytes))
			}

			allocation.RemainingBandwidthBytes -= reduceBytes
			allocation.RemainingTokens -= refund
			if allocation.RemainingBandwidthBytes <= 0 {
				allocation.RemainingBandwidthBytes = 0
				allocation.RemainingTokens = 0
			}
			if allocation.RemainingTokens < 0 {
				allocation.RemainingTokens = 0
			}

			if err := tx.Save(allocation).Error; err != nil {
				return err
			}

			remainingReductionBytes -= reduceBytes
			refundedTokens += refund
		}

		if err := s.refundUserTokensToMainWalletTx(tx, user.ID, refundedTokens, s.buildReductionRefundNote(input, refundedTokens)); err != nil {
			return err
		}

		if err := s.refreshUserSummaryWithAllocationsTx(tx, &user, allocations); err != nil {
			return err
		}
		if err := s.createUserRecordTx(tx, user.ID, "bandwidth_reduced", "Bandwidth reduced", s.describeBandwidthReductionRecord(input, refundedTokens)); err != nil {
			return err
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(userID)
}

func (s *UserService) AdjustBandwidthAllocation(userID, allocationID string, input UserBandwidthAllocationAdjustmentInput) (*models.User, error) {
	if input.BandwidthGB <= 0 {
		return nil, errors.New("bandwidth must be greater than zero")
	}
	if input.Action != "increase" && input.Action != "reduce" {
		return nil, errors.New("invalid adjustment action")
	}

	deltaBytes := input.BandwidthGB * bytesPerGB
	if deltaBytes <= 0 {
		return nil, errors.New("bandwidth must be greater than zero")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			return err
		}

		var allocation models.UserBandwidthAllocation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&allocation, "id = ? AND user_id = ?", allocationID, user.ID).Error; err != nil {
			return err
		}

		if allocation.TotalBandwidthBytes <= 0 {
			return errors.New("allocation has no bandwidth to adjust")
		}

		tokenDelta := roundTokenAmount(allocation.TokenAmount * (float64(deltaBytes) / float64(allocation.TotalBandwidthBytes)))
		if tokenDelta < 0 {
			tokenDelta = 0
		}
		adminDelta := roundTokenAmount(tokenDelta * allocation.AdminPercent / 100)
		usageDelta := roundTokenAmount(tokenDelta * allocation.UsagePoolPercent / 100)
		reserveDelta := roundTokenAmount(tokenDelta - adminDelta - usageDelta)

		switch input.Action {
		case "increase":
			if err := s.transferFromMainWalletToUserTx(tx, user.ID, tokenDelta, s.buildAdjustmentWalletNote(input, allocation, tokenDelta)); err != nil {
				return err
			}
			if err := s.creditAdminWalletTx(tx, user.ID, adminDelta, s.buildAdjustmentWalletNote(input, allocation, adminDelta)); err != nil {
				return err
			}

			allocation.TotalBandwidthBytes += deltaBytes
			allocation.RemainingBandwidthBytes += deltaBytes
			allocation.TokenAmount = roundTokenAmount(allocation.TokenAmount + tokenDelta)
			allocation.AdminAmount = roundTokenAmount(allocation.AdminAmount + adminDelta)
			allocation.UsagePoolTotal = roundTokenAmount(allocation.UsagePoolTotal + usageDelta)
			allocation.ReservePoolTotal = roundTokenAmount(allocation.ReservePoolTotal + reserveDelta)
			allocation.RemainingTokens = roundTokenAmount(s.calculateRemainingAllocationTokens(allocation))
			user.Enabled = true
		case "reduce":
			if allocation.RemainingBandwidthBytes < deltaBytes {
				return fmt.Errorf("only %s remains in this bandwidth entry", formatBandwidthBytes(allocation.RemainingBandwidthBytes))
			}

			if err := s.refundUserTokensToMainWalletTx(tx, user.ID, tokenDelta, s.buildAdjustmentWalletNote(input, allocation, tokenDelta)); err != nil {
				return err
			}

			allocation.TotalBandwidthBytes -= deltaBytes
			allocation.RemainingBandwidthBytes -= deltaBytes
			allocation.TokenAmount = roundTokenAmount(allocation.TokenAmount - tokenDelta)
			allocation.AdminAmount = roundTokenAmount(maxFloat64(0, allocation.AdminAmount-adminDelta))
			allocation.UsagePoolTotal = roundTokenAmount(maxFloat64(allocation.UsagePoolDistributed, allocation.UsagePoolTotal-usageDelta))
			allocation.ReservePoolTotal = roundTokenAmount(maxFloat64(allocation.ReservePoolDistributed, allocation.ReservePoolTotal-reserveDelta))
			allocation.RemainingTokens = roundTokenAmount(s.calculateRemainingAllocationTokens(allocation))

			if allocation.TotalBandwidthBytes < 0 {
				allocation.TotalBandwidthBytes = 0
			}
			if allocation.RemainingBandwidthBytes < 0 {
				allocation.RemainingBandwidthBytes = 0
			}
			allocation.TokenAmount = maxFloat64(0, allocation.TokenAmount)
			allocation.RemainingTokens = maxFloat64(0, allocation.RemainingTokens)
		}

		if err := tx.Save(&allocation).Error; err != nil {
			return err
		}

		var allocations []models.UserBandwidthAllocation
		if err := tx.Where("user_id = ?", user.ID).Find(&allocations).Error; err != nil {
			return err
		}
		if err := s.refreshUserSummaryWithAllocationsTx(tx, &user, allocations); err != nil {
			return err
		}
		if !hasActiveBandwidth(allocations, time.Now()) {
			user.Enabled = false
		}

		if err := s.createUserRecordTx(tx, user.ID, "bandwidth_entry_adjusted", "Bandwidth entry adjusted", s.describeBandwidthAdjustmentRecord(input, &allocation, tokenDelta)); err != nil {
			return err
		}

		return tx.Save(&user).Error
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(userID)
}

func (s *UserService) UpdateBandwidthAllocation(userID, allocationID string, input UserBandwidthAllocationUpdateInput) (*models.User, error) {
	if input.ExpiresAt == nil {
		return nil, errors.New("expiry is required")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			return err
		}

		var allocation models.UserBandwidthAllocation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&allocation, "id = ? AND user_id = ?", allocationID, user.ID).Error; err != nil {
			return err
		}

		allocation.ExpiresAt = input.ExpiresAt
		if err := tx.Save(&allocation).Error; err != nil {
			return err
		}

		var allocations []models.UserBandwidthAllocation
		if err := tx.Where("user_id = ?", user.ID).Find(&allocations).Error; err != nil {
			return err
		}
		if err := s.refreshUserSummaryWithAllocationsTx(tx, &user, allocations); err != nil {
			return err
		}
		if hasActiveBandwidth(allocations, time.Now()) {
			user.Enabled = true
		}

		if err := s.createUserRecordTx(tx, user.ID, "bandwidth_entry_updated", "Bandwidth entry updated", s.describeBandwidthEntryUpdateRecord(&allocation)); err != nil {
			return err
		}

		return tx.Save(&user).Error
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(userID)
}

func (s *UserService) ActiveUsers() ([]models.User, error) {
	users, err := s.List()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	active := make([]models.User, 0, len(users))
	for _, user := range users {
		if user.Enabled && hasActiveBandwidth(user.BandwidthAllocations, now) {
			active = append(active, user)
		}
	}
	return active, nil
}

func (s *UserService) ListRecords(userID string) ([]models.UserRecord, error) {
	var records []models.UserRecord
	err := s.db.Where("user_id = ?", userID).Order("created_at desc").Find(&records).Error
	return records, err
}

func (s *UserService) AddUsage(uuid string, bytes int64) error {
	_, _, err := s.RecordUsageOnNode(uuid, "", bytes)
	return err
}

func (s *UserService) AddUsageAndDisableIfLimitReached(uuid string, bytes int64) (bool, error) {
	disabled, _, err := s.RecordUsageOnNode(uuid, "", bytes)
	return disabled, err
}

func (s *UserService) RecordUsageOnNode(uuid string, nodeName string, bytes int64) (bool, float64, error) {
	if uuid == "" || bytes <= 0 {
		return false, 0, errors.New("invalid usage payload")
	}

	disabled := false
	rewardedTokens := 0.0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "uuid = ?", uuid).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		var allocations []models.UserBandwidthAllocation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", user.ID).
			Find(&allocations).Error; err != nil {
			return err
		}

		if err := s.settleExpiredAllocationsForUserTx(tx, user.ID, allocations, time.Now()); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", user.ID).
			Find(&allocations).Error; err != nil {
			return err
		}

		sortUserAllocations(allocations)

		now := time.Now()
		remainingRewardableBytes := bytes

		for index := range allocations {
			allocation := &allocations[index]
			if allocation.RemainingBandwidthBytes <= 0 {
				continue
			}
			if allocation.ExpiresAt != nil && !allocation.ExpiresAt.After(now) {
				continue
			}
			if remainingRewardableBytes <= 0 {
				break
			}

			consumeBytes := minInt64(remainingRewardableBytes, allocation.RemainingBandwidthBytes)
			reward := s.calculateUsageReward(*allocation, consumeBytes)

			allocation.RemainingBandwidthBytes -= consumeBytes
			allocation.UsagePoolDistributed = roundTokenAmount(allocation.UsagePoolDistributed + reward)
			allocation.RemainingTokens = roundTokenAmount(s.calculateRemainingAllocationTokens(*allocation))
			if allocation.RemainingBandwidthBytes <= 0 {
				allocation.RemainingBandwidthBytes = 0
			}
			if allocation.RemainingTokens < 0 {
				allocation.RemainingTokens = 0
			}

			if err := tx.Save(allocation).Error; err != nil {
				return err
			}

			if reward > 0 && nodeName != "" {
				if err := s.recordAllocationNodeUsageTx(tx, *allocation, nodeName, user.ID, consumeBytes, reward, now); err != nil {
					return err
				}
			}

			remainingRewardableBytes -= consumeBytes
			rewardedTokens += reward
		}

		user.BandwidthUsedBytes += bytes
		if err := s.refreshUserSummaryWithAllocationsTx(tx, &user, allocations); err != nil {
			return err
		}

		if !hasActiveBandwidth(allocations, now) {
			if user.Enabled {
				user.Enabled = false
				disabled = true
			}
		}

		if err := tx.Save(&user).Error; err != nil {
			return err
		}

		return nil
	})

	return disabled, roundTokenAmount(rewardedTokens), err
}

func (s *UserService) createAllocations(tx *gorm.DB, userID uint, inputs []UserBandwidthAllocationInput, transferContext string) error {
	for _, input := range inputs {
		if input.BandwidthGB <= 0 {
			continue
		}

		tokenAmount := input.TokenAmount
		if tokenAmount <= 0 {
			tokenAmount = float64(input.BandwidthGB)
		}

		split, err := s.getDistributionSettingsTx(tx)
		if err != nil {
			return err
		}

		if err := s.transferFromMainWalletToUserTx(tx, userID, tokenAmount, buildAllocationTransferNote(transferContext, input.BandwidthGB, input.ExpiresAt)); err != nil {
			return err
		}

		adminAmount := roundTokenAmount(tokenAmount * split.AdminPercent / 100)
		usagePoolTotal := roundTokenAmount(tokenAmount * split.UsagePoolPercent / 100)
		reservePoolTotal := roundTokenAmount(tokenAmount - adminAmount - usagePoolTotal)
		if reservePoolTotal < 0 {
			reservePoolTotal = 0
		}
		if err := s.creditAdminWalletTx(tx, userID, adminAmount, buildAdminWalletNote(transferContext, input.BandwidthGB)); err != nil {
			return err
		}

		totalBytes := input.BandwidthGB * bytesPerGB
		allocation := models.UserBandwidthAllocation{
			UserID:                  userID,
			TotalBandwidthBytes:     totalBytes,
			RemainingBandwidthBytes: totalBytes,
			TokenAmount:             tokenAmount,
			RemainingTokens:         usagePoolTotal + reservePoolTotal,
			AdminPercent:            split.AdminPercent,
			UsagePoolPercent:        split.UsagePoolPercent,
			ReservePoolPercent:      split.ReservePoolPercent,
			AdminAmount:             adminAmount,
			UsagePoolTotal:          usagePoolTotal,
			UsagePoolDistributed:    0,
			ReservePoolTotal:        reservePoolTotal,
			ReservePoolDistributed:  0,
			SettlementStatus:        "pending",
			ExpiresAt:               input.ExpiresAt,
		}
		if err := tx.Create(&allocation).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *UserService) transferFromMainWalletToUserTx(tx *gorm.DB, userID uint, amount float64, note string) error {
	if amount <= 0 {
		return nil
	}

	state, err := getOrCreateMintPoolStateTx(tx)
	if err != nil {
		return err
	}

	if state.MainWalletBalance+walletFloatTolerance < amount {
		return fmt.Errorf("main wallet has %.2f Mei available, but %.2f Mei is required. Mint more Mei first", state.MainWalletBalance, amount)
	}

	state.MainWalletBalance -= amount
	if math.Abs(state.MainWalletBalance) < walletFloatTolerance {
		state.MainWalletBalance = 0
	}
	state.TotalTransferredToUsers += amount

	if err := tx.Save(state).Error; err != nil {
		return err
	}

	userIDCopy := userID
	transferEvent := models.MintPoolTransferEvent{
		TransferType: "main_to_user",
		FromWallet:   "main_wallet",
		ToWallet:     fmt.Sprintf("user:%d", userID),
		Amount:       amount,
		UserID:       &userIDCopy,
		Note:         note,
		CreatedAt:    time.Now(),
	}
	return tx.Create(&transferEvent).Error
}

func (s *UserService) recordUserToMinerTransferTx(tx *gorm.DB, userID, minerID, nodeID uint, amount float64, transferType, nodeName string, now time.Time) error {
	if amount <= 0 {
		return nil
	}

	state, err := getOrCreateMintPoolStateTx(tx)
	if err != nil {
		return err
	}

	state.TotalRewardedToMiners = roundTokenAmount(state.TotalRewardedToMiners + amount)
	if err := tx.Save(state).Error; err != nil {
		return err
	}

	var transferEvent models.MintPoolTransferEvent
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("transfer_type = ? AND user_id = ? AND miner_id = ?", transferType, userID, minerID).
		First(&transferEvent).Error

	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		userIDCopy := userID
		minerIDCopy := minerID
		nodeIDCopy := nodeID
		transferEvent = models.MintPoolTransferEvent{
			TransferType: transferType,
			FromWallet:   fmt.Sprintf("user:%d", userID),
			ToWallet:     fmt.Sprintf("miner:%d", minerID),
			Amount:       amount,
			UserID:       &userIDCopy,
			MinerID:      &minerIDCopy,
			NodeID:       &nodeIDCopy,
			Note:         fmt.Sprintf("Aggregated %s rewards via miner %d", transferType, minerID),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		return tx.Create(&transferEvent).Error
	}

	transferEvent.Amount = roundTokenAmount(transferEvent.Amount + amount)
	transferEvent.NodeID = &nodeID
	transferEvent.Note = fmt.Sprintf("Aggregated %s rewards via node %s", transferType, nodeName)
	transferEvent.UpdatedAt = now
	return tx.Save(&transferEvent).Error
}

func (s *UserService) refundUserTokensToMainWalletTx(tx *gorm.DB, userID uint, amount float64, note string) error {
	if amount <= 0 {
		return nil
	}

	state, err := getOrCreateMintPoolStateTx(tx)
	if err != nil {
		return err
	}

	state.MainWalletBalance += amount
	if math.Abs(state.MainWalletBalance) < walletFloatTolerance {
		state.MainWalletBalance = 0
	}

	if err := tx.Save(state).Error; err != nil {
		return err
	}

	userIDCopy := userID
	transferEvent := models.MintPoolTransferEvent{
		TransferType: "user_to_main",
		FromWallet:   fmt.Sprintf("user:%d", userID),
		ToWallet:     "main_wallet",
		Amount:       amount,
		UserID:       &userIDCopy,
		Note:         note,
		CreatedAt:    time.Now(),
	}
	return tx.Create(&transferEvent).Error
}

func (s *UserService) creditAdminWalletTx(tx *gorm.DB, userID uint, amount float64, note string) error {
	if amount <= 0 {
		return nil
	}

	state, err := getOrCreateMintPoolStateTx(tx)
	if err != nil {
		return err
	}

	state.AdminWalletBalance = roundTokenAmount(state.AdminWalletBalance + amount)
	state.TotalAdminCollected = roundTokenAmount(state.TotalAdminCollected + amount)
	if err := tx.Save(state).Error; err != nil {
		return err
	}

	userIDCopy := userID
	transferEvent := models.MintPoolTransferEvent{
		TransferType: "admin_fee",
		FromWallet:   fmt.Sprintf("user:%d", userID),
		ToWallet:     "admin_wallet",
		Amount:       amount,
		UserID:       &userIDCopy,
		Note:         note,
		CreatedAt:    time.Now(),
	}
	return tx.Create(&transferEvent).Error
}

func (s *UserService) getDistributionSettingsTx(tx *gorm.DB) (allocationDistributionSnapshot, error) {
	var admin models.AdminSetting
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&admin).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return allocationDistributionSnapshot{}, err
		}
		return allocationDistributionSnapshot{
			AdminPercent:       DefaultAdminPercent,
			UsagePoolPercent:   DefaultUsagePoolPercent,
			ReservePoolPercent: DefaultReservePercent,
		}, nil
	}

	snapshot := allocationDistributionSnapshot{
		AdminPercent:       normalizedPercent(admin.AdminPercent, DefaultAdminPercent),
		UsagePoolPercent:   normalizedPercent(admin.UsagePoolPercent, DefaultUsagePoolPercent),
		ReservePoolPercent: normalizedPercent(admin.ReservePoolPercent, DefaultReservePercent),
	}
	if err := validateDistributionSettings(DistributionSettings{
		AdminPercent:       snapshot.AdminPercent,
		UsagePoolPercent:   snapshot.UsagePoolPercent,
		ReservePoolPercent: snapshot.ReservePoolPercent,
	}); err != nil {
		return allocationDistributionSnapshot{}, err
	}
	return snapshot, nil
}

func (s *UserService) calculateUsageReward(allocation models.UserBandwidthAllocation, consumeBytes int64) float64 {
	if consumeBytes <= 0 || allocation.RemainingBandwidthBytes <= 0 {
		return 0
	}

	remainingUsagePool := allocation.UsagePoolTotal - allocation.UsagePoolDistributed
	if remainingUsagePool <= 0 {
		return 0
	}

	reward := remainingUsagePool * (float64(consumeBytes) / float64(allocation.RemainingBandwidthBytes))
	return roundTokenAmount(reward)
}

func (s *UserService) calculateRemainingAllocationTokens(allocation models.UserBandwidthAllocation) float64 {
	remaining := (allocation.UsagePoolTotal - allocation.UsagePoolDistributed) + (allocation.ReservePoolTotal - allocation.ReservePoolDistributed)
	if remaining < 0 {
		return 0
	}
	return roundTokenAmount(remaining)
}

func (s *UserService) recordAllocationNodeUsageTx(tx *gorm.DB, allocation models.UserBandwidthAllocation, nodeName string, userID uint, bandwidthBytes int64, reward float64, now time.Time) error {
	if nodeName == "" || bandwidthBytes <= 0 {
		return nil
	}

	var node models.Node
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("name = ?", nodeName).
		First(&node).Error; err != nil {
		return nil
	}

	var usage models.UserBandwidthNodeUsage
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("allocation_id = ? AND node_id = ?", allocation.ID, node.ID).
		First(&usage).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		usage = models.UserBandwidthNodeUsage{
			AllocationID:   allocation.ID,
			UserID:         userID,
			NodeID:         node.ID,
			MinerID:        node.MinerID,
			BandwidthBytes: bandwidthBytes,
			RewardedTokens: reward,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&usage).Error; err != nil {
			return err
		}
	} else {
		usage.BandwidthBytes += bandwidthBytes
		usage.RewardedTokens = roundTokenAmount(usage.RewardedTokens + reward)
		usage.MinerID = node.MinerID
		usage.UpdatedAt = now
		if err := tx.Save(&usage).Error; err != nil {
			return err
		}
	}

	if node.MinerID == nil || reward <= 0 {
		return nil
	}

	return s.creditMinerRewardTx(tx, userID, allocation.ID, *node.MinerID, node.ID, bandwidthBytes, reward, "usage_pool", node.Name, now)
}

func (s *UserService) creditMinerRewardTx(tx *gorm.DB, userID, allocationID, minerID, nodeID uint, bandwidthBytes int64, amount float64, source, nodeName string, now time.Time) error {
	if amount <= 0 {
		return nil
	}

	if err := tx.Model(&models.Node{}).
		Where("id = ?", nodeID).
		UpdateColumn("rewarded_tokens", gorm.Expr("COALESCE(rewarded_tokens, 0) + ?", amount)).Error; err != nil {
		return err
	}
	if err := tx.Model(&models.Miner{}).
		Where("id = ?", minerID).
		UpdateColumn("rewarded_tokens", gorm.Expr("COALESCE(rewarded_tokens, 0) + ?", amount)).Error; err != nil {
		return err
	}

	rewardEvent := models.MinerReward{
		MinerID:        minerID,
		NodeID:         nodeID,
		UserID:         userID,
		AllocationID:   &allocationID,
		BandwidthBytes: bandwidthBytes,
		RewardedTokens: amount,
		RewardSource:   source,
		CreatedAt:      now,
	}
	if err := tx.Create(&rewardEvent).Error; err != nil {
		return err
	}

	return s.recordUserToMinerTransferTx(tx, userID, minerID, nodeID, amount, source, nodeName, now)
}

func (s *UserService) settleExpiredAllocations() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var users []models.User
		if err := tx.Find(&users).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, user := range users {
			var allocations []models.UserBandwidthAllocation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ?", user.ID).
				Find(&allocations).Error; err != nil {
				return err
			}
			if err := s.settleExpiredAllocationsForUserTx(tx, user.ID, allocations, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *UserService) settleExpiredAllocationsForUserTx(tx *gorm.DB, userID uint, allocations []models.UserBandwidthAllocation, now time.Time) error {
	for index := range allocations {
		allocation := &allocations[index]
		if allocation.ExpiresAt == nil || allocation.ExpiresAt.After(now) {
			continue
		}
		if allocation.SettledAt != nil {
			continue
		}
		if err := s.settleExpiredAllocationTx(tx, userID, allocation, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *UserService) settleExpiredAllocationTx(tx *gorm.DB, userID uint, allocation *models.UserBandwidthAllocation, now time.Time) error {
	var nodeUsages []models.UserBandwidthNodeUsage
	if err := tx.Where("allocation_id = ?", allocation.ID).Find(&nodeUsages).Error; err != nil {
		return err
	}

	totalUsageBytes := int64(0)
	for _, usage := range nodeUsages {
		totalUsageBytes += usage.BandwidthBytes
	}

	if totalUsageBytes <= 0 {
		allocation.SettlementStatus = "warning"
		allocation.SettlementWarning = "Expired with no recorded node usage. Reserve and leftover usage pool were not auto-distributed."
		allocation.RemainingTokens = roundTokenAmount(s.calculateRemainingAllocationTokens(*allocation))
		return tx.Save(allocation).Error
	}

	remainingUsagePool := roundTokenAmount(allocation.UsagePoolTotal - allocation.UsagePoolDistributed)
	remainingReservePool := roundTokenAmount(allocation.ReservePoolTotal - allocation.ReservePoolDistributed)
	if remainingUsagePool <= 0 && remainingReservePool <= 0 {
		allocation.SettledAt = &now
		allocation.SettlementStatus = "settled"
		allocation.SettlementWarning = ""
		allocation.RemainingTokens = 0
		return tx.Save(allocation).Error
	}

	if err := s.distributeAllocationPoolTx(tx, userID, *allocation, nodeUsages, remainingUsagePool, "usage_pool_expiry", now); err != nil {
		return err
	}
	if err := s.distributeAllocationPoolTx(tx, userID, *allocation, nodeUsages, remainingReservePool, "reserve_pool", now); err != nil {
		return err
	}

	allocation.UsagePoolDistributed = allocation.UsagePoolTotal
	allocation.ReservePoolDistributed = allocation.ReservePoolTotal
	allocation.RemainingTokens = 0
	allocation.SettledAt = &now
	allocation.SettlementStatus = "settled"
	allocation.SettlementWarning = ""
	return tx.Save(allocation).Error
}

func getOrCreateMintPoolStateTx(tx *gorm.DB) (*models.MintPoolState, error) {
	var state models.MintPoolState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state).Error
	if err == nil {
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	state = models.MintPoolState{}
	if err := tx.Create(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

func buildAllocationTransferNote(context string, bandwidthGB int64, expiresAt *time.Time) string {
	note := fmt.Sprintf("%s: %d GB", context, bandwidthGB)
	if expiresAt != nil {
		note = fmt.Sprintf("%s, expires %s", note, expiresAt.Format(time.RFC3339))
	}
	return note
}

func formatBandwidthBytes(bytes int64) string {
	if bytes <= 0 {
		return "0.00 GB"
	}
	return fmt.Sprintf("%.2f GB", float64(bytes)/float64(bytesPerGB))
}

func sortUserAllocations(allocations []models.UserBandwidthAllocation) {
	sort.SliceStable(allocations, func(i, j int) bool {
		left := allocations[i]
		right := allocations[j]
		switch {
		case left.ExpiresAt == nil && right.ExpiresAt == nil:
			return left.CreatedAt.Before(right.CreatedAt)
		case left.ExpiresAt == nil:
			return false
		case right.ExpiresAt == nil:
			return true
		default:
			if left.ExpiresAt.Equal(*right.ExpiresAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
			return left.ExpiresAt.Before(*right.ExpiresAt)
		}
	})
}

func (s *UserService) createUserRecordTx(tx *gorm.DB, userID uint, action, title, details string) error {
	record := models.UserRecord{
		UserID:  userID,
		Action:  action,
		Title:   title,
		Details: details,
	}
	return tx.Create(&record).Error
}

func (s *UserService) describeCreateRecord(input CreateUserInput, user *models.User) string {
	details := fmt.Sprintf("Email: %s", user.Email)
	if input.Notes != "" {
		details += fmt.Sprintf(" | Notes: %s", input.Notes)
	}
	if len(input.BandwidthAllocations) > 0 {
		first := input.BandwidthAllocations[0]
		details += fmt.Sprintf(" | Initial package: %d GB / %.2f tokens", first.BandwidthGB, normalizedTokenAmount(first))
		if first.ExpiresAt != nil {
			details += fmt.Sprintf(" | Expiry: %s", first.ExpiresAt.Format(time.RFC3339))
		}
	}
	return details
}

func (s *UserService) describeUpdateRecord(input UpdateUserInput, user *models.User) string {
	parts := []string{fmt.Sprintf("Email: %s", user.Email), fmt.Sprintf("Enabled: %t", user.Enabled)}
	if user.Notes != "" {
		parts = append(parts, fmt.Sprintf("Notes: %s", user.Notes))
	}
	return strings.Join(parts, " | ")
}

func (s *UserService) describeBandwidthRecord(input UserBandwidthAllocationInput) string {
	details := fmt.Sprintf("Added package: %d GB / %.2f tokens", input.BandwidthGB, normalizedTokenAmount(input))
	if input.ExpiresAt != nil {
		details += fmt.Sprintf(" | Expiry: %s", input.ExpiresAt.Format(time.RFC3339))
	}
	return details
}

func (s *UserService) describeBandwidthReductionRecord(input UserBandwidthReductionInput, refundedTokens float64) string {
	details := fmt.Sprintf("Reduced package: %d GB / %.2f refunded tokens", input.BandwidthGB, refundedTokens)
	if trimmedNote := strings.TrimSpace(input.Note); trimmedNote != "" {
		details += fmt.Sprintf(" | Note: %s", trimmedNote)
	}
	return details
}

func (s *UserService) buildReductionRefundNote(input UserBandwidthReductionInput, refundedTokens float64) string {
	note := fmt.Sprintf("bandwidth reduction refund: %d GB / %.2f tokens", input.BandwidthGB, refundedTokens)
	if trimmedNote := strings.TrimSpace(input.Note); trimmedNote != "" {
		note += fmt.Sprintf(" | %s", trimmedNote)
	}
	return note
}

func (s *UserService) buildAdjustmentWalletNote(input UserBandwidthAllocationAdjustmentInput, allocation models.UserBandwidthAllocation, tokenDelta float64) string {
	return fmt.Sprintf(
		"bandwidth entry %s: allocation %d, %d GB / %.2f tokens%s",
		input.Action,
		allocation.ID,
		input.BandwidthGB,
		tokenDelta,
		formatOptionalNote(input.Note),
	)
}

func (s *UserService) describeDeleteRecord(user *models.User, refundedTokens float64) string {
	details := fmt.Sprintf("Deleted user %s", user.Email)
	if refundedTokens > 0 {
		details += fmt.Sprintf(" | Refunded %.2f tokens to main wallet", refundedTokens)
	}
	return details
}

func (s *UserService) describeBandwidthAdjustmentRecord(input UserBandwidthAllocationAdjustmentInput, allocation *models.UserBandwidthAllocation, tokenDelta float64) string {
	return fmt.Sprintf(
		"Entry #%d %sed by %d GB / %.2f tokens%s",
		allocation.ID,
		input.Action,
		input.BandwidthGB,
		tokenDelta,
		formatOptionalNote(input.Note),
	)
}

func (s *UserService) describeBandwidthEntryUpdateRecord(allocation *models.UserBandwidthAllocation) string {
	expiry := "No expiry"
	if allocation.ExpiresAt != nil {
		expiry = allocation.ExpiresAt.Format(time.RFC3339)
	}
	return fmt.Sprintf("Entry #%d expiry updated to %s", allocation.ID, expiry)
}

func normalizedTokenAmount(input UserBandwidthAllocationInput) float64 {
	if input.TokenAmount > 0 {
		return input.TokenAmount
	}
	return float64(input.BandwidthGB)
}

func (s *UserService) refreshUserSummaryTx(tx *gorm.DB, user *models.User) error {
	var allocations []models.UserBandwidthAllocation
	if err := tx.Where("user_id = ?", user.ID).Find(&allocations).Error; err != nil {
		return err
	}
	return s.refreshUserSummaryWithAllocationsTx(tx, user, allocations)
}

func (s *UserService) refreshUserSummaryWithAllocationsTx(_ *gorm.DB, user *models.User, allocations []models.UserBandwidthAllocation) error {
	now := time.Now()
	totalRemainingBytes := int64(0)
	totalRemainingTokens := 0.0
	var latestExpiry *time.Time

	for _, allocation := range allocations {
		if allocation.RemainingBandwidthBytes <= 0 {
			continue
		}
		if allocation.ExpiresAt != nil && !allocation.ExpiresAt.After(now) {
			continue
		}

		totalRemainingBytes += allocation.RemainingBandwidthBytes
		totalRemainingTokens += allocation.RemainingTokens
		if allocation.ExpiresAt != nil {
			if latestExpiry == nil || allocation.ExpiresAt.After(*latestExpiry) {
				expiry := *allocation.ExpiresAt
				latestExpiry = &expiry
			}
		}
	}

	user.TokenBalance = totalRemainingTokens
	user.BandwidthLimitGB = bytesToRoundedGB(totalRemainingBytes)
	user.ExpiresAt = latestExpiry
	user.BandwidthAllocations = allocations
	return nil
}

func (s *UserService) hydrateUserSummary(user *models.User) {
	if user == nil {
		return
	}
	_ = s.refreshUserSummaryWithAllocationsTx(nil, user, user.BandwidthAllocations)
}

func normalizeAllocationInputs(inputs []UserBandwidthAllocationInput, legacyBandwidthGB int64, legacyExpiry *time.Time) []UserBandwidthAllocationInput {
	if len(inputs) > 0 {
		return inputs
	}
	if legacyBandwidthGB <= 0 {
		return nil
	}

	return []UserBandwidthAllocationInput{
		{
			BandwidthGB: legacyBandwidthGB,
			ExpiresAt:   legacyExpiry,
			TokenAmount: float64(legacyBandwidthGB),
		},
	}
}

func validateAllocationInputs(inputs []UserBandwidthAllocationInput) error {
	for _, input := range inputs {
		if err := validateAllocationExpiry(input); err != nil {
			return err
		}
	}
	return nil
}

func validateAllocationExpiry(input UserBandwidthAllocationInput) error {
	if input.BandwidthGB > 0 && input.ExpiresAt == nil {
		return errors.New("expiry is required when assigning bandwidth")
	}
	return nil
}

func preloadUserAllocations(db *gorm.DB) *gorm.DB {
	return db.Order("created_at desc")
}

func hasActiveBandwidth(allocations []models.UserBandwidthAllocation, now time.Time) bool {
	for _, allocation := range allocations {
		if allocation.RemainingBandwidthBytes <= 0 {
			continue
		}
		if allocation.ExpiresAt != nil && !allocation.ExpiresAt.After(now) {
			continue
		}
		return true
	}
	return false
}

func bytesToRoundedGB(bytes int64) int64 {
	if bytes <= 0 {
		return 0
	}
	return int64(math.Ceil(float64(bytes) / float64(bytesPerGB)))
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxFloat64(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func uintToString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}

func formatOptionalNote(note string) string {
	trimmed := strings.TrimSpace(note)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf(" | Note: %s", trimmed)
}

func buildAdminWalletNote(context string, bandwidthGB int64) string {
	return fmt.Sprintf("%s admin fee: %d GB package", context, bandwidthGB)
}

func roundTokenAmount(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func (s *UserService) distributeAllocationPoolTx(tx *gorm.DB, userID uint, allocation models.UserBandwidthAllocation, nodeUsages []models.UserBandwidthNodeUsage, totalAmount float64, source string, now time.Time) error {
	if totalAmount <= 0 {
		return nil
	}

	totalUsageBytes := int64(0)
	for _, usage := range nodeUsages {
		if usage.MinerID == nil {
			continue
		}
		totalUsageBytes += usage.BandwidthBytes
	}
	if totalUsageBytes <= 0 {
		return nil
	}

	distributed := 0.0
	distributableIndexes := make([]int, 0, len(nodeUsages))
	for index, usage := range nodeUsages {
		if usage.MinerID != nil {
			distributableIndexes = append(distributableIndexes, index)
		}
	}

	for idx, usageIndex := range distributableIndexes {
		usage := nodeUsages[usageIndex]
		share := totalAmount * (float64(usage.BandwidthBytes) / float64(totalUsageBytes))
		if idx == len(distributableIndexes)-1 {
			share = totalAmount - distributed
		}
		share = roundTokenAmount(share)
		if share <= 0 {
			continue
		}
		if err := s.creditMinerRewardTx(tx, userID, allocation.ID, *usage.MinerID, usage.NodeID, 0, share, source, fmt.Sprintf("node-%d", usage.NodeID), now); err != nil {
			return err
		}
		distributed = roundTokenAmount(distributed + share)
	}

	return nil
}
