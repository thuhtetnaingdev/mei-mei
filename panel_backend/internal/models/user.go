package models

import "time"

type User struct {
	ID                   uint                      `json:"id" gorm:"primaryKey"`
	UUID                 string                    `json:"uuid" gorm:"uniqueIndex;not null"`
	Email                string                    `json:"email" gorm:"uniqueIndex;not null"`
	Enabled              bool                      `json:"enabled" gorm:"default:true"`
	IsTesting            bool                      `json:"isTesting" gorm:"default:false"`
	ExpiresAt            *time.Time                `json:"expiresAt"`
	BandwidthLimitGB     int64                     `json:"bandwidthLimitGb"`
	BandwidthUsedBytes   int64                     `json:"bandwidthUsedBytes"`
	LastWeekMeteredBytes int64                     `json:"lastWeekMeteredBytes" gorm:"default:0"`
	TokenBalance         float64                   `json:"tokenBalance"`
	Notes                string                    `json:"notes"`
	UserType             string                    `json:"userType" gorm:"default:'unknown'"`
	LastClassifiedAt     *time.Time                `json:"lastClassifiedAt"`
	BandwidthAllocations []UserBandwidthAllocation `json:"bandwidthAllocations" gorm:"constraint:OnDelete:CASCADE;"`
	CreatedAt            time.Time                 `json:"createdAt"`
	UpdatedAt            time.Time                 `json:"updatedAt"`
}

type UserBandwidthAllocation struct {
	ID                      uint                     `json:"id" gorm:"primaryKey"`
	UserID                  uint                     `json:"userId" gorm:"index;not null"`
	TotalBandwidthBytes     int64                    `json:"totalBandwidthBytes" gorm:"not null"`
	RemainingBandwidthBytes int64                    `json:"remainingBandwidthBytes" gorm:"not null"`
	TokenAmount             float64                  `json:"tokenAmount" gorm:"not null"`
	RemainingTokens         float64                  `json:"remainingTokens" gorm:"not null"`
	AdminPercent            float64                  `json:"adminPercent"`
	UsagePoolPercent        float64                  `json:"usagePoolPercent"`
	ReservePoolPercent      float64                  `json:"reservePoolPercent"`
	AdminAmount             float64                  `json:"adminAmount"`
	UsagePoolTotal          float64                  `json:"usagePoolTotal"`
	UsagePoolDistributed    float64                  `json:"usagePoolDistributed"`
	ReservePoolTotal        float64                  `json:"reservePoolTotal"`
	ReservePoolDistributed  float64                  `json:"reservePoolDistributed"`
	SettlementStatus        string                   `json:"settlementStatus"`
	SettlementWarning       string                   `json:"settlementWarning"`
	SettledAt               *time.Time               `json:"settledAt"`
	ExpiresAt               *time.Time               `json:"expiresAt"`
	NodeUsages              []UserBandwidthNodeUsage `json:"nodeUsages" gorm:"foreignKey:AllocationID;constraint:OnDelete:CASCADE;"`
	CreatedAt               time.Time                `json:"createdAt"`
	UpdatedAt               time.Time                `json:"updatedAt"`
}

type UserBandwidthNodeUsage struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	AllocationID   uint      `json:"allocationId" gorm:"index;not null"`
	UserID         uint      `json:"userId" gorm:"index;not null"`
	NodeID         uint      `json:"nodeId" gorm:"index;not null"`
	MinerID        *uint     `json:"minerId" gorm:"index"`
	BandwidthBytes int64     `json:"bandwidthBytes" gorm:"not null"`
	RewardedTokens float64   `json:"rewardedTokens" gorm:"not null"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type UserRecord struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"userId" gorm:"index;not null"`
	Action    string    `json:"action" gorm:"not null"`
	Title     string    `json:"title" gorm:"not null"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PublicUserResponse represents a sanitized user object for public access
// It excludes sensitive fields like tokenBalance, notes, and internal allocation details
type PublicUserResponse struct {
	ID                 uint       `json:"id"`
	UUID               string     `json:"uuid"`
	Email              string     `json:"email"`
	Enabled            bool       `json:"enabled"`
	IsTesting          bool       `json:"isTesting"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	BandwidthLimitGB   int64      `json:"bandwidthLimitGb"`
	BandwidthUsedBytes int64      `json:"bandwidthUsedBytes"`
	UserType           string     `json:"userType"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	// Computed fields for user convenience
	BandwidthRemainingGB int64   `json:"bandwidthRemainingGb"`
	UsagePercentage      float64 `json:"usagePercentage"`
	SubscriptionURL      string  `json:"subscriptionUrl,omitempty"`
	SingboxImportURL     string  `json:"singboxImportUrl,omitempty"`
	HiddifyImportURL     string  `json:"hiddifyImportUrl,omitempty"`
	SingboxProfileURL    string  `json:"singboxProfileUrl,omitempty"`
	ClashProfileURL      string  `json:"clashProfileUrl,omitempty"`
}
