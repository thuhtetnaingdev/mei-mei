package services

import (
	"errors"
	"panel_backend/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
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
