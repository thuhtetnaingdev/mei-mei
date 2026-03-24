package services

import (
	"fmt"
	"log"
	"panel_backend/internal/models"
	"sync"
	"time"

	"gorm.io/gorm"
)

// User classification thresholds (in bytes per week)
const (
	GB                     = 1024 * 1024 * 1024
	LightUserThresholdBytes   = 25 * GB   // 25 GB/week
	MediumUserThresholdBytes  = 30 * GB   // 30 GB/week
	ModerateUserThresholdBytes = 30 * GB  // > 30 GB/week
)

// ClassificationTimeThreshold is the minimum time between classifications
const ClassificationTimeThreshold = 7 * 24 * time.Hour

// UserType represents the classification of a user based on bandwidth usage
type UserType string

const (
	UserTypeUnknown  UserType = "unknown"
	UserTypeLight    UserType = "light"
	UserTypeMedium   UserType = "medium"
	UserTypeModerate UserType = "moderate"
)

// UserClassification represents the result of classifying a user
type UserClassification struct {
	UserID        uint      `json:"userId"`
	UUID          string    `json:"uuid"`
	Email         string    `json:"email"`
	PreviousType  string    `json:"previousType"`
	NewType       string    `json:"newType"`
	BandwidthUsed int64     `json:"bandwidthUsed"`
	ClassifiedAt  time.Time `json:"classifiedAt"`
}

// ClassificationStats represents statistics about user classifications
type ClassificationStats struct {
	TotalUsers      int64              `json:"totalUsers"`
	LightUsers      int64              `json:"lightUsers"`
	MediumUsers     int64              `json:"mediumUsers"`
	ModerateUsers   int64              `json:"moderateUsers"`
	UnknownUsers    int64              `json:"unknownUsers"`
	LastClassified  *time.Time         `json:"lastClassified"`
	Classifications []UserClassification `json:"classifications,omitempty"`
}

// UserClassificationService handles user classification based on bandwidth usage
type UserClassificationService struct {
	db *gorm.DB

	mu              sync.RWMutex
	lastRunTime     time.Time
	lastRunErr      error
	consecutiveErrs int
	isRunning       bool
}

// NewUserClassificationService creates a new user classification service
func NewUserClassificationService(db *gorm.DB) *UserClassificationService {
	return &UserClassificationService{
		db: db,
	}
}

// classifyUser determines the user type based on their weekly bandwidth usage
func (s *UserClassificationService) classifyUser(weeklyUsageBytes int64) UserType {
	if weeklyUsageBytes < LightUserThresholdBytes {
		return UserTypeLight
	} else if weeklyUsageBytes < MediumUserThresholdBytes {
		return UserTypeMedium
	}
	return UserTypeModerate
}

// ClassifyUser classifies a single user and updates their classification in the database
func (s *UserClassificationService) ClassifyUser(user *models.User) (*UserClassification, error) {
	previousType := user.UserType
	now := time.Now()

	// Skip classification if too soon after last classification (enforce 7-day minimum)
	if user.LastClassifiedAt != nil {
		timeSinceLastClassify := now.Sub(*user.LastClassifiedAt)
		if timeSinceLastClassify < ClassificationTimeThreshold {
			remainingTime := ClassificationTimeThreshold - timeSinceLastClassify
			log.Printf("[user-classification] skipping user %s: only %.1f days since last classification (%.1f days remaining)",
				user.UUID, timeSinceLastClassify.Hours()/24, remainingTime.Hours()/24)
			return nil, nil // Skip, not an error
		}
	}

	// Calculate weekly usage (normalized to 7-day period)
	var weeklyUsageBytes int64
	if user.LastWeekMeteredBytes > 0 && user.LastClassifiedAt != nil {
		// Calculate bytes used since last classification
		bytesSinceLastClassify := user.BandwidthUsedBytes - user.LastWeekMeteredBytes
		// Handle negative usage (data reset/corruption)
		if bytesSinceLastClassify < 0 {
			log.Printf("[user-classification] negative usage detected for user %s (%d bytes), resetting to current usage",
				user.UUID, bytesSinceLastClassify)
			bytesSinceLastClassify = user.BandwidthUsedBytes
		}
		
		// Calculate time elapsed since last classification
		timeSinceLastClassify := now.Sub(*user.LastClassifiedAt)
		hoursSinceLastClassify := timeSinceLastClassify.Hours()
		
		// Normalize to weekly rate (168 hours = 7 days)
		if hoursSinceLastClassify > 0 {
			weeklyUsageBytes = int64(float64(bytesSinceLastClassify) * (168.0 / hoursSinceLastClassify))
			log.Printf("[user-classification] user %s: %d bytes in %.1f hours, normalized to %d bytes/week",
				user.UUID, bytesSinceLastClassify, hoursSinceLastClassify, weeklyUsageBytes)
		} else {
			weeklyUsageBytes = bytesSinceLastClassify
		}
	} else {
		// First classification - use current as baseline
		log.Printf("[user-classification] first-time classification for user %s, using current usage as baseline", user.UUID)
		weeklyUsageBytes = user.BandwidthUsedBytes
	}

	// Classify based on weekly usage
	newType := string(s.classifyUser(weeklyUsageBytes))

	// Update user classification and metered bytes in database
	updates := map[string]interface{}{
		"user_type":              newType,
		"last_classified_at":     &now,
		"last_week_metered_bytes": user.BandwidthUsedBytes,
	}

	if err := s.db.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update user classification: %w", err)
	}

	// Update the user object in memory
	user.UserType = newType
	user.LastClassifiedAt = &now
	user.LastWeekMeteredBytes = user.BandwidthUsedBytes

	return &UserClassification{
		UserID:        user.ID,
		UUID:          user.UUID,
		Email:         user.Email,
		PreviousType:  previousType,
		NewType:       newType,
		BandwidthUsed: user.BandwidthUsedBytes,
		ClassifiedAt:  now,
	}, nil
}

