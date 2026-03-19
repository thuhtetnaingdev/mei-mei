package services

import (
	"errors"
	"fmt"
	"panel_backend/internal/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserService struct {
	db *gorm.DB
}

type CreateUserInput struct {
	Email            string     `json:"email" binding:"required,email"`
	Enabled          *bool      `json:"enabled"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	BandwidthLimitGB int64      `json:"bandwidthLimitGb"`
	Notes            string     `json:"notes"`
}

type UpdateUserInput struct {
	Email            *string    `json:"email"`
	Enabled          *bool      `json:"enabled"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	BandwidthLimitGB *int64     `json:"bandwidthLimitGb"`
	Notes            *string    `json:"notes"`
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) Create(input CreateUserInput) (*models.User, error) {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	user := models.User{
		UUID:             uuid.NewString(),
		Email:            input.Email,
		Enabled:          enabled,
		ExpiresAt:        input.ExpiresAt,
		BandwidthLimitGB: input.BandwidthLimitGB,
		Notes:            input.Notes,
	}

	return &user, s.db.Create(&user).Error
}

func (s *UserService) List() ([]models.User, error) {
	var users []models.User
	err := s.db.Order("created_at desc").Find(&users).Error
	return users, err
}

func (s *UserService) GetByID(id string) (*models.User, error) {
	var user models.User
	err := s.db.First(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserService) GetByUUID(uuid string) (*models.User, error) {
	var user models.User
	err := s.db.First(&user, "uuid = ?", uuid).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserService) Delete(id string) error {
	result := s.db.Delete(&models.User{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func (s *UserService) Update(id string, input UpdateUserInput) (*models.User, error) {
	user, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	if input.Enabled != nil {
		user.Enabled = *input.Enabled
	}
	if input.Email != nil && *input.Email != "" {
		user.Email = *input.Email
	}
	if input.ExpiresAt != nil {
		user.ExpiresAt = input.ExpiresAt
	}
	if input.BandwidthLimitGB != nil {
		user.BandwidthLimitGB = *input.BandwidthLimitGB
	}
	if input.Notes != nil {
		user.Notes = *input.Notes
	}

	// If the user was auto-disabled after exceeding bandwidth, raising the limit
	// should bring them back automatically. The frontend currently sends the
	// existing enabled state on edits, so treat an unchanged false value as
	// incidental rather than an explicit "keep disabled" instruction.
	enabledUnchanged := input.Enabled == nil || *input.Enabled == false
	if input.BandwidthLimitGB != nil && !user.Enabled && enabledUnchanged {
		withinLimit := user.BandwidthLimitGB == 0 ||
			user.BandwidthUsedBytes < user.BandwidthLimitGB*1024*1024*1024
		notExpired := user.ExpiresAt == nil || user.ExpiresAt.After(time.Now())
		if withinLimit && notExpired {
			user.Enabled = true
		}
	}

	return user, s.db.Save(user).Error
}

func (s *UserService) ActiveUsers() ([]models.User, error) {
	var users []models.User
	now := time.Now()
	err := s.db.
		Where("enabled = ?", true).
		Where("expires_at IS NULL OR expires_at > ?", now).
		Where("bandwidth_limit_gb = 0 OR bandwidth_used_bytes < bandwidth_limit_gb * 1024 * 1024 * 1024").
		Find(&users).Error
	return users, err
}

func (s *UserService) AddUsage(uuid string, bytes int64) error {
	if uuid == "" || bytes <= 0 {
		return errors.New("invalid usage payload")
	}

	return s.db.Model(&models.User{}).
		Where("uuid = ?", uuid).
		UpdateColumn("bandwidth_used_bytes", gorm.Expr("bandwidth_used_bytes + ?", bytes)).Error
}

func (s *UserService) AddUsageAndDisableIfLimitReached(uuid string, bytes int64) (bool, error) {
	if uuid == "" || bytes <= 0 {
		return false, errors.New("invalid usage payload")
	}

	disabled := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uuid = ?", uuid).
			First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		newUsage := user.BandwidthUsedBytes + bytes
		updates := map[string]interface{}{
			"bandwidth_used_bytes": newUsage,
		}

		if user.Enabled && user.BandwidthLimitGB > 0 {
			limitBytes := user.BandwidthLimitGB * 1024 * 1024 * 1024
			if newUsage >= limitBytes {
				updates["enabled"] = false
				disabled = true
			}
		}

		result := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("user %s was not updated", uuid)
		}

		return nil
	})

	return disabled, err
}
