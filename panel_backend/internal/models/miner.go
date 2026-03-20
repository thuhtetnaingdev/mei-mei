package models

import "time"

type Miner struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Name           string    `json:"name" gorm:"uniqueIndex;not null"`
	WalletAddress  string    `json:"walletAddress" gorm:"not null"`
	RewardedTokens float64   `json:"rewardedTokens"`
	Notes          string    `json:"notes"`
	Nodes          []Node    `json:"nodes" gorm:"foreignKey:MinerID"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type MinerReward struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	MinerID        uint      `json:"minerId" gorm:"index;not null"`
	NodeID         uint      `json:"nodeId" gorm:"index;not null"`
	UserID         uint      `json:"userId" gorm:"index;not null"`
	AllocationID   *uint     `json:"allocationId" gorm:"index"`
	BandwidthBytes int64     `json:"bandwidthBytes" gorm:"not null"`
	RewardedTokens float64   `json:"rewardedTokens" gorm:"not null"`
	RewardSource   string    `json:"rewardSource" gorm:"not null;default:usage_pool"`
	CreatedAt      time.Time `json:"createdAt"`
}
