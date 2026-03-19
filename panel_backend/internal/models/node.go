package models

import "time"

type Node struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	Name               string     `json:"name" gorm:"uniqueIndex;not null"`
	BaseURL            string     `json:"baseUrl" gorm:"not null"`
	Location           string     `json:"location"`
	SSHHost            string     `json:"sshHost"`
	SSHPort            int        `json:"sshPort"`
	SSHUsername        string     `json:"sshUsername"`
	SSHPrivateKey      string     `json:"-" gorm:"type:text"`
	SSHPublicKey       string     `json:"-" gorm:"type:text"`
	PublicHost         string     `json:"publicHost" gorm:"not null"`
	VLESSPort          int        `json:"vlessPort"`
	TUICPort           int        `json:"tuicPort"`
	Hysteria2Port      int        `json:"hysteria2Port"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	BandwidthLimitGB   int64      `json:"bandwidthLimitGb"`
	BandwidthUsedBytes int64      `json:"bandwidthUsedBytes"`
	RealityPublicKey   string     `json:"realityPublicKey"`
	RealityShortID     string     `json:"realityShortId"`
	RealityServerName  string     `json:"realityServerName"`
	ProtocolToken      string     `json:"-" gorm:"not null"`
	HealthStatus       string     `json:"healthStatus" gorm:"default:unknown"`
	LastHeartbeat      *time.Time `json:"lastHeartbeat"`
	LastSyncAt         *time.Time `json:"lastSyncAt"`
	SingboxVersion     string     `json:"singboxVersion"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
