package models

import "time"

type AdminSetting struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	Username           string    `json:"username" gorm:"uniqueIndex;not null"`
	PasswordHash       string    `json:"-"`
	AdminPercent       float64   `json:"adminPercent"`
	UsagePoolPercent   float64   `json:"usagePoolPercent"`
	ReservePoolPercent float64   `json:"reservePoolPercent"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
