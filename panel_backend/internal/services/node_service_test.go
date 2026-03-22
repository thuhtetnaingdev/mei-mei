package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
	"panel_backend/internal/db"
	"panel_backend/internal/models"
)

func TestSyncAllUsersVerifiesAppliedPayload(t *testing.T) {
	service, conn := newTestNodeService(t, func(captured *SyncPayloadWithLimits) nodeStatusResponse {
		return nodeStatusResponse{
			Status:                 "ok",
			LastAppliedConfigHash:  hashSyncPayload(*captured),
			AppliedUserCount:       countEnabledSyncUsers(captured.Users),
			RealityPublicKey:       "pubkey-1",
			RealityShortID:         "short-1",
			RealityServerName:      "example.com",
			SyncVerificationStatus: "applied",
		}
	})

	users := []models.User{{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true}}
	results, err := service.SyncAllUsers(users)
	if err != nil {
		t.Fatalf("SyncAllUsers() error = %v", err)
	}
	if got := results[0]["status"]; got != "success" {
		t.Fatalf("unexpected sync status: got %v want success", got)
	}

	var node models.Node
	if err := conn.First(&node).Error; err != nil {
		t.Fatalf("load node: %v", err)
	}
	if node.SyncVerificationStatus != "verified" {
		t.Fatalf("unexpected sync verification status: got %q want verified", node.SyncVerificationStatus)
	}
	if node.AppliedUserCount != 1 {
		t.Fatalf("unexpected applied user count: got %d want 1", node.AppliedUserCount)
	}
	if node.RealityPublicKey != "pubkey-1" {
		t.Fatalf("unexpected reality public key: got %q want pubkey-1", node.RealityPublicKey)
	}
	if node.RealityShortID != "short-1" {
		t.Fatalf("unexpected reality short id: got %q want short-1", node.RealityShortID)
	}
	if node.RealityServerName != "example.com" {
		t.Fatalf("unexpected reality server name: got %q want example.com", node.RealityServerName)
	}
}

func TestSyncAllUsersFailsVerificationOnHashMismatch(t *testing.T) {
	service, conn := newTestNodeService(t, func(captured *SyncPayloadWithLimits) nodeStatusResponse {
		return nodeStatusResponse{
			Status:                 "ok",
			LastAppliedConfigHash:  "wrong-hash",
			AppliedUserCount:       countEnabledSyncUsers(captured.Users),
			SyncVerificationStatus: "applied",
		}
	})

	users := []models.User{{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true}}
	results, err := service.SyncAllUsers(users)
	if err != nil {
		t.Fatalf("SyncAllUsers() error = %v", err)
	}
	if got := results[0]["status"]; got != "failed" {
		t.Fatalf("unexpected sync status: got %v want failed", got)
	}

	var node models.Node
	if err := conn.First(&node).Error; err != nil {
		t.Fatalf("load node: %v", err)
	}
	if node.SyncVerificationStatus != "mismatch" {
		t.Fatalf("unexpected sync verification status: got %q want mismatch", node.SyncVerificationStatus)
	}
	if node.HealthStatus != "online" {
		t.Fatalf("expected node health to stay online, got %q", node.HealthStatus)
	}
}

func TestSyncAllUsersUsesFilteredTestingUserCount(t *testing.T) {
	service, conn := newTestNodeService(t, func(captured *SyncPayloadWithLimits) nodeStatusResponse {
		return nodeStatusResponse{
			Status:                 "ok",
			LastAppliedConfigHash:  hashSyncPayload(*captured),
			AppliedUserCount:       countEnabledSyncUsers(captured.Users),
			SyncVerificationStatus: "applied",
		}
	})

	var node models.Node
	if err := conn.First(&node).Error; err != nil {
		t.Fatalf("load node: %v", err)
	}
	node.IsTestable = false
	if err := conn.Save(&node).Error; err != nil {
		t.Fatalf("save node: %v", err)
	}

	users := []models.User{{ID: 1, UUID: "user-1", Email: "one@example.com", Enabled: true, IsTesting: true}}
	results, err := service.SyncAllUsers(users)
	if err != nil {
		t.Fatalf("SyncAllUsers() error = %v", err)
	}
	if got := results[0]["status"]; got != "success" {
		t.Fatalf("unexpected sync status: got %v want success", got)
	}
	if got := results[0]["expectedUserCount"]; got != 0 {
		t.Fatalf("unexpected expectedUserCount: got %v want 0", got)
	}
	if got := results[0]["appliedUserCount"]; got != 0 {
		t.Fatalf("unexpected appliedUserCount: got %v want 0", got)
	}
}

func newTestNodeService(t *testing.T, statusFor func(*SyncPayloadWithLimits) nodeStatusResponse) (*NodeService, *gorm.DB) {
	t.Helper()

	var captured SyncPayloadWithLimits
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apply-config":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"applied"}`))
		case "/status":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(statusFor(&captured))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	conn, err := db.Connect(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("db.Connect() error = %v", err)
	}

	node := models.Node{
		Name:          "node-1",
		BaseURL:       server.URL,
		PublicHost:    "node.example.com",
		ProtocolToken: "token",
		Enabled:       true,
		HealthStatus:  "unknown",
	}
	if err := conn.Create(&node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	userService := NewUserService(conn)
	return NewNodeService(conn, "shared-token", 5*time.Second, userService), conn
}