// ClassifyAllUsers classifies all users in the database
func (s *UserClassificationService) ClassifyAllUsers() ([]UserClassification, error) {
	log.Printf("[user-classification] starting classification for all users")

	var users []models.User
	if err := s.db.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", err)
	}

	if len(users) == 0 {
		log.Printf("[user-classification] no users to classify")
		return []UserClassification{}, nil
	}

	classifications := make([]UserClassification, 0, len(users))
	for i := range users {
		classification, err := s.ClassifyUser(&users[i])
		if err != nil {
			log.Printf("[user-classification] failed to classify user %s: %v", users[i].UUID, err)
			continue
		}
		if classification == nil {
			// Skipped due to 7-day minimum interval
			continue
		}
		classifications = append(classifications, *classification)
		log.Printf("[user-classification] classified user %s (%s) as %s (was %s), usage: %d bytes",
			users[i].Email, users[i].UUID, classification.NewType, classification.PreviousType, users[i].BandwidthUsedBytes)
	}

	s.recordSuccess()
	log.Printf("[user-classification] completed classification for %d users", len(classifications))

	return classifications, nil
}

// GetClassificationStats returns statistics about user classifications
func (s *UserClassificationService) GetClassificationStats() (*ClassificationStats, error) {
	var totalUsers, lightUsers, mediumUsers, moderateUsers, unknownUsers int64

	// Count users by type
	if err := s.db.Model(&models.User{}).Where("user_type = ?", UserTypeLight).Count(&lightUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.User{}).Where("user_type = ?", UserTypeMedium).Count(&mediumUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.User{}).Where("user_type = ?", UserTypeModerate).Count(&moderateUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.User{}).Where("user_type = ? OR user_type = ''", UserTypeUnknown).Count(&unknownUsers).Error; err != nil {
		return nil, err
	}

	totalUsers = lightUsers + mediumUsers + moderateUsers + unknownUsers

	// Get the most recent classification time
	var lastClassified *time.Time
	var user models.User
	if err := s.db.Where("last_classified_at IS NOT NULL").Order("last_classified_at DESC").First(&user).Error; err == nil {
		lastClassified = user.LastClassifiedAt
	}

	return &ClassificationStats{
		TotalUsers:     totalUsers,
		LightUsers:     lightUsers,
		MediumUsers:    mediumUsers,
		ModerateUsers:  moderateUsers,
		UnknownUsers:   unknownUsers,
		LastClassified: lastClassified,
	}, nil
}

// GetStatus returns the current status of the classification service
func (s *UserClassificationService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastRunAt string
	if !s.lastRunTime.IsZero() {
		lastRunAt = s.lastRunTime.Format(time.RFC3339)
	}

	return map[string]interface{}{
		"isRunning":       s.isRunning,
		"lastRunAt":       lastRunAt,
		"lastRunErr":      s.lastRunErr,
		"consecutiveErrs": s.consecutiveErrs,
	}
}

// recordSuccess records a successful classification run
func (s *UserClassificationService) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRunTime = time.Now()
	s.lastRunErr = nil
	s.consecutiveErrs = 0
}

// recordError records a failed classification run
func (s *UserClassificationService) recordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRunTime = time.Now()
	s.lastRunErr = err
	s.consecutiveErrs++
}

// ForceClassify triggers an immediate classification of all users
func (s *UserClassificationService) ForceClassify() ([]UserClassification, error) {
	log.Printf("[user-classification] triggering forced classification")
	return s.ClassifyAllUsers()
}
