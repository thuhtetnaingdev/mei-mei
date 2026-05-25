package models

import "time"

type ClashSetting struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	UserID              uint      `json:"userId" gorm:"uniqueIndex"`
	NodeMode            string    `json:"clashNodeMode" gorm:"default:sub_integration"`
	Fallback            bool      `json:"clashFallback"`
	AutoInterval        int       `json:"clashAutoInterval" gorm:"default:600"`
	AutoTolerance       int       `json:"clashAutoTolerance" gorm:"default:50"`
	AutoType            string    `json:"clashAutoType" gorm:"default:url-test"`
	LoadBalanceStrategy string    `json:"clashLoadBalanceStrategy" gorm:"default:round-robin"`
	AutoTimeout         int       `json:"clashAutoTimeout" gorm:"default:2000"`
	AutoMaxFailed       int       `json:"clashAutoMaxFailed" gorm:"default:1"`
	FallbackMode        string    `json:"clashFallbackMode" gorm:"default:nodes"`
	FallbackInterval    int       `json:"clashFallbackInterval" gorm:"default:10"`
	FallbackCount       int       `json:"clashFallbackCount" gorm:"default:10"`
	FallbackTolerance   int       `json:"clashFallbackTolerance" gorm:"default:50"`
	FallbackTimeout     int       `json:"clashFallbackTimeout" gorm:"default:2000"`
	FallbackMaxFailed   int       `json:"clashFallbackMaxFailed" gorm:"default:1"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
