package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"panel_backend/internal/models"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NodeService struct {
	db            *gorm.DB
	httpClient    *http.Client
	sharedToken   string
	userService   *UserService
	bootstrapMu   sync.RWMutex
	bootstrapJobs map[string]*BootstrapJob
}

type NodeDeleteRemoteError struct {
	err error
}

func (e *NodeDeleteRemoteError) Error() string {
	if e == nil || e.err == nil {
		return "node uninstall failed"
	}
	return e.err.Error()
}

func (e *NodeDeleteRemoteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

type RegisterNodeInput struct {
	RegistrationToken string     `json:"registrationToken" binding:"required"`
	Name              string     `json:"name" binding:"required"`
	BaseURL           string     `json:"baseUrl" binding:"required,url"`
	Location          string     `json:"location"`
	PublicHost        string     `json:"publicHost" binding:"required"`
	VLESSPort         int        `json:"vlessPort"`
	TUICPort          int        `json:"tuicPort"`
	Hysteria2Port     int        `json:"hysteria2Port"`
	RealityPublicKey  string     `json:"realityPublicKey"`
	RealityShortID    string     `json:"realityShortId"`
	RealityServerName string     `json:"realityServerName"`
	ExpiresAt         *time.Time `json:"expiresAt"`
	BandwidthLimitGB  int64      `json:"bandwidthLimitGb"`
	ProtocolToken     string     `json:"token" binding:"required"`
	SingboxVersion    string     `json:"singboxVersion"`
}

type UpdateNodeInput struct {
	Location         *string    `json:"location"`
	PublicHost       *string    `json:"publicHost"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	BandwidthLimitGB *int64     `json:"bandwidthLimitGb"`
	Enabled          *bool      `json:"enabled"`
}

type SyncPayload struct {
	NodeName string        `json:"nodeName"`
	Users    []models.User `json:"users"`
}

type SyncUser struct {
	ID               uint   `json:"id"`
	UUID             string `json:"uuid"`
	Email            string `json:"email"`
	Enabled          bool   `json:"enabled"`
	BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
}

type SyncPayloadWithLimits struct {
	NodeName             string     `json:"nodeName"`
	RealitySNIs          []string   `json:"realitySnis"`
	Hysteria2Masquerades []string   `json:"hysteria2Masquerades"`
	Users                []SyncUser `json:"users"`
}

type BootstrapJob struct {
	ID         string             `json:"id"`
	Status     string             `json:"status"`
	Steps      []string           `json:"steps"`
	Logs       []string           `json:"logs"`
	Error      string             `json:"error,omitempty"`
	Node       *models.Node       `json:"node,omitempty"`
	StartedAt  time.Time          `json:"startedAt"`
	FinishedAt *time.Time         `json:"finishedAt,omitempty"`
	Input      BootstrapNodeInput `json:"input"`
}

type NodePortDiagnostic struct {
	Label        string `json:"label"`
	Port         int    `json:"port"`
	Protocol     string `json:"protocol"`
	Checked      bool   `json:"checked"`
	Reachable    bool   `json:"reachable"`
	LatencyMs    int64  `json:"latencyMs"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type NodeDiagnosticResult struct {
	NodeID          uint                 `json:"nodeId"`
	NodeName        string               `json:"nodeName"`
	PublicHost      string               `json:"publicHost"`
	BaseURL         string               `json:"baseUrl"`
	APIReachable    bool                 `json:"apiReachable"`
	APILatencyMs    int64                `json:"apiLatencyMs"`
	APIErrorMessage string               `json:"apiErrorMessage,omitempty"`
	DownloadMbps    float64              `json:"downloadMbps"`
	UploadMbps      float64              `json:"uploadMbps"`
	DownloadBytes   int64                `json:"downloadBytes"`
	UploadBytes     int64                `json:"uploadBytes"`
	DownloadError   string               `json:"downloadError,omitempty"`
	UploadError     string               `json:"uploadError,omitempty"`
	Ports           []NodePortDiagnostic `json:"ports"`
	QualityStatus   string               `json:"qualityStatus"`
	TestedAt        time.Time            `json:"testedAt"`
}

func NewNodeService(db *gorm.DB, sharedToken string, timeout time.Duration, userService *UserService) *NodeService {
	return &NodeService{
		db:            db,
		sharedToken:   sharedToken,
		userService:   userService,
		bootstrapJobs: map[string]*BootstrapJob{},
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetDB returns the database instance for direct access
func (s *NodeService) GetDB() *gorm.DB {
	return s.db
}

func (s *NodeService) StartBootstrap(input BootstrapNodeInput) *BootstrapJob {
	job := &BootstrapJob{
		ID:        uuid.NewString(),
		Status:    "running",
		Steps:     []string{"Bootstrap job created"},
		Logs:      []string{},
		StartedAt: time.Now(),
		Input: BootstrapNodeInput{
			Name:                 input.Name,
			IP:                   input.IP,
			Username:             input.Username,
			Location:             input.Location,
			PublicHost:           input.PublicHost,
			SSHPort:              input.SSHPort,
			NodePort:             input.NodePort,
			SingboxReloadCommand: input.SingboxReloadCommand,
		},
	}

	s.bootstrapMu.Lock()
	s.bootstrapJobs[job.ID] = job
	s.bootstrapMu.Unlock()

	go func() {
		result, err := s.bootstrapAndRegister(input, job.ID)
		s.bootstrapMu.Lock()
		defer s.bootstrapMu.Unlock()

		finishedAt := time.Now()
		job.FinishedAt = &finishedAt
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
			job.Logs = append(job.Logs, "ERROR: "+err.Error())
			return
		}

		job.Status = "completed"
		job.Node = result.Node
		job.Steps = append([]string{}, result.Steps...)
	}()

	return s.cloneBootstrapJob(job)
}

func (s *NodeService) GetBootstrapJob(id string) (*BootstrapJob, error) {
	s.bootstrapMu.RLock()
	defer s.bootstrapMu.RUnlock()

	job, ok := s.bootstrapJobs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}

	return s.cloneBootstrapJob(job), nil
}

func (s *NodeService) appendBootstrapStep(jobID, step string) {
	s.bootstrapMu.Lock()
	defer s.bootstrapMu.Unlock()

	if job, ok := s.bootstrapJobs[jobID]; ok {
		job.Steps = append(job.Steps, step)
	}
}

func (s *NodeService) appendBootstrapLog(jobID, entry string) {
	s.bootstrapMu.Lock()
	defer s.bootstrapMu.Unlock()

	if job, ok := s.bootstrapJobs[jobID]; ok {
		job.Logs = append(job.Logs, entry)
	}
}

func (s *NodeService) cloneBootstrapJob(job *BootstrapJob) *BootstrapJob {
	if job == nil {
		return nil
	}

	cloned := *job
	cloned.Steps = append([]string{}, job.Steps...)
	cloned.Logs = append([]string{}, job.Logs...)
	return &cloned
}

func (s *NodeService) Register(input RegisterNodeInput) (*models.Node, error) {
	node := models.Node{
		Name:              input.Name,
		BaseURL:           input.BaseURL,
		Location:          input.Location,
		PublicHost:        input.PublicHost,
		VLESSPort:         input.VLESSPort,
		TUICPort:          input.TUICPort,
		Hysteria2Port:     input.Hysteria2Port,
		ExpiresAt:         input.ExpiresAt,
		BandwidthLimitGB:  input.BandwidthLimitGB,
		RealityPublicKey:  input.RealityPublicKey,
		RealityShortID:    input.RealityShortID,
		RealityServerName: input.RealityServerName,
		ProtocolToken:     input.ProtocolToken,
		Enabled:           true,
		HealthStatus:      "registered",
		SingboxVersion:    input.SingboxVersion,
	}

	err := s.db.Where(models.Node{Name: input.Name}).
		Assign(node).
		FirstOrCreate(&node).Error
	if err != nil {
		return nil, err
	}

	return &node, nil
}

func (s *NodeService) List() ([]models.Node, error) {
	var nodes []models.Node
	err := s.db.Order("created_at desc").Find(&nodes).Error
	return nodes, err
}

func (s *NodeService) ListWithRuntimeStatus() ([]models.Node, error) {
	nodes, err := s.List()
	if err != nil {
		return nil, err
	}

	for index := range nodes {
		s.refreshNodeRuntimeStatus(&nodes[index])
	}

	return nodes, nil
}

func (s *NodeService) RunDiagnostics() ([]NodeDiagnosticResult, error) {
	nodes, err := s.List()
	if err != nil {
		return nil, err
	}

	results := make([]NodeDiagnosticResult, 0, len(nodes))
	for _, node := range nodes {
		results = append(results, s.runNodeDiagnostic(node))
	}

	return results, nil
}

func (s *NodeService) Update(id string, input UpdateNodeInput) (*models.Node, error) {
	var node models.Node
	if err := s.db.First(&node, "id = ?", id).Error; err != nil {
		return nil, err
	}
	previousEnabled := node.Enabled

	if input.Location != nil {
		node.Location = *input.Location
	}
	if input.PublicHost != nil && *input.PublicHost != "" {
		node.PublicHost = *input.PublicHost
	}
	if input.BandwidthLimitGB != nil {
		node.BandwidthLimitGB = *input.BandwidthLimitGB
	}
	if input.Enabled != nil {
		node.Enabled = *input.Enabled
	}
	node.ExpiresAt = input.ExpiresAt

	if err := s.db.Save(&node).Error; err != nil {
		return nil, err
	}

	if input.Enabled != nil && previousEnabled != node.Enabled {
		if err := s.syncNodeEnabledState(&node); err != nil {
			node.Enabled = previousEnabled
			_ = s.db.Model(&node).Update("enabled", previousEnabled).Error
			return nil, err
		}
	}

	return &node, nil
}

func (s *NodeService) Delete(id string) error {
	return s.deleteNodeRecord(id)
}

func (s *NodeService) Uninstall(id string) error {
	var node models.Node
	if err := s.db.First(&node, "id = ?", id).Error; err != nil {
		return err
	}

	if err := s.requestNodeUninstall(node); err != nil {
		return &NodeDeleteRemoteError{err: err}
	}

	return s.deleteNodeRecord(id)
}

func (s *NodeService) deleteNodeRecord(id string) error {
	result := s.db.Delete(&models.Node{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func (s *NodeService) requestNodeUninstall(node models.Node) error {
	var errs []string

	if node.SSHPrivateKey != "" && node.SSHUsername != "" {
		if err := s.requestNodeUninstallViaSSH(node); err == nil {
			return nil
		} else {
			errs = append(errs, err.Error())
		}
	}

	if err := s.requestNodeUninstallViaAPI(node); err == nil {
		return nil
	} else {
		errs = append(errs, err.Error())
	}

	if len(errs) == 0 {
		return errors.New("node uninstall failed")
	}

	return errors.New(strings.Join(errs, "; "))
}

func (s *NodeService) requestNodeUninstallViaAPI(node models.Node) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/uninstall", node.BaseURL), bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("build node uninstall request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+node.ProtocolToken)
	req.Header.Set("X-Control-Plane-Token", s.sharedToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("node uninstall request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 {
		return nil
	}

	message := fmt.Sprintf("node returned status %d while scheduling uninstall", resp.StatusCode)
	body, readErr := io.ReadAll(resp.Body)
	if readErr == nil && len(body) > 0 {
		var payload struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
			message = payload.Error
		} else {
			message = string(body)
		}
	}

	return errors.New(message)
}

func (s *NodeService) requestNodeUninstallViaSSH(node models.Node) error {
	if node.SSHPrivateKey == "" || node.SSHUsername == "" {
		return errors.New("node uninstall endpoint is unavailable and no SSH credentials are stored for fallback cleanup")
	}

	sshHost := node.SSHHost
	if sshHost == "" {
		sshHost = extractSSHHost(node.BaseURL)
	}
	if sshHost == "" {
		return errors.New("node uninstall endpoint is unavailable and SSH host information is missing")
	}

	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	client, err := s.openSSHClientWithPrivateKey(sshHost, sshPort, node.SSHUsername, node.SSHPrivateKey)
	if err != nil {
		return fmt.Errorf("node uninstall endpoint is unavailable and SSH fallback connect failed: %w", err)
	}
	defer client.Close()

	if _, err := runSimpleRemoteCommand(client, buildNodeUninstallCommand(node)); err != nil {
		return fmt.Errorf("node uninstall endpoint is unavailable and SSH fallback cleanup failed: %w", err)
	}

	return nil
}

func (s *NodeService) SyncAllUsers(users []models.User) ([]map[string]interface{}, error) {
	nodes, err := s.List()
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		result := map[string]interface{}{
			"node":   node.Name,
			"status": "success",
		}

		usersToSync := users
		if !node.Enabled {
			usersToSync = []models.User{}
			result["mode"] = "disabled"
		}

		err := s.syncNode(node, usersToSync)
		if err != nil {
			result["status"] = "failed"
			result["error"] = err.Error()
		} else {
			now := time.Now()
			_ = s.db.Model(&node).Updates(map[string]interface{}{
				"last_sync_at":   &now,
				"health_status":  "online",
				"last_heartbeat": &now,
			}).Error
		}

		results = append(results, result)
	}

	return results, nil
}

func (s *NodeService) syncNodeEnabledState(node *models.Node) error {
	if node == nil {
		return errors.New("node not found")
	}

	usersToSync := []models.User{}
	if node.Enabled {
		activeUsers, err := s.userService.ActiveUsers()
		if err != nil {
			return err
		}
		usersToSync = activeUsers
	}

	if err := s.syncNode(*node, usersToSync); err != nil {
		return err
	}

	now := time.Now()
	updates := map[string]interface{}{
		"last_sync_at": &now,
	}
	if node.Enabled {
		updates["health_status"] = "online"
		updates["last_heartbeat"] = &now
	}

	if err := s.db.Model(node).Updates(updates).Error; err != nil {
		return err
	}

	node.LastSyncAt = &now
	if node.Enabled {
		node.HealthStatus = "online"
		node.LastHeartbeat = &now
	}
	return nil
}

func (s *NodeService) syncNode(node models.Node, users []models.User) error {
	syncUsers := make([]SyncUser, 0, len(users))
	for _, user := range users {
		syncUsers = append(syncUsers, SyncUser{
			ID:               user.ID,
			UUID:             user.UUID,
			Email:            user.Email,
			Enabled:          user.Enabled,
			BandwidthLimitGB: user.BandwidthLimitGB,
		})
	}

	protocolSettings, err := loadProtocolSettings(s.db)
	if err != nil {
		return err
	}

	payload := SyncPayloadWithLimits{
		NodeName:             node.Name,
		RealitySNIs:          protocolSettings.RealitySNIs,
		Hysteria2Masquerades: protocolSettings.Hysteria2Masquerades,
		Users:                syncUsers,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/apply-config", node.BaseURL), bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+node.ProtocolToken)
	req.Header.Set("X-Control-Plane-Token", s.sharedToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	return nil
}

type nodeStatusResponse struct {
	Status             string    `json:"status"`
	LastReload         time.Time `json:"lastReload"`
	BandwidthUsedBytes int64     `json:"bandwidthUsedBytes"`
	RealityPublicKey   string    `json:"realityPublicKey"`
	RealityShortID     string    `json:"realityShortId"`
	RealityServerName  string    `json:"realityServerName"`
}

func (s *NodeService) refreshNodeRuntimeStatus(node *models.Node) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/status", node.BaseURL), nil)
	if err != nil {
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.markNodeOffline(node)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		s.markNodeOffline(node)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var status nodeStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return
	}

	now := time.Now()
	node.HealthStatus = "online"
	node.LastHeartbeat = &now
	node.BandwidthUsedBytes = status.BandwidthUsedBytes
	updates := map[string]interface{}{
		"health_status":        "online",
		"last_heartbeat":       &now,
		"bandwidth_used_bytes": status.BandwidthUsedBytes,
	}

	if status.RealityPublicKey != "" && status.RealityShortID != "" {
		node.RealityPublicKey = status.RealityPublicKey
		node.RealityShortID = status.RealityShortID
		updates["reality_public_key"] = status.RealityPublicKey
		updates["reality_short_id"] = status.RealityShortID
	}
	if status.RealityServerName != "" {
		node.RealityServerName = status.RealityServerName
		updates["reality_server_name"] = status.RealityServerName
	}

	_ = s.db.Model(node).Updates(updates).Error
}

func (s *NodeService) markNodeOffline(node *models.Node) {
	node.HealthStatus = "offline"
	_ = s.db.Model(node).Update("health_status", "offline").Error
}

type ReinstallNodeInput struct {
	TargetArch string `json:"targetArch"`
}

type ReinstallNodeResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *NodeService) Reinstall(nodeID string, input ReinstallNodeInput) (*ReinstallNodeResult, error) {
	var node models.Node
	if err := s.db.First(&node, "id = ?", nodeID).Error; err != nil {
		return nil, gorm.ErrRecordNotFound
	}

	if node.SSHPrivateKey == "" || node.SSHUsername == "" {
		return nil, fmt.Errorf("node is missing stored SSH credentials; bootstrap it again before reinstalling")
	}

	sshHost := node.SSHHost
	if sshHost == "" {
		sshHost = extractSSHHost(node.BaseURL)
	}
	if sshHost == "" {
		return nil, fmt.Errorf("node is missing SSH host information")
	}

	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	client, err := s.openSSHClientWithPrivateKey(sshHost, sshPort, node.SSHUsername, node.SSHPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("key-based SSH connect failed: %w", err)
	}
	defer client.Close()

	installOutput, err := runSimpleRemoteCommand(client, buildNodeInstallCommand(node, s.sharedToken))
	if err != nil {
		return nil, fmt.Errorf("remote node installer failed: %w", err)
	}

	message := "node reinstall completed via installer"
	if installOutput != "" {
		message = installOutput
	}

	if err := s.waitForNodeStatus(node.BaseURL); err != nil {
		return nil, fmt.Errorf("node reinstall ran but node did not come back online: %w", err)
	}

	installedMetadata, err := readInstalledNodeMetadata(client)
	if err != nil {
		return nil, fmt.Errorf("reinstall succeeded but reading installed node metadata failed: %w", err)
	}

	now := time.Now()
	_ = s.db.Model(&node).Updates(map[string]interface{}{
		"health_status":       "online",
		"last_heartbeat":      &now,
		"singbox_version":     "installer-managed",
		"reality_public_key":  installedMetadata.RealityPublicKey,
		"reality_short_id":    installedMetadata.RealityShortID,
		"reality_server_name": installedMetadata.RealityServerName,
	}).Error

	return &ReinstallNodeResult{
		Success: true,
		Message: message,
	}, nil
}

func (s *NodeService) runNodeDiagnostic(node models.Node) NodeDiagnosticResult {
	result := NodeDiagnosticResult{
		NodeID:        node.ID,
		NodeName:      node.Name,
		PublicHost:    node.PublicHost,
		BaseURL:       node.BaseURL,
		QualityStatus: "healthy",
		TestedAt:      time.Now(),
	}

	apiLatency, apiErr := s.measureHTTPLatency(strings.TrimRight(node.BaseURL, "/") + "/status")
	if apiErr != nil {
		result.APIReachable = false
		result.APIErrorMessage = apiErr.Error()
		result.QualityStatus = "offline"
	} else {
		result.APIReachable = true
		result.APILatencyMs = apiLatency
	}

	if result.APIReachable {
		downloadMbps, downloadBytes, downloadErr := s.measureNodeDownload(node, 1024*1024)
		if downloadErr != nil {
			result.DownloadError = downloadErr.Error()
		} else {
			result.DownloadMbps = downloadMbps
			result.DownloadBytes = downloadBytes
		}

		uploadMbps, uploadBytes, uploadErr := s.measureNodeUpload(node, 512*1024)
		if uploadErr != nil {
			result.UploadError = uploadErr.Error()
		} else {
			result.UploadMbps = uploadMbps
			result.UploadBytes = uploadBytes
		}
	}

	if result.QualityStatus != "offline" {
		switch {
		case !result.APIReachable:
			result.QualityStatus = "offline"
		case result.DownloadError != "" || result.UploadError != "":
			result.QualityStatus = "degraded"
		case result.APILatencyMs >= 1200 || result.DownloadMbps < 5 || result.UploadMbps < 2:
			result.QualityStatus = "degraded"
		default:
			result.QualityStatus = "healthy"
		}
	}

	return result
}

func (s *NodeService) measureHTTPLatency(target string) (int64, error) {
	startedAt := time.Now()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	return time.Since(startedAt).Milliseconds(), nil
}

func (s *NodeService) measureNodeDownload(node models.Node, sizeBytes int64) (float64, int64, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/speed-test/download?bytes=%d", strings.TrimRight(node.BaseURL, "/"), sizeBytes), nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+node.ProtocolToken)
	req.Header.Set("X-Control-Plane-Token", s.sharedToken)

	startedAt := time.Now()
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return 0, 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	written, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, written, err
	}

	return calculateMbps(written, time.Since(startedAt)), written, nil
}

func (s *NodeService) measureNodeUpload(node models.Node, sizeBytes int64) (float64, int64, error) {
	payload := bytes.Repeat([]byte("MEIMEI_UPLOAD_TEST"), int(sizeBytes/18)+1)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/speed-test/upload", strings.TrimRight(node.BaseURL, "/")), bytes.NewReader(payload[:sizeBytes]))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+node.ProtocolToken)
	req.Header.Set("X-Control-Plane-Token", s.sharedToken)

	startedAt := time.Now()
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return 0, 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return calculateMbps(sizeBytes, time.Since(startedAt)), sizeBytes, nil
}

func calculateMbps(bytesTransferred int64, duration time.Duration) float64 {
	seconds := duration.Seconds()
	if seconds <= 0 {
		return 0
	}
	return (float64(bytesTransferred) * 8) / seconds / 1_000_000
}

func (s *NodeService) measureTCPPort(label, host string, port int, protocol string) NodePortDiagnostic {
	diagnostic := NodePortDiagnostic{
		Label:    label,
		Port:     port,
		Protocol: protocol,
		Checked:  true,
	}

	if host == "" || port <= 0 {
		diagnostic.ErrorMessage = "not configured"
		return diagnostic
	}

	targetHost := host
	if parsed, err := neturl.Parse(host); err == nil && parsed.Hostname() != "" {
		targetHost = parsed.Hostname()
	}

	startedAt := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(targetHost, fmt.Sprintf("%d", port)), 5*time.Second)
	if err != nil {
		diagnostic.ErrorMessage = err.Error()
		return diagnostic
	}
	_ = conn.Close()

	diagnostic.Reachable = true
	diagnostic.LatencyMs = time.Since(startedAt).Milliseconds()
	return diagnostic
}

func (s *NodeService) measureUnsupportedUDPPort(label, host string, port int) NodePortDiagnostic {
	diagnostic := NodePortDiagnostic{
		Label:    label,
		Port:     port,
		Protocol: "udp-pending",
		Checked:  false,
	}

	if host == "" || port <= 0 {
		diagnostic.ErrorMessage = "not configured"
		return diagnostic
	}

	diagnostic.ErrorMessage = "UDP quality probe not implemented yet"
	return diagnostic
}
