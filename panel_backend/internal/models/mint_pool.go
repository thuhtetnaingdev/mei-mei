package models

import "time"

type MintPoolState struct {
	ID                      uint       `json:"id" gorm:"primaryKey"`
	TotalMMKReserve         int64      `json:"totalMmkReserve"`
	TotalMeiMinted          int64      `json:"totalMeiMinted"`
	MainWalletBalance       float64    `json:"mainWalletBalance"`
	AdminWalletBalance      float64    `json:"adminWalletBalance"`
	TotalTransferredToUsers float64    `json:"totalTransferredToUsers"`
	TotalRewardedToMiners   float64    `json:"totalRewardedToMiners"`
	TotalAdminCollected     float64    `json:"totalAdminCollected"`
	LastMintAt              *time.Time `json:"lastMintAt"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

type MintPoolEvent struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	MMKAmount    int64     `json:"mmkAmount" gorm:"not null"`
	MeiAmount    int64     `json:"meiAmount" gorm:"not null"`
	ExchangeRate string    `json:"exchangeRate" gorm:"not null;default:1:1"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"createdAt"`
}

type MintPoolTransferEvent struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	TransferType string    `json:"transferType" gorm:"not null"`
	FromWallet   string    `json:"fromWallet" gorm:"not null"`
	ToWallet     string    `json:"toWallet" gorm:"not null"`
	Amount       float64   `json:"amount" gorm:"not null"`
	UserID       *uint     `json:"userId"`
	MinerID      *uint     `json:"minerId"`
	NodeID       *uint     `json:"nodeId"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
