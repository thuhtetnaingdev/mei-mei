package services

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"panel_backend/internal/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ssh"
)

const (
	defaultInstallRepo         = "thuhtetnaingdev/mei-mei"
	defaultInstallVersion      = "latest"
	defaultNodeInstallerRawURL = "https://raw.githubusercontent.com/thuhtetnaingdev/mei-mei/main/install/node.sh"
)

type BootstrapNodeInput struct {
	Name                 string `json:"name" binding:"required"`
	IP                   string `json:"ip" binding:"required,ip"`
	Username             string `json:"username" binding:"required"`
	Password             string `json:"password" binding:"required"`
	Location             string `json:"location"`
	PublicHost           string `json:"publicHost"`
	SSHPort              int    `json:"sshPort"`
	NodePort             int    `json:"nodePort"`
	SingboxReloadCommand string `json:"singboxReloadCommand"`
}

type BootstrapNodeResult struct {
	Node  *models.Node `json:"node"`
	Steps []string     `json:"steps"`
}

type bootstrapArtifact struct {
	token             string
	realityPublicKey  string
	realityPrivateKey string
	realityShortID    string
	realityServerName string
	sshPrivateKeyPEM  string
	sshPublicKey      string
}

func (s *NodeService) BootstrapAndRegister(input BootstrapNodeInput) (*BootstrapNodeResult, error) {
	return s.bootstrapAndRegister(input, "")
}

