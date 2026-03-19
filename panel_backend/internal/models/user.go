package models

import "time"

type User struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	UUID               string     `json:"uuid" gorm:"uniqueIndex;not null"`
	Email              string     `json:"email" gorm:"uniqueIndex;not null"`
	Enabled            bool       `json:"enabled" gorm:"default:true"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	BandwidthLimitGB   int64      `json:"bandwidthLimitGb"`
	BandwidthUsedBytes int64      `json:"bandwidthUsedBytes"`
	Notes              string     `json:"notes"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
