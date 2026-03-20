package services

import (
	"errors"
	"panel_backend/internal/models"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MintPoolService struct {
	db *gorm.DB
}

type MintPoolMintInput struct {
	MMKAmount int64  `json:"mmkAmount" binding:"required,min=1"`
	Note      string `json:"note"`
	Approved  bool   `json:"approved"`
}

type MintPoolSnapshot struct {
	Pool      models.MintPoolState           `json:"pool"`
	History   []models.MintPoolEvent         `json:"history"`
	Transfers []models.MintPoolTransferEvent `json:"transfers"`
}

func NewMintPoolService(db *gorm.DB) *MintPoolService {
	return &MintPoolService{db: db}
}

func (s *MintPoolService) GetSnapshot() (*MintPoolSnapshot, error) {
	pool, err := s.getOrCreatePoolState()
	if err != nil {
		return nil, err
	}

	history := make([]models.MintPoolEvent, 0)
	if err := s.db.Order("created_at desc").Limit(25).Find(&history).Error; err != nil {
		return nil, err
	}

	transfers := make([]models.MintPoolTransferEvent, 0)
	if err := s.db.Order("updated_at desc, created_at desc").Limit(25).Find(&transfers).Error; err != nil {
		return nil, err
	}

	return &MintPoolSnapshot{
		Pool:      *pool,
		History:   history,
		Transfers: transfers,
	}, nil
}

func (s *MintPoolService) Mint(input MintPoolMintInput) (*MintPoolSnapshot, error) {
	if input.MMKAmount <= 0 {
		return nil, errors.New("mmk amount must be greater than zero")
	}
	if !input.Approved {
		return nil, errors.New("approval is required before minting")
	}

	note := strings.TrimSpace(input.Note)

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var state models.MintPoolState
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				state = models.MintPoolState{}
				if err := tx.Create(&state).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}

		now := time.Now()
		event := models.MintPoolEvent{
			MMKAmount:    input.MMKAmount,
			MeiAmount:    input.MMKAmount,
			ExchangeRate: "1:1",
			Note:         note,
			CreatedAt:    now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}

		state.TotalMMKReserve += input.MMKAmount
		state.TotalMeiMinted += input.MMKAmount
		state.MainWalletBalance += float64(input.MMKAmount)
		state.LastMintAt = &now

		return tx.Save(&state).Error
	})
	if err != nil {
		return nil, err
	}

	return s.GetSnapshot()
}

func (s *MintPoolService) Reset() (*MintPoolSnapshot, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.MintPoolEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.MintPoolTransferEvent{}).Error; err != nil {
			return err
		}

		pool, err := s.getOrCreatePoolStateTx(tx)
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

	return s.GetSnapshot()
}

func (s *MintPoolService) getOrCreatePoolState() (*models.MintPoolState, error) {
	return s.getOrCreatePoolStateTx(s.db)
}

func (s *MintPoolService) getOrCreatePoolStateTx(db *gorm.DB) (*models.MintPoolState, error) {
	var state models.MintPoolState
	err := db.First(&state).Error
	if err == nil {
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	state = models.MintPoolState{}
	if err := db.Create(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}
