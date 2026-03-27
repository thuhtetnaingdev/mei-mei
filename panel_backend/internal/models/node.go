package models

import "time"

type Node struct {
	ID                     uint       `json:"id" gorm:"primaryKey"`
	MinerID                *uint      `json:"minerId"`
	Name                   string     `json:"name" gorm:"uniqueIndex;not null"`
	BaseURL                string     `json:"baseUrl" gorm:"not null"`
	Location               string     `json:"location"`
	SSHHost                string     `json:"sshHost"`
	SSHPort                int        `json:"sshPort"`
	SSHUsername            string     `json:"sshUsername"`
	SSHPrivateKey          string     `json:"-" gorm:"type:text"`
	SSHPublicKey           string     `json:"-" gorm:"type:text"`
	PublicHost             string     `json:"publicHost" gorm:"not null"`
	VLESSPort              int        `json:"vlessPort"`
	TUICPort               int        `json:"tuicPort"`
	Hysteria2Port          int        `json:"hysteria2Port"`
	ExpiresAt              *time.Time `json:"expiresAt"`
	BandwidthLimitGB       int64      `json:"bandwidthLimitGb"`
	BandwidthUsedBytes     int64      `json:"bandwidthUsedBytes"`
	RewardedTokens         float64    `json:"rewardedTokens"`
	RealityPublicKey       string     `json:"realityPublicKey"`
	RealityShortID         string     `json:"realityShortId"`
	RealityServerName      string     `json:"realityServerName"`
	RealityPrivateKeyHash  string     `json:"realityPrivateKeyHash" gorm:"type:varchar(64)"`
	ProtocolToken          string     `json:"-" gorm:"not null"`
	Enabled                bool       `json:"enabled" gorm:"default:true"`
	IsTestable             bool       `json:"isTestable" gorm:"default:false"`
	HealthStatus           string     `json:"healthStatus" gorm:"default:unknown"`
	SyncVerificationStatus string     `json:"syncVerificationStatus" gorm:"default:unknown"`
	SyncVerificationError  string     `json:"syncVerificationError"`
	SyncVerifiedAt         *time.Time `json:"syncVerifiedAt"`
	LastAppliedConfigHash  string     `json:"lastAppliedConfigHash"`
	AppliedUserCount       int        `json:"appliedUserCount"`
	LastConfigAppliedAt    *time.Time `json:"lastConfigAppliedAt"`
	LastHeartbeat          *time.Time `json:"lastHeartbeat"`
	LastSyncAt             *time.Time `json:"lastSyncAt"`
	LastKeyVerificationAt  *time.Time `json:"lastKeyVerificationAt"`
	KeyMismatchDetectedAt  *time.Time `json:"keyMismatchDetectedAt"`
	KeyMismatchAutoFixedAt *time.Time `json:"keyMismatchAutoFixedAt"`
	SingboxVersion         string     `json:"singboxVersion"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}
