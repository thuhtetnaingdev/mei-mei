package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"panel_backend/internal/models"
)

type MigrationService struct {
	db         *gorm.DB
	httpClient *http.Client
}

type MigrationImportInput struct {
	SubscriptionURL string `json:"subscriptionUrl" binding:"required,url"`
}

type MigrationImportResult struct {
	User             models.User `json:"user"`
	Username         string      `json:"username"`
	SubscriptionURL  string      `json:"subscriptionUrl"`
	UploadBytes      int64       `json:"uploadBytes"`
	DownloadBytes    int64       `json:"downloadBytes"`
	UsedBytes        int64       `json:"usedBytes"`
	TotalBytes       int64       `json:"totalBytes"`
	RemainingBytes   int64       `json:"remainingBytes"`
	ExpiresAt        *time.Time  `json:"expiresAt"`
	BandwidthLimitGB int64       `json:"bandwidthLimitGb"`
	Enabled          bool        `json:"enabled"`
}

type subscriptionUserInfo struct {
	UploadBytes   int64
	DownloadBytes int64
	TotalBytes    int64
	ExpiresAt     *time.Time
}

func NewMigrationService(db *gorm.DB) *MigrationService {
	return &MigrationService{
		db: db,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *MigrationService) ImportSubscription(ctx context.Context, input MigrationImportInput) (*MigrationImportResult, error) {
	subscriptionURL := strings.TrimSpace(input.SubscriptionURL)
	if subscriptionURL == "" {
		return nil, errors.New("subscription URL is required")
	}

	username, err := usernameFromSubscriptionURL(subscriptionURL)
	if err != nil {
		return nil, err
	}

	info, err := s.fetchSubscriptionUserInfo(ctx, subscriptionURL)
	if err != nil {
		return nil, err
	}

	if info.TotalBytes <= 0 {
		return nil, errors.New("subscription total bandwidth must be greater than zero")
	}

	usedBytes := info.UploadBytes + info.DownloadBytes
	if usedBytes < 0 {
		usedBytes = 0
	}
	remainingBytes := info.TotalBytes - usedBytes
	if remainingBytes < 0 {
		remainingBytes = 0
	}

	bandwidthLimitGB := int64(math.Ceil(float64(info.TotalBytes) / float64(bytesPerGB)))
	if bandwidthLimitGB < 1 {
		bandwidthLimitGB = 1
	}

	now := time.Now()
	enabled := remainingBytes > 0 && (info.ExpiresAt == nil || info.ExpiresAt.After(now))
	totalTokens := roundTokenAmount(float64(info.TotalBytes) / float64(bytesPerGB))
	remainingTokens := roundTokenAmount(float64(remainingBytes) / float64(bytesPerGB))
	if remainingTokens > totalTokens {
		remainingTokens = totalTokens
	}

	var importedUser models.User
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", username).First(&existing).Error
		if err == nil {
			return fmt.Errorf("user %q already exists", username)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		importedUser = models.User{
			UUID:                 uuid.NewString(),
			Email:                username,
			Enabled:              enabled,
			IsTesting:            false,
			ExpiresAt:            info.ExpiresAt,
			BandwidthLimitGB:     bandwidthLimitGB,
			BandwidthUsedBytes:   usedBytes,
			LastWeekMeteredBytes: 0,
			TokenBalance:         remainingTokens,
			Notes:                "Migrated from subscription URL",
			UserType:             "unknown",
		}
		if err := tx.Select("*").Create(&importedUser).Error; err != nil {
			return err
		}
		if !enabled {
			if err := tx.Model(&models.User{}).Where("id = ?", importedUser.ID).Update("enabled", false).Error; err != nil {
				return err
			}
			importedUser.Enabled = false
		}

		allocation := models.UserBandwidthAllocation{
			UserID:                  importedUser.ID,
			TotalBandwidthBytes:     info.TotalBytes,
			RemainingBandwidthBytes: remainingBytes,
			TokenAmount:             totalTokens,
			RemainingTokens:         remainingTokens,
			AdminPercent:            0,
			UsagePoolPercent:        100,
			ReservePoolPercent:      0,
			AdminAmount:             0,
			UsagePoolTotal:          totalTokens,
			UsagePoolDistributed:    roundTokenAmount(totalTokens - remainingTokens),
			ReservePoolTotal:        0,
			ReservePoolDistributed:  0,
			SettlementStatus:        "migrated",
			ExpiresAt:               info.ExpiresAt,
		}
		if err := tx.Create(&allocation).Error; err != nil {
			return err
		}

		if err := s.createMigrationRecordTx(tx, importedUser.ID, subscriptionURL, usedBytes, info.TotalBytes, info.ExpiresAt); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	userService := NewUserService(s.db)
	user, err := userService.GetByID(uintToString(importedUser.ID))
	if err != nil {
		return nil, err
	}

	return &MigrationImportResult{
		User:             *user,
		Username:         username,
		SubscriptionURL:  subscriptionURL,
		UploadBytes:      info.UploadBytes,
		DownloadBytes:    info.DownloadBytes,
		UsedBytes:        usedBytes,
		TotalBytes:       info.TotalBytes,
		RemainingBytes:   remainingBytes,
		ExpiresAt:        info.ExpiresAt,
		BandwidthLimitGB: bandwidthLimitGB,
		Enabled:          enabled,
	}, nil
}

func (s *MigrationService) UserIDMap() (map[string]string, error) {
	var users []models.User
	if err := s.db.Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}

	out := make(map[string]string, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Email) == "" {
			continue
		}
		out[user.Email] = user.UUID
	}
	return out, nil
}