func (s *NodeService) bootstrapAndRegister(input BootstrapNodeInput, jobID string) (*BootstrapNodeResult, error) {
	steps := []string{}
	addStep := func(step string) {
		steps = append(steps, step)
		if jobID != "" {
			s.appendBootstrapStep(jobID, step)
		}
	}
	addLog := func(format string, args ...interface{}) {
		if jobID == "" {
			return
		}
		s.appendBootstrapLog(jobID, fmt.Sprintf(format, args...))
	}

	normalizeBootstrapInput(&input)
	addLog("bootstrap started for node %s (%s)", input.Name, input.IP)

	addStep("connecting to VPS over SSH with password")
	passwordClient, err := s.openSSHClientWithPassword(input)
	if err != nil {
		return nil, fmt.Errorf("ssh connect failed: %w", err)
	}
	defer passwordClient.Close()
	addLog("password SSH connection established to %s:%d as %s", input.IP, input.SSHPort, input.Username)

	artifact, err := buildBootstrapArtifact()
	if err != nil {
		return nil, fmt.Errorf("building bootstrap metadata failed: %w", err)
	}

	addStep("installing persistent SSH public key on VPS")
	if err := installAuthorizedKey(passwordClient, input.Password, artifact.sshPublicKey); err != nil {
		return nil, fmt.Errorf("installing SSH public key failed: %w", err)
	}
	addLog("SSH public key installed successfully")

	addStep("validating key-based SSH access")
	keyClient, err := s.openSSHClientWithPrivateKey(input.IP, input.SSHPort, input.Username, artifact.sshPrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("validating SSH private key failed: %w", err)
	}
	defer keyClient.Close()
	addLog("key-based SSH validation succeeded")

	node, err := s.upsertBootstrapNodeRecord(input, artifact)
	if err != nil {
		return nil, fmt.Errorf("saving node bootstrap metadata failed: %w", err)
	}
	addStep("saved SSH keys and node metadata in control plane")

	addStep("running remote node installer")
	installOutput, err := runSimpleRemoteCommand(keyClient, buildNodeInstallCommand(*node, s.sharedToken))
	addLog("remote installer output:\n%s", installOutput)
	if err != nil {
		return nil, fmt.Errorf("remote node installer failed: %w", err)
	}

	addStep("waiting for node API handshake on " + node.BaseURL)
	if err := s.waitForNodeStatus(node.BaseURL); err != nil {
		addLog("node status check failed for %s: %v", node.BaseURL, err)
		return nil, fmt.Errorf("node did not come online: %w", err)
	}
	addLog("node API responded successfully at %s/status", node.BaseURL)

	installedMetadata, err := readInstalledNodeMetadata(keyClient)
	if err != nil {
		return nil, fmt.Errorf("reading installed node metadata failed: %w", err)
	}

	now := time.Now()
	node.HealthStatus = "online"
	node.LastHeartbeat = &now
	node.SingboxVersion = "installer-managed"
	node.RealityPublicKey = installedMetadata.RealityPublicKey
	node.RealityShortID = installedMetadata.RealityShortID
	node.RealityServerName = installedMetadata.RealityServerName
	if err := s.db.Model(node).Updates(map[string]interface{}{
		"health_status":       "online",
		"last_heartbeat":      &now,
		"singbox_version":     "installer-managed",
		"last_sync_at":        nil,
		"reality_public_key":  installedMetadata.RealityPublicKey,
		"reality_short_id":    installedMetadata.RealityShortID,
		"reality_server_name": installedMetadata.RealityServerName,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating node runtime status failed: %w", err)
	}
	addStep("registered node in control plane")
	addLog("node %s registered with public host %s", node.Name, node.PublicHost)
	addLog("captured live reality metadata from installed node")

	users, err := s.userService.ActiveUsers()
	if err == nil {
		if _, syncErr := s.SyncAllUsers(users); syncErr == nil {
			addStep(fmt.Sprintf("synced %d active users to the new node", len(users)))
			addLog("initial sync completed for %d active users", len(users))
		} else {
			addStep("node registered, but initial user sync failed: " + syncErr.Error())
			addLog("initial sync failed: %v", syncErr)
		}
	}
	addLog("bootstrap completed successfully")

	return &BootstrapNodeResult{
		Node:  node,
		Steps: steps,
	}, nil
}

func normalizeBootstrapInput(input *BootstrapNodeInput) {
	if input.SSHPort == 0 {
		input.SSHPort = 22
	}
	if input.NodePort == 0 {
		input.NodePort = 9090
	}
	if input.PublicHost == "" {
		input.PublicHost = input.IP
	}
	if input.SingboxReloadCommand == "" {
		input.SingboxReloadCommand = "systemctl restart meimei-sing-box.service"
	}
}

func buildBootstrapArtifact() (*bootstrapArtifact, error) {
	realityPrivateKey, realityPublicKey, err := generateRealityKeypair()
	if err != nil {
		return nil, err
	}

	shortID, err := generateRealityShortID()
	if err != nil {
		return nil, err
	}

	sshPrivateKeyPEM, sshPublicKey, err := generateSSHKeypair()
	if err != nil {
		return nil, err
	}

	return &bootstrapArtifact{
		token:             uuid.NewString(),
		realityPublicKey:  realityPublicKey,
		realityPrivateKey: realityPrivateKey,
		realityShortID:    shortID,
		realityServerName: "www.cloudflare.com",
		sshPrivateKeyPEM:  sshPrivateKeyPEM,
		sshPublicKey:      strings.TrimSpace(sshPublicKey),
	}, nil
}

func generateSSHKeypair() (string, string, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	})

	sshPublic, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return "", "", err
	}

	return string(privatePEM), string(ssh.MarshalAuthorizedKey(sshPublic)), nil
}

func (s *NodeService) openSSHClientWithPassword(input BootstrapNodeInput) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            input.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(input.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}

	address := net.JoinHostPort(input.IP, strconv.Itoa(input.SSHPort))
	return ssh.Dial("tcp", address, config)
}

func (s *NodeService) openSSHClientWithPrivateKey(host string, port int, username, privateKeyPEM string) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}

	address := net.JoinHostPort(host, strconv.Itoa(port))
	return ssh.Dial("tcp", address, config)
}

func installAuthorizedKey(client *ssh.Client, password string, authorizedKey string) error {
	cmd := fmt.Sprintf(
		"mkdir -p ~/.ssh && chmod 700 ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && grep -qxF %s ~/.ssh/authorized_keys || printf '%%s\\n' %s >> ~/.ssh/authorized_keys",
		shellQuote(authorizedKey),
		shellQuote(authorizedKey),
	)
	_, err := runRemoteCommand(client, password, false, cmd)
	return err
}

