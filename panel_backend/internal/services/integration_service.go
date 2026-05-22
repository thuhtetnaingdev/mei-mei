package services

import (
	"encoding/json"
	"fmt"
	"math"
	"panel_backend/internal/models"
	"time"

	"gorm.io/gorm"
)

type IntegrationService struct {
	db *gorm.DB
}

func NewIntegrationService(db *gorm.DB) *IntegrationService {
	return &IntegrationService{db: db}
}

func (s *IntegrationService) Create(url, resultJSON string, workingCount, totalCount int) (*models.SubscriptionIntegration, error) {
	now := time.Now()
	integ := &models.SubscriptionIntegration{
		SubscriptionURL: url,
		Result:          resultJSON,
		WorkingCount:    workingCount,
		TotalCount:      totalCount,
		Status:          models.IntegrationStatusPending,
		NextTestAt:      &now,
	}
	if err := s.db.Create(integ).Error; err != nil {
		return nil, fmt.Errorf("create integration: %w", err)
	}
	return integ, nil
}

func (s *IntegrationService) List() ([]models.SubscriptionIntegration, error) {
	var list []models.SubscriptionIntegration
	if err := s.db.Order("created_at desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *IntegrationService) GetByID(id string) (*models.SubscriptionIntegration, error) {
	var integ models.SubscriptionIntegration
	if err := s.db.First(&integ, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("integration not found")
		}
		return nil, err
	}
	return &integ, nil
}

func (s *IntegrationService) GetDetail(id string, page, pageSize int) (*models.IntegrationDetail, error) {
	integ, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Working []map[string]any `json:"working"`
		Tested  []map[string]any `json:"tested"`
	}
	json.Unmarshal([]byte(integ.Result), &parsed)

	totalWorking := len(parsed.Working)
	totalPages := int(math.Ceil(float64(totalWorking) / float64(pageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	start := (page - 1) * pageSize
	if start >= totalWorking {
		parsed.Working = nil
	} else {
		end := start + pageSize
		if end > totalWorking {
			end = totalWorking
		}
		parsed.Working = parsed.Working[start:end]
	}

	failCount := integ.TotalCount - totalWorking

	return &models.IntegrationDetail{
		ID:                 integ.ID,
		SubscriptionURL:    integ.SubscriptionURL,
		WorkingCount:       totalWorking,
		TotalCount:         integ.TotalCount,
		FailCount:          failCount,
		Status:             integ.Status,
		Working:            parsed.Working,
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
		LastTestStartedAt:  integ.LastTestStartedAt,
		LastTestCompletedAt: integ.LastTestCompletedAt,
		NextTestAt:         integ.NextTestAt,
		CreatedAt:          integ.CreatedAt,
		UpdatedAt:          integ.UpdatedAt,
	}, nil
}

func (s *IntegrationService) Delete(id string) error {
	return s.db.Delete(&models.SubscriptionIntegration{}, id).Error
}

func (s *IntegrationService) GetPendingForTest(limit int) ([]models.SubscriptionIntegration, error) {
	var list []models.SubscriptionIntegration
	now := time.Now()
	if err := s.db.Where(
		"status != ? AND (next_test_at IS NULL OR next_test_at <= ?)",
		models.IntegrationStatusTesting, now,
	).Order("next_test_at ASC, created_at ASC").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *IntegrationService) StartTest(id string, testRunID string) error {
	now := time.Now()
	result := s.db.Model(&models.SubscriptionIntegration{}).Where("id = ?", id).Updates(map[string]any{
		"status":              models.IntegrationStatusTesting,
		"test_run_id":         testRunID,
		"last_test_started_at": now,
		"result":              "",
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("integration %s not found", id)
	}
	return nil
}

func (s *IntegrationService) AppendTestResult(id, testRunID string, tested, working []map[string]any, workingCount, totalCount int) error {
	var integ models.SubscriptionIntegration
	if err := s.db.Where("id = ? AND test_run_id = ?", id, testRunID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("integration not found or testRunID mismatch")
		}
		return err
	}

	result := make(map[string]any)
	if integ.Result != "" {
		json.Unmarshal([]byte(integ.Result), &result)
	}

	existingTested, _ := result["tested"].([]any)
	for _, t := range tested {
		existingTested = append(existingTested, t)
	}
	result["tested"] = existingTested

	existingWorking, _ := result["working"].([]any)
	for _, w := range working {
		existingWorking = append(existingWorking, w)
	}
	result["working"] = existingWorking

	newResultBytes, _ := json.Marshal(result)

	return s.db.Model(&models.SubscriptionIntegration{}).Where(
		"id = ? AND test_run_id = ?", id, testRunID,
	).Updates(map[string]any{
		"result":        string(newResultBytes),
		"working_count": workingCount,
		"total_count":   totalCount,
	}).Error
}

func (s *IntegrationService) CompleteTest(id, testRunID, resultJSON string, workingCount, totalCount int, status string, errorMsg string) error {
	now := time.Now()
	intervalHours := 1
	nextTestAt := now.Add(time.Duration(intervalHours) * time.Hour)

	updates := map[string]any{
		"result":                resultJSON,
		"working_count":         workingCount,
		"total_count":           totalCount,
		"status":                status,
		"error_message":         errorMsg,
		"last_test_completed_at": now,
		"next_test_at":          nextTestAt,
	}

	result := s.db.Model(&models.SubscriptionIntegration{}).Where(
		"id = ? AND test_run_id = ?", id, testRunID,
	).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("integration %s not found or testRunID mismatch", id)
	}
	return nil
}
