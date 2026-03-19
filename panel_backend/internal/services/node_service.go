package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"panel_backend/internal/models"
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
	NodeName string     `json:"nodeName"`
	Users    []SyncUser `json:"users"`
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

	now := time.Now()
	_ = s.db.Model(&node).Updates(map[string]interface{}{
		"health_status":   "online",
		"last_heartbeat":  &now,
		"singbox_version": "installer-managed",
	}).Error

	return &ReinstallNodeResult{
		Success: true,
		Message: message,
	}, nil
}