func (s *NodeService) upsertBootstrapNodeRecord(input BootstrapNodeInput, artifact *bootstrapArtifact) (*models.Node, error) {
	baseURL := fmt.Sprintf("http://%s:%d", input.IP, input.NodePort)
	node := models.Node{
		Name:              input.Name,
		BaseURL:           baseURL,
		Location:          input.Location,
		SSHHost:           input.IP,
		SSHPort:           input.SSHPort,
		SSHUsername:       input.Username,
		SSHPrivateKey:     artifact.sshPrivateKeyPEM,
		SSHPublicKey:      artifact.sshPublicKey,
		PublicHost:        input.PublicHost,
		VLESSPort:         0,
		TUICPort:          0,
		Hysteria2Port:     0,
		RealityPublicKey:  artifact.realityPublicKey,
		RealityShortID:    artifact.realityShortID,
		RealityServerName: artifact.realityServerName,
		ProtocolToken:     artifact.token,
		Enabled:           true,
		HealthStatus:      "bootstrap_pending",
		SingboxVersion:    "installer-managed",
	}

	if err := s.db.Where(models.Node{Name: input.Name}).Assign(node).FirstOrCreate(&node).Error; err != nil {
		return nil, err
	}

	return &node, nil
}

func buildNodeInstallCommand(node models.Node, sharedToken string) string {
	scriptURL := defaultNodeInstallerRawURL

	env := []string{
		"MEIMEI_REPO=" + shellQuote(defaultInstallRepo),
		"MEIMEI_VERSION=" + shellQuote(defaultInstallVersion),
		"MEIMEI_NODE_NAME=" + shellQuote(node.Name),
		"MEIMEI_NODE_PORT=" + shellQuote(strconv.Itoa(extractNodePort(node.BaseURL))),
		"MEIMEI_PUBLIC_HOST=" + shellQuote(node.PublicHost),
		"MEIMEI_NODE_TOKEN=" + shellQuote(node.ProtocolToken),
		"MEIMEI_CONTROL_PLANE_TOKEN=" + shellQuote(sharedToken),
	}

	return strings.Join(append(env, "bash <(curl -fsSL "+shellQuote(scriptURL)+")"), " ")
}

func buildNodeUninstallCommand(node models.Node) string {
	installDir := "/opt/meimei-node"

	return strings.Join([]string{
		"if command -v systemctl >/dev/null 2>&1; then systemctl disable --now meimei-node.service >/dev/null 2>&1 || true; systemctl disable --now meimei-sing-box.service >/dev/null 2>&1 || true; rm -f /etc/systemd/system/meimei-node.service /etc/systemd/system/meimei-sing-box.service; systemctl daemon-reload >/dev/null 2>&1 || true; systemctl reset-failed >/dev/null 2>&1 || true; fi",
		fmt.Sprintf("if command -v ufw >/dev/null 2>&1; then ufw --force delete allow %d/tcp >/dev/null 2>&1 || true; fi", extractNodePort(node.BaseURL)),
		"rm -rf " + installDir,
	}, "; ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func (s *NodeService) waitForNodeStatus(baseURL string) error {
	deadline := time.Now().Add(90 * time.Second)
	statusURL := strings.TrimRight(baseURL, "/") + "/status"
	for time.Now().Before(deadline) {
		resp, err := s.httpClient.Get(statusURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %s", statusURL)
}

func extractSSHHost(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func extractNodePort(baseURL string) int {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return 9090
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port == 0 {
		return 9090
	}
	return port
}

func runRemoteCommand(client *ssh.Client, password string, sudo bool, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if sudo {
		stdin, err := session.StdinPipe()
		if err != nil {
			return "", err
		}

		wrapped := fmt.Sprintf("sudo -S -p '' bash -lc %q", command)
		if err := session.Start(wrapped); err != nil {
			return "", err
		}
		if _, err := io.WriteString(stdin, password+"\n"); err != nil {
			return "", err
		}
		_ = stdin.Close()
		if err := session.Wait(); err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
	} else {
		if err := session.Run(fmt.Sprintf("bash -lc %q", command)); err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

func runSimpleRemoteCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(fmt.Sprintf("bash -lc %q", command)); err != nil {
		return strings.TrimSpace(stdout.String()), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func generateRealityKeypair() (string, string, error) {
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return "", "", err
	}
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}

	return base64.RawURLEncoding.EncodeToString(privateKey[:]), base64.RawURLEncoding.EncodeToString(publicKey), nil
}

func generateRealityShortID() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
