package models

import "time"

type ClashSetting struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	UserID              uint      `json:"userId" gorm:"uniqueIndex"`
	NodeMode            string    `json:"nodeMode" gorm:"default:sub_integration"`
	Fallback            bool      `json:"fallback"`
	AutoInterval        int       `json:"autoInterval" gorm:"default:600"`
	AutoTolerance       int       `json:"autoTolerance" gorm:"default:50"`
	AutoType            string    `json:"autoType" gorm:"default:url-test"`
	LoadBalanceStrategy string    `json:"loadBalanceStrategy" gorm:"default:round-robin"`
	AutoTimeout         int       `json:"autoTimeout" gorm:"default:2000"`
	AutoMaxFailed       int       `json:"autoMaxFailed" gorm:"default:1"`
	FallbackMode        string    `json:"fallbackMode" gorm:"default:nodes"`
	FallbackInterval    int       `json:"fallbackInterval" gorm:"default:10"`
	FallbackCount       int       `json:"fallbackCount" gorm:"default:10"`
	FallbackTolerance   int       `json:"fallbackTolerance" gorm:"default:50"`
	FallbackTimeout     int       `json:"fallbackTimeout" gorm:"default:2000"`
	FallbackMaxFailed   int       `json:"fallbackMaxFailed" gorm:"default:1"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