func (s *MigrationService) fetchSubscriptionUserInfo(ctx context.Context, subscriptionURL string) (subscriptionUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscriptionURL, nil)
	if err != nil {
		return subscriptionUserInfo{}, fmt.Errorf("build subscription request: %w", err)
	}
	req.Header.Set("Accept", "application/json,text/plain,*/*")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return subscriptionUserInfo{}, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return subscriptionUserInfo{}, fmt.Errorf("subscription returned status %d", resp.StatusCode)
	}

	header := resp.Header.Get("Subscription-Userinfo")
	if strings.TrimSpace(header) == "" {
		return subscriptionUserInfo{}, errors.New("subscription response is missing Subscription-Userinfo header")
	}

	info, err := parseSubscriptionUserInfo(header)
	if err != nil {
		return subscriptionUserInfo{}, err
	}
	return info, nil
}

func parseSubscriptionUserInfo(header string) (subscriptionUserInfo, error) {
	values := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key != "" {
			values[key] = value
		}
	}

	parseBytes := func(key string) (int64, error) {
		raw := values[key]
		if raw == "" {
			return 0, fmt.Errorf("Subscription-Userinfo missing %s", key)
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			return 0, fmt.Errorf("Subscription-Userinfo has invalid %s", key)
		}
		return value, nil
	}

	upload, err := parseBytes("upload")
	if err != nil {
		return subscriptionUserInfo{}, err
	}
	download, err := parseBytes("download")
	if err != nil {
		return subscriptionUserInfo{}, err
	}
	total, err := parseBytes("total")
	if err != nil {
		return subscriptionUserInfo{}, err
	}

	var expiresAt *time.Time
	if rawExpire := values["expire"]; rawExpire != "" {
		expireUnix, err := strconv.ParseInt(rawExpire, 10, 64)
		if err != nil || expireUnix < 0 {
			return subscriptionUserInfo{}, errors.New("Subscription-Userinfo has invalid expire")
		}
		if expireUnix > 0 {
			expiry := time.Unix(expireUnix, 0).UTC()
			expiresAt = &expiry
		}
	}

	return subscriptionUserInfo{
		UploadBytes:   upload,
		DownloadBytes: download,
		TotalBytes:    total,
		ExpiresAt:     expiresAt,
	}, nil
}

func usernameFromSubscriptionURL(subscriptionURL string) (string, error) {
	parsed, err := url.Parse(subscriptionURL)
	if err != nil {
		return "", errors.New("invalid subscription URL")
	}

	cleanPath := path.Clean(parsed.EscapedPath())
	if cleanPath == "." || cleanPath == "/" {
		return "", errors.New("subscription URL must include a username path segment")
	}

	segment := path.Base(cleanPath)
	username, err := url.PathUnescape(segment)
	if err != nil {
		return "", errors.New("subscription URL username is invalid")
	}
	username = strings.TrimSpace(username)
	if username == "" || username == "." || username == "/" {
		return "", errors.New("subscription URL must include a username path segment")
	}
	return username, nil
}

func (s *MigrationService) createMigrationRecordTx(tx *gorm.DB, userID uint, subscriptionURL string, usedBytes int64, totalBytes int64, expiresAt *time.Time) error {
	details := fmt.Sprintf("Imported subscription %s with %s used of %s total", subscriptionURL, formatBandwidthBytes(usedBytes), formatBandwidthBytes(totalBytes))
	if expiresAt != nil {
		details += fmt.Sprintf(", expires at %s", expiresAt.UTC().Format(time.RFC3339))
	}

	return tx.Create(&models.UserRecord{
		UserID:  userID,
		Action:  "migrated",
		Title:   "User migrated",
		Details: details,
	}).Error
}
