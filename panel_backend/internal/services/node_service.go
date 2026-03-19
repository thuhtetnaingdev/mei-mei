package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
}

type SyncPayload struct {
	NodeName string        `json:"nodeName"`
	Users    []models.User `json:"users"`
}

type SyncUser struct {
	UUID             string `json:"uuid"`
	Email            string `json:"email"`
	Enabled          bool   `json:"enabled"`
	BandwidthLimitGB int64  `json:"bandwidthLimitGb"`
}

type SyncPayloadWithLimits struct {
	NodeName string      `json:"nodeName"`
	Users    []SyncUser  `json:"users"`
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
			VLESSPort:            input.VLESSPort,
			TUICPort:             input.TUICPort,
			Hysteria2Port:        input.Hysteria2Port,
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

func (s *NodeService) Update(id string, input UpdateNodeInput) (*models.Node, error) {
	var node models.Node
	if err := s.db.First(&node, "id = ?", id).Error; err != nil {
		return nil, err
	}

	if input.Location != nil {
		node.Location = *input.Location
	}
	if input.PublicHost != nil && *input.PublicHost != "" {
		node.PublicHost = *input.PublicHost
	}
	if input.BandwidthLimitGB != nil {
		node.BandwidthLimitGB = *input.BandwidthLimitGB
	}
	node.ExpiresAt = input.ExpiresAt

	if err := s.db.Save(&node).Error; err != nil {
		return nil, err
	}

	return &node, nil
}

func (s *NodeService) Delete(id string) error {
	result := s.db.Delete(&models.Node{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
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

		err := s.syncNode(node, users)
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

func (s *NodeService) syncNode(node models.Node, users []models.User) error {
	syncUsers := make([]SyncUser, 0, len(users))
	for _, user := range users {
		syncUsers = append(syncUsers, SyncUser{
			UUID:             user.UUID,
			Email:            user.Email,
			Enabled:          user.Enabled,
			BandwidthLimitGB: user.BandwidthLimitGB,
		})
	}

	payload := SyncPayloadWithLimits{
		NodeName: node.Name,
		Users:    syncUsers,
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
	_ = s.db.Model(node).Updates(map[string]interface{}{
		"health_status":        "online",
		"last_heartbeat":       &now,
		"bandwidth_used_bytes": status.BandwidthUsedBytes,
	}).Error
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

	if node.HealthStatus == "offline" {
		return nil, fmt.Errorf("node is offline and cannot be reinstalled")
	}

	targetArch := input.TargetArch
	if targetArch == "" {
		targetArch = "amd64"
	}

	log.Printf("[reinstall] Building tarball for node %s (arch: %s)", node.Name, targetArch)
	tarballPath, tempDir, err := s.buildNodeTarball(targetArch)
	if err != nil {
		return nil, fmt.Errorf("building node tarball failed: %w", err)
	}
	defer os.RemoveAll(tempDir)

	log.Printf("[reinstall] Tarball created at %s", tarballPath)
	return s.sendReinstallRequest(node, tarballPath)
}

func (s *NodeService) buildNodeTarball(goarch string) (string, string, error) {
	repoRoot, err := filepath.Abs(filepath.Join(".", ".."))
	if err != nil {
		return "", "", err
	}

	distDir := filepath.Join(repoRoot, "dist")
	
	// Try multiple possible binary paths
	possiblePaths := []string{
		filepath.Join(distDir, "node_backend", "node_backend"), // From make release-node
		filepath.Join(distDir, "node_backend-linux-amd64", "node_backend"),
		filepath.Join(distDir, "node_backend-linux-arm64", "node_backend"),
		filepath.Join(distDir, "node_backend"),                  // Direct binary (last resort)
	}

	binaryPath := ""
	for _, path := range possiblePaths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			binaryPath = path
			log.Printf("[reinstall] Found binary candidate at %s (size: %d bytes)", path, info.Size())
			break
		}
	}

	if binaryPath == "" {
		// List what's actually in dist directory
		entries, _ := os.ReadDir(distDir)
		var dirContents []string
		for _, e := range entries {
			dirContents = append(dirContents, e.Name())
		}
		return "", "", fmt.Errorf("node_backend binary not found in %s, found: %v", distDir, dirContents)
	}

	// Verify it's actually a file with reasonable size
	info, err := os.Stat(binaryPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to stat binary: %w", err)
	}
	
	if info.IsDir() {
		return "", "", fmt.Errorf("binary path %s is a directory, not a file", binaryPath)
	}
	
	if info.Size() < 1000 {
		return "", "", fmt.Errorf("binary file %s seems too small (%d bytes), may be corrupted", binaryPath, info.Size())
	}
	
	if info.Size() > 100*1024*1024 {
		return "", "", fmt.Errorf("binary file %s seems too large (%d bytes), may be wrong file", binaryPath, info.Size())
	}

	log.Printf("[reinstall] Using binary at %s (size: %d bytes)", binaryPath, info.Size())

	tempDir, err := os.MkdirTemp("", "meimei-node-reinstall-*")
	if err != nil {
		return "", "", err
	}

	tarballPath := filepath.Join(tempDir, "node_backend.tar.gz")
	if err := archiveSingleFile(binaryPath, tarballPath, "node_backend", 0o755); err != nil {
		return "", "", err
	}

	return tarballPath, tempDir, nil
}

func (s *NodeService) sendReinstallRequest(node models.Node, tarballPath string) (*ReinstallNodeResult, error) {
	file, err := os.Open(tarballPath)
	if err != nil {
		return nil, fmt.Errorf("opening tarball failed: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("getting file info failed: %w", err)
	}

	log.Printf("[reinstall] Tarball size: %d bytes", fileInfo.Size())

	body := &bytes.Buffer{}
	writer := &multipartWriter{
		buffer: body,
	}

	if err := writer.WriteField("action", "reinstall"); err != nil {
		return nil, fmt.Errorf("writing action field failed: %w", err)
	}

	if err := writer.CreateFormFile("tarball", "node_backend.tar.gz", fileInfo.Size(), file); err != nil {
		return nil, fmt.Errorf("creating form file failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer failed: %w", err)
	}

	log.Printf("[reinstall] Sending request to %s/reinstall", node.BaseURL)
	log.Printf("[reinstall] Request body size: %d bytes", body.Len())

	reinstallURL := fmt.Sprintf("%s/reinstall", node.BaseURL)

	req, err := http.NewRequest(http.MethodPost, reinstallURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request failed: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+node.ProtocolToken)
	req.Header.Set("X-Control-Plane-Token", s.sharedToken)

	log.Printf("[reinstall] Content-Type: %s", writer.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		// If we get EOF, it might mean the service restarted successfully
		// Check if the error contains EOF
		if err == io.EOF || strings.Contains(err.Error(), "EOF") {
			log.Printf("[reinstall] Got EOF (service likely restarted), considering as success")
			return &ReinstallNodeResult{
				Success: true,
				Message: "Reinstall completed (service restarted)",
			}, nil
		}
		log.Printf("[reinstall] HTTP request failed: %v", err)
		return nil, fmt.Errorf("sending reinstall request failed: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[reinstall] Response status: %d", resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[reinstall] Reading response body failed: %v", err)
		return nil, fmt.Errorf("reading response body failed: %w", err)
	}

	log.Printf("[reinstall] Response body: %s", string(respBody))

	if resp.StatusCode >= 300 {
		return &ReinstallNodeResult{
			Success: false,
			Message: fmt.Sprintf("node returned status %d: %s", resp.StatusCode, string(respBody)),
		}, nil
	}

	var nodeResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &nodeResp); err != nil {
		return &ReinstallNodeResult{
			Success: true,
			Message: string(respBody),
		}, nil
	}

	return &ReinstallNodeResult{
		Success: nodeResp.Success,
		Message: nodeResp.Message,
	}, nil
}

type multipartWriter struct {
	buffer      *bytes.Buffer
	contentType string
}

func (w *multipartWriter) WriteField(fieldname, value string) error {
	if w.contentType == "" {
		boundary := uuid.NewString()
		w.contentType = "multipart/form-data; boundary=" + boundary
	}
	boundary := w.getBoundary()

	_, err := fmt.Fprintf(w.buffer, "--%s\r\nContent-Disposition: form-data; name=\"%s\"\r\n\r\n%s\r\n", boundary, fieldname, value)
	return err
}

func (w *multipartWriter) CreateFormFile(fieldname, filename string, filesize int64, file io.Reader) error {
	boundary := w.getBoundary()

	_, err := fmt.Fprintf(w.buffer, "--%s\r\nContent-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n", boundary, fieldname, filename, filesize)
	if err != nil {
		return err
	}

	_, err = io.Copy(w.buffer, file)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w.buffer, "\r\n")
	return err
}

func (w *multipartWriter) Close() error {
	boundary := w.getBoundary()
	_, err := fmt.Fprintf(w.buffer, "--%s--\r\n", boundary)
	return err
}

func (w *multipartWriter) FormDataContentType() string {
	return w.contentType
}

func (w *multipartWriter) getBoundary() string {
	parts := strings.SplitN(w.contentType, "=", 2)
	if len(parts) < 2 {
		return "----FormBoundary" + uuid.NewString()
	}
	return strings.TrimSpace(parts[1])
}
