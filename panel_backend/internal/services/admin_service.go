package services

import (
	"errors"
	"fmt"
	"panel_backend/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	DefaultAdminPercent     = 25.0
	DefaultUsagePoolPercent = 55.0
	DefaultReservePercent   = 20.0
)

type AdminService struct {
	db              *gorm.DB
	defaultUsername string
	defaultPassword string
}

type UpdateAdminCredentialsInput struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	Username        string `json:"username" binding:"required"`
	Password        string `json:"password" binding:"required,min=4"`
}

type DistributionSettings struct {
	AdminPercent       float64 `json:"adminPercent"`
	UsagePoolPercent   float64 `json:"usagePoolPercent"`
	ReservePoolPercent float64 `json:"reservePoolPercent"`
}

func NewAdminService(db *gorm.DB, defaultUsername, defaultPassword string) *AdminService {
	return &AdminService{
		db:              db,
		defaultUsername: defaultUsername,
		defaultPassword: defaultPassword,
	}
}

func (s *AdminService) ValidateCredentials(username, password string) bool {
	admin, err := s.getStoredAdmin()
	if err != nil {
		return username == s.defaultUsername && password == s.defaultPassword
	}

	if admin.Username != username {
		return false
	}

	return bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) == nil
}

func (s *AdminService) GetProfile() map[string]string {
	admin, err := s.getStoredAdmin()
	if err != nil {
		return map[string]string{"username": s.defaultUsername}
	}

	return map[string]string{"username": admin.Username}
}

func (s *AdminService) GetDistributionSettings() (DistributionSettings, error) {
	admin, err := s.getOrCreateAdminSettings()
	if err != nil {
		return DistributionSettings{}, err
	}

	return DistributionSettings{
		AdminPercent:       normalizedPercent(admin.AdminPercent, DefaultAdminPercent),
		UsagePoolPercent:   normalizedPercent(admin.UsagePoolPercent, DefaultUsagePoolPercent),
		ReservePoolPercent: normalizedPercent(admin.ReservePoolPercent, DefaultReservePercent),
	}, nil
}

func (s *AdminService) UpdateDistributionSettings(input DistributionSettings) (DistributionSettings, error) {
	if err := validateDistributionSettings(input); err != nil {
		return DistributionSettings{}, err
	}

	admin, err := s.getOrCreateAdminSettings()
	if err != nil {
		return DistributionSettings{}, err
	}

	admin.AdminPercent = input.AdminPercent
	admin.UsagePoolPercent = input.UsagePoolPercent
	admin.ReservePoolPercent = input.ReservePoolPercent

	if err := s.db.Save(admin).Error; err != nil {
		return DistributionSettings{}, err
	}

	return s.GetDistributionSettings()
}

func (s *AdminService) UpdateCredentials(input UpdateAdminCredentialsInput) (map[string]string, error) {
	if !s.ValidateCredentials(s.currentUsername(), input.CurrentPassword) {
		return nil, errors.New("current password is incorrect")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	admin, err := s.getStoredAdmin()
	if err != nil {
		admin = &models.AdminSetting{}
	}

	admin.Username = input.Username
	admin.PasswordHash = string(passwordHash)

	if err := s.db.Save(admin).Error; err != nil {
		return nil, err
	}

	return map[string]string{"username": admin.Username}, nil
}

func (s *AdminService) currentUsername() string {
	admin, err := s.getStoredAdmin()
	if err != nil {
		return s.defaultUsername
	}
	return admin.Username
}

func (s *AdminService) getStoredAdmin() (*models.AdminSetting, error) {
	var admin models.AdminSetting
	err := s.db.First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

func (s *AdminService) getOrCreateAdminSettings() (*models.AdminSetting, error) {
	admin, err := s.getStoredAdmin()
	if err == nil {
		changed := false
		if admin.AdminPercent == 0 && admin.UsagePoolPercent == 0 && admin.ReservePoolPercent == 0 {
			admin.AdminPercent = DefaultAdminPercent
			admin.UsagePoolPercent = DefaultUsagePoolPercent
			admin.ReservePoolPercent = DefaultReservePercent
			changed = true
		}
		if changed {
			if err := s.db.Save(admin).Error; err != nil {
				return nil, err
			}
		}
		return admin, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	admin = &models.AdminSetting{
		Username:           s.defaultUsername,
		AdminPercent:       DefaultAdminPercent,
		UsagePoolPercent:   DefaultUsagePoolPercent,
		ReservePoolPercent: DefaultReservePercent,
	}
	if err := s.db.Create(admin).Error; err != nil {
		return nil, err
	}
	return admin, nil
}

func validateDistributionSettings(input DistributionSettings) error {
	if input.AdminPercent < 0 || input.UsagePoolPercent < 0 || input.ReservePoolPercent < 0 {
		return errors.New("distribution percentages cannot be negative")
	}
	total := input.AdminPercent + input.UsagePoolPercent + input.ReservePoolPercent
	if total != 100 {
		return fmt.Errorf("distribution percentages must total 100.00, got %.2f", total)
	}
	return nil
}

func normalizedPercent(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}
