package services

import (
	"errors"
	"panel_backend/internal/models"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type MinerService struct {
	db *gorm.DB
}

type CreateMinerInput struct {
	Name          string `json:"name" binding:"required"`
	WalletAddress string `json:"walletAddress" binding:"required"`
	Notes         string `json:"notes"`
	NodeIDs       []uint `json:"nodeIds"`
}

type UpdateMinerInput struct {
	Name          *string `json:"name"`
	WalletAddress *string `json:"walletAddress"`
	Notes         *string `json:"notes"`
	NodeIDs       *[]uint `json:"nodeIds"`
}

func NewMinerService(db *gorm.DB) *MinerService {
	return &MinerService{db: db}
}

func (s *MinerService) List() ([]models.Miner, error) {
	var miners []models.Miner
	err := s.db.Preload("Nodes").Order("created_at desc").Find(&miners).Error
	return miners, err
}

func (s *MinerService) GetByID(id string) (*models.Miner, error) {
	var miner models.Miner
	err := s.db.Preload("Nodes").First(&miner, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &miner, nil
}

func (s *MinerService) Create(input CreateMinerInput) (*models.Miner, error) {
	name := strings.TrimSpace(input.Name)
	walletAddress := strings.TrimSpace(input.WalletAddress)
	if name == "" {
		return nil, errors.New("miner name is required")
	}
	if walletAddress == "" {
		return nil, errors.New("wallet address is required")
	}

	var miner models.Miner
	err := s.db.Transaction(func(tx *gorm.DB) error {
		miner = models.Miner{
			Name:          name,
			WalletAddress: walletAddress,
			Notes:         strings.TrimSpace(input.Notes),
		}
		if err := tx.Create(&miner).Error; err != nil {
			return err
		}

		return s.replaceMinerNodes(tx, miner.ID, input.NodeIDs)
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(stringID(miner.ID))
}

func (s *MinerService) Update(id string, input UpdateMinerInput) (*models.Miner, error) {
	var miner models.Miner
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&miner, "id = ?", id).Error; err != nil {
			return err
		}

		if input.Name != nil {
			name := strings.TrimSpace(*input.Name)
			if name == "" {
				return errors.New("miner name is required")
			}
			miner.Name = name
		}
		if input.WalletAddress != nil {
			walletAddress := strings.TrimSpace(*input.WalletAddress)
			if walletAddress == "" {
				return errors.New("wallet address is required")
			}
			miner.WalletAddress = walletAddress
		}
		if input.Notes != nil {
			miner.Notes = strings.TrimSpace(*input.Notes)
		}

		if err := tx.Save(&miner).Error; err != nil {
			return err
		}
		if input.NodeIDs != nil {
			return s.replaceMinerNodes(tx, miner.ID, *input.NodeIDs)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

func (s *MinerService) Delete(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var miner models.Miner
		if err := tx.First(&miner, "id = ?", id).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.Node{}).
			Where("miner_id = ?", miner.ID).
			Update("miner_id", nil).Error; err != nil {
			return err
		}

		result := tx.Delete(&models.Miner{}, "id = ?", id)
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return result.Error
	})
}

func (s *MinerService) replaceMinerNodes(tx *gorm.DB, minerID uint, nodeIDs []uint) error {
	if err := tx.Model(&models.Node{}).
		Where("miner_id = ?", minerID).
		Update("miner_id", nil).Error; err != nil {
		return err
	}

	if len(nodeIDs) == 0 {
		return nil
	}

	return tx.Model(&models.Node{}).
		Where("id IN ?", nodeIDs).
		Update("miner_id", minerID).Error
}

func stringID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
