package services

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"panel_backend/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestParseSubscriptionUserInfo(t *testing.T) {
	info, err := parseSubscriptionUserInfo("upload=120827; download=256562; total=32212254720; expire=1781884020")
	if err != nil {
		t.Fatalf("parseSubscriptionUserInfo() error = %v", err)
	}

	if info.UploadBytes != 120827 || info.DownloadBytes != 256562 || info.TotalBytes != 32212254720 {
		t.Fatalf("unexpected info: %#v", info)
	}
	if info.ExpiresAt == nil || info.ExpiresAt.Unix() != 1781884020 {
		t.Fatalf("unexpected expiry: %#v", info.ExpiresAt)
	}
}

func TestImportSubscriptionCreatesUserFromHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=120827; download=256562; total=32212254720; expire=1781884020")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	conn := newMigrationTestDB(t)
	service := NewMigrationService(conn)

	result, err := service.ImportSubscription(t.Context(), MigrationImportInput{
		SubscriptionURL: server.URL + "/sub/Tiffiny?format=json",
	})
	if err != nil {
		t.Fatalf("ImportSubscription() error = %v", err)
	}

	if result.Username != "Tiffiny" {
		t.Fatalf("expected username Tiffiny, got %q", result.Username)
	}
	if result.User.Email != "Tiffiny" {
		t.Fatalf("expected user identity Tiffiny, got %q", result.User.Email)
	}
	if result.User.BandwidthUsedBytes != 377389 {
		t.Fatalf("unexpected used bytes: %d", result.User.BandwidthUsedBytes)
	}
	if result.User.BandwidthLimitGB != 30 {
		t.Fatalf("unexpected bandwidth limit: %d", result.User.BandwidthLimitGB)
	}
	if result.User.ExpiresAt == nil || result.User.ExpiresAt.Unix() != 1781884020 {
		t.Fatalf("unexpected user expiry: %#v", result.User.ExpiresAt)
	}
	if !result.User.Enabled {
		t.Fatal("expected imported user to be enabled")
	}
	if len(result.User.BandwidthAllocations) != 1 {
		t.Fatalf("expected one allocation, got %d", len(result.User.BandwidthAllocations))
	}

	allocation := result.User.BandwidthAllocations[0]
	if allocation.TotalBandwidthBytes != 32212254720 {
		t.Fatalf("unexpected total allocation bytes: %d", allocation.TotalBandwidthBytes)
	}
	if allocation.RemainingBandwidthBytes != 32212254720-377389 {
		t.Fatalf("unexpected remaining allocation bytes: %d", allocation.RemainingBandwidthBytes)
	}
	if allocation.SettlementStatus != "migrated" {
		t.Fatalf("unexpected settlement status: %q", allocation.SettlementStatus)
	}

	mapping, err := service.UserIDMap()
	if err != nil {
		t.Fatalf("UserIDMap() error = %v", err)
	}
	if mapping["Tiffiny"] != result.User.UUID {
		t.Fatalf("expected map to include Tiffiny => %s, got %#v", result.User.UUID, mapping)
	}
}

func TestImportSubscriptionDisablesExpiredOrDepletedUser(t *testing.T) {
	expired := time.Now().Add(-time.Hour).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=10; download=90; total=100; expire="+strconv.FormatInt(expired, 10))
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	conn := newMigrationTestDB(t)
	service := NewMigrationService(conn)

	result, err := service.ImportSubscription(t.Context(), MigrationImportInput{
		SubscriptionURL: server.URL + "/sub/Expired?format=json",
	})
	if err != nil {
		t.Fatalf("ImportSubscription() error = %v", err)
	}
	if result.User.Enabled {
		t.Fatal("expected expired depleted user to be disabled")
	}
}

func newMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	conn, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := conn.AutoMigrate(
		&models.User{},
		&models.UserBandwidthAllocation{},
		&models.UserBandwidthNodeUsage{},
		&models.UserRecord{},
	); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return conn
}
