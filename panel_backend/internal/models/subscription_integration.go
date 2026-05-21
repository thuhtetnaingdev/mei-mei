package models

import "time"

const (
	IntegrationStatusPending   = "pending"
	IntegrationStatusTesting   = "testing"
	IntegrationStatusCompleted = "completed"
	IntegrationStatusFailed    = "failed"
)

type SubscriptionIntegration struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	SubscriptionURL    string     `json:"subscriptionUrl" gorm:"not null"`
	Result             string     `json:"-" gorm:"type:text"`
	WorkingCount       int        `json:"workingCount"`
	TotalCount         int        `json:"totalCount"`
	Status             string     `json:"status" gorm:"default:'pending'"`
	ErrorMessage       string     `json:"errorMessage,omitempty"`
	TestRunID          string     `json:"testRunId,omitempty"`
	LastTestStartedAt  *time.Time `json:"lastTestStartedAt,omitempty"`
	LastTestCompletedAt *time.Time `json:"lastTestCompletedAt,omitempty"`
	NextTestAt         *time.Time `json:"nextTestAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type IntegrationDetail struct {
	ID                  uint              `json:"id"`
	SubscriptionURL     string            `json:"subscriptionUrl"`
	WorkingCount        int               `json:"workingCount"`
	TotalCount          int               `json:"totalCount"`
	FailCount           int               `json:"failCount"`
	Status              string            `json:"status"`
	Working             []map[string]any  `json:"working"`
	Page                int               `json:"page"`
	PageSize            int               `json:"pageSize"`
	TotalPages          int               `json:"totalPages"`
	LastTestStartedAt   *time.Time        `json:"lastTestStartedAt,omitempty"`
	LastTestCompletedAt *time.Time        `json:"lastTestCompletedAt,omitempty"`
	NextTestAt          *time.Time        `json:"nextTestAt,omitempty"`
	CreatedAt           time.Time         `json:"createdAt"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}
