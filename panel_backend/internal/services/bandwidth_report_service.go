package services

import (
	"errors"
	"fmt"
	"log"
	"panel_backend/internal/models"
	"time"

	"gorm.io/gorm"
)

// UserUsageReport represents bandwidth usage for a single user from a node report
type UserUsageReport struct {
	UUID      string `json:"uuid"`
	Email     string `json:"email"`
	BytesUsed int64  `json:"bytesUsed"`
}

// BandwidthReport represents a complete bandwidth report from a node
type BandwidthReport struct {
	NodeName     string            `json:"nodeName"`
	ReportPeriod time.Time         `json:"reportPeriod"`
	TotalBytes   int64             `json:"totalBytes"`
	UserUsage    []UserUsageReport `json:"userUsage"`
}

// BandwidthReportService handles processing of bandwidth reports from nodes
type BandwidthReportService struct {
	db *gorm.DB
}

// NewBandwidthReportService creates a new bandwidth report service
func NewBandwidthReportService(db *gorm.DB) *BandwidthReportService {
	return &BandwidthReportService{db: db}
}

// ProcessReport processes a bandwidth report from a node and updates user usage
func (s *BandwidthReportService) ProcessReport(report BandwidthReport, nodeName string) error {
	if report.NodeName == "" {
		report.NodeName = nodeName
	}

	// Validate report
	if err := s.validateReport(report); err != nil {
		return fmt.Errorf("invalid report: %w", err)
	}

	log.Printf("[bandwidth-report] processing report from node %s: %d bytes for %d users",
		report.NodeName, report.TotalBytes, len(report.UserUsage))

	// Process each user's usage
	for _, usage := range report.UserUsage {
		if err := s.updateUserUsage(report.NodeName, usage); err != nil {
			log.Printf("[bandwidth-report] failed to update usage for user %s: %v", usage.UUID, err)
			// Continue processing other users even if one fails
		}
	}

	// Update node's total bandwidth usage
	if err := s.updateNodeUsage(report.NodeName, report.TotalBytes); err != nil {
		log.Printf("[bandwidth-report] failed to update node usage: %v", err)
	}

	// Update node's last sync time
	now := time.Now()
	if err := s.db.Model(&models.Node{}).
		Where("name = ?", report.NodeName).
		Updates(map[string]interface{}{
			"last_sync_at":   &now,
			"health_status":  "online",
			"last_heartbeat": &now,
		}).Error; err != nil {
		log.Printf("[bandwidth-report] failed to update node status: %v", err)
	}

	return nil
}

// validateReport validates the bandwidth report structure and data
func (s *BandwidthReportService) validateReport(report BandwidthReport) error {
	if report.NodeName == "" {
		return errors.New("node name is required")
	}

	if report.TotalBytes < 0 {
		return errors.New("total bytes cannot be negative")
	}

	for _, usage := range report.UserUsage {
		if usage.UUID == "" {
			return errors.New("user UUID is required")
		}
		if usage.BytesUsed < 0 {
			return fmt.Errorf("bytes used cannot be negative for user %s", usage.UUID)
		}
	}

	return nil
}

// updateUserUsage updates the bandwidth usage for a specific user
func (s *BandwidthReportService) updateUserUsage(nodeName string, usage UserUsageReport) error {
	if usage.UUID == "" || usage.BytesUsed <= 0 {
		return nil // Skip invalid or zero usage
	}

	userService := NewUserService(s.db)
	_, _, err := userService.RecordUsageOnNode(usage.UUID, nodeName, usage.BytesUsed)
	if err != nil {
		return fmt.Errorf("database update failed: %w", err)
	}

	log.Printf("[bandwidth-report] updated user %s: +%d bytes", usage.UUID, usage.BytesUsed)
	return nil
}

// updateNodeUsage updates the total bandwidth usage for a node
func (s *BandwidthReportService) updateNodeUsage(nodeName string, bytes int64) error {
	if nodeName == "" || bytes <= 0 {
		return nil
	}

	result := s.db.Model(&models.Node{}).
		Where("name = ?", nodeName).
		UpdateColumn("bandwidth_used_bytes", gorm.Expr("bandwidth_used_bytes + ?", bytes))

	if result.Error != nil {
		return fmt.Errorf("database update failed: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		log.Printf("[bandwidth-report] node %s not found, skipping usage update", nodeName)
		return nil // Node not found, but not an error
	}

	log.Printf("[bandwidth-report] updated node %s: +%d bytes", nodeName, bytes)
	return nil
}

// GetUserUsage retrieves the current bandwidth usage for a user
func (s *BandwidthReportService) GetUserUsage(uuid string) (int64, error) {
	var user models.User
	if err := s.db.Where("uuid = ?", uuid).First(&user).Error; err != nil {
		return 0, err
	}
	return user.BandwidthUsedBytes, nil
}

// GetNodeUsage retrieves the current bandwidth usage for a node
func (s *BandwidthReportService) GetNodeUsage(nodeName string) (int64, error) {
	var node models.Node
	if err := s.db.Where("name = ?", nodeName).First(&node).Error; err != nil {
		return 0, err
	}
	return node.BandwidthUsedBytes, nil
}

// ResetUserUsage resets the bandwidth usage for a user (useful for billing cycles)
func (s *BandwidthReportService) ResetUserUsage(uuid string) error {
	return s.db.Model(&models.User{}).
		Where("uuid = ?", uuid).
		UpdateColumn("bandwidth_used_bytes", 0).Error
}

// ResetNodeUsage resets the bandwidth usage for a node
func (s *BandwidthReportService) ResetNodeUsage(nodeName string) error {
	return s.db.Model(&models.Node{}).
		Where("name = ?", nodeName).
		UpdateColumn("bandwidth_used_bytes", 0).Error
}

// GetUsageSummary returns a summary of bandwidth usage across all users
func (s *BandwidthReportService) GetUsageSummary() (map[string]interface{}, error) {
	var totalUsage int64
	var userCount int64

	// Get total usage
	if err := s.db.Model(&models.User{}).
		Select("COALESCE(SUM(bandwidth_used_bytes), 0)").
		Scan(&totalUsage).Error; err != nil {
		return nil, err
	}

	// Get user count
	if err := s.db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return nil, err
	}

	// Get top users by usage
	type UserUsage struct {
		UUID         string
		Email        string
		Usage        int64
		Limit        int64
		UsagePercent float64
	}

	var topUsers []UserUsage
	if err := s.db.Table("users").
		Select("uuid, email, bandwidth_used_bytes as usage, bandwidth_limit_gb as limit, (bandwidth_used_bytes * 100.0 / (bandwidth_limit_gb * 1024 * 1024 * 1024)) as usage_percent").
		Where("bandwidth_limit_gb > 0").
		Order("bandwidth_used_bytes DESC").
		Limit(10).
		Scan(&topUsers).Error; err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"totalUsageBytes": totalUsage,
		"totalUsers":      userCount,
		"topUsers":        topUsers,
	}, nil
}
