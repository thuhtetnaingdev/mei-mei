package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"panel_backend/internal/models"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ssh"
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
	VLESSPort            int    `json:"vlessPort"`
	TUICPort             int    `json:"tuicPort"`
	Hysteria2Port        int    `json:"hysteria2Port"`
	SingboxReloadCommand string `json:"singboxReloadCommand"`
}

type BootstrapNodeResult struct {
	Node  *models.Node `json:"node"`
	Steps []string     `json:"steps"`
}

type bootstrapArtifact struct {
	tempDir           string
	tarballPath       string
	token             string
	realityPublicKey  string
	realityPrivateKey string
	realityShortID    string
	realityServerName string
	tlsCertificatePEM string
	tlsKeyPEM         string
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

	if input.SSHPort == 0 {
		input.SSHPort = 22
	}
	if input.NodePort == 0 {
		input.NodePort = 9090
	}
	if input.VLESSPort == 0 {
		input.VLESSPort = 443
	}
	if input.TUICPort == 0 {
		input.TUICPort = 8443
	}
	if input.Hysteria2Port == 0 {
		input.Hysteria2Port = 9443
	}
	if input.PublicHost == "" {
		input.PublicHost = input.IP
	}
	if input.SingboxReloadCommand == "" {
		input.SingboxReloadCommand = "systemctl restart meimei-sing-box.service"
	}
	addLog("bootstrap started for node %s (%s)", input.Name, input.IP)

	addStep("connecting to VPS over SSH")
	client, err := s.openSSHClient(input)
	if err != nil {
		return nil, fmt.Errorf("ssh connect failed: %w", err)
	}
	defer client.Close()
	addLog("SSH connection established to %s:%d as %s", input.IP, input.SSHPort, input.Username)

	arch, archOutput, err := detectRemoteArch(client, input.Password)
	if err != nil {
		return nil, fmt.Errorf("detecting remote architecture failed: %w", err)
	}
	addStep("detected VPS architecture: " + arch)
	addLog("remote architecture detection output: %s", archOutput)

	artifact, err := s.buildBootstrapArtifact(arch, input.PublicHost)
	if err != nil {
		return nil, fmt.Errorf("building node artifact failed: %w", err)
	}
	defer os.RemoveAll(artifact.tempDir)
	addStep("built Linux node deployment tarball")
	addLog("bootstrap artifact built in %s", artifact.tempDir)

	addStep("running apt update and apt upgrade")
	aptOutput, err := runRemoteCommand(client, input.Password, true, "apt update && DEBIAN_FRONTEND=noninteractive apt upgrade -y && apt install -y tar curl ca-certificates && curl -fsSL https://sing-box.app/install.sh | sh")
	addLog("apt/install output:\n%s", aptOutput)
	if err != nil {
		return nil, fmt.Errorf("remote apt bootstrap failed: %w", err)
	}

	addStep("uploading node binary bundle and service files")
	if err := s.uploadBootstrapFiles(client, input, artifact); err != nil {
		addLog("file upload failed: %v", err)
		return nil, fmt.Errorf("uploading files failed: %w", err)
	}
	addLog("uploaded tarball, env, certs, and service files to VPS")

	addStep("installing node service")
	installOutput, err := runRemoteCommand(client, input.Password, true, strings.Join([]string{
		"mkdir -p /opt/meimei-node",
		"tar -xzf /tmp/meimei-node-bootstrap/node_backend.tar.gz -C /opt/meimei-node",
		"install -m 0644 /tmp/meimei-node-bootstrap/node_backend.env /opt/meimei-node/.env",
		"install -m 0644 /tmp/meimei-node-bootstrap/tls.crt /opt/meimei-node/tls.crt",
		"install -m 0600 /tmp/meimei-node-bootstrap/tls.key /opt/meimei-node/tls.key",
		"install -m 0644 /tmp/meimei-node-bootstrap/meimei-node.service /etc/systemd/system/meimei-node.service",
		"install -m 0644 /tmp/meimei-node-bootstrap/meimei-sing-box.service /etc/systemd/system/meimei-sing-box.service",
		"chmod +x /opt/meimei-node/node_backend",
		"systemctl daemon-reload",
		"systemctl enable meimei-sing-box.service",
		"systemctl enable --now meimei-node.service",
	}, " && "))
	addLog("service installation output:\n%s", installOutput)
	if err != nil {
		return nil, fmt.Errorf("service installation failed: %w", err)
	}

	baseURL := fmt.Sprintf("http://%s:%d", input.IP, input.NodePort)
	addStep("waiting for node API handshake on " + baseURL)
	if err := s.waitForNodeStatus(baseURL); err != nil {
		addLog("node status check failed for %s: %v", baseURL, err)
		return nil, fmt.Errorf("node did not come online: %w", err)
	}
	addLog("node API responded successfully at %s/status", baseURL)

	node, err := s.Register(RegisterNodeInput{
		Name:              input.Name,
		BaseURL:           baseURL,
		Location:          input.Location,
		PublicHost:        input.PublicHost,
		VLESSPort:         input.VLESSPort,
		TUICPort:          input.TUICPort,
		Hysteria2Port:     input.Hysteria2Port,
		RealityPublicKey:  artifact.realityPublicKey,
		RealityShortID:    artifact.realityShortID,
		RealityServerName: artifact.realityServerName,
		ProtocolToken:     artifact.token,
		SingboxVersion:    "bootstrap-managed",
		RegistrationToken: s.sharedToken,
	})
	if err != nil {
		return nil, fmt.Errorf("registering node failed: %w", err)
	}
	addStep("registered node in control plane")
	addLog("node %s registered with public host %s", node.Name, node.PublicHost)

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

func (s *NodeService) openSSHClient(input BootstrapNodeInput) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            input.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(input.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}

	address := fmt.Sprintf("%s:%d", input.IP, input.SSHPort)
	return ssh.Dial("tcp", address, config)
}

func detectRemoteArch(client *ssh.Client, password string) (string, string, error) {
	output, err := runRemoteCommand(client, password, false, "uname -m")
	if err != nil {
		return "", "", err
	}

	arch := strings.TrimSpace(output)
	switch arch {
	case "x86_64", "amd64":
		return "amd64", output, nil
	case "aarch64", "arm64":
		return "arm64", output, nil
	default:
		return "", output, fmt.Errorf("unsupported remote architecture %q", arch)
	}
}

func (s *NodeService) buildBootstrapArtifact(goarch string, publicHost string) (*bootstrapArtifact, error) {
	repoRoot, err := filepath.Abs(filepath.Join(".", ".."))
	if err != nil {
		return nil, err
	}

	nodeSourceDir := filepath.Join(repoRoot, "node_backend")
	if _, err := os.Stat(nodeSourceDir); err != nil {
		return nil, errors.New("node_backend source directory not found relative to panel_backend")
	}

	tempDir, err := os.MkdirTemp("", "meimei-node-bootstrap-*")
	if err != nil {
		return nil, err
	}

	binaryPath := filepath.Join(tempDir, "node_backend")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
	cmd.Dir = nodeSourceDir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, string(output))
	}

	tarballPath := filepath.Join(tempDir, "node_backend.tar.gz")
	if err := archiveSingleFile(binaryPath, tarballPath, "node_backend", 0o755); err != nil {
		return nil, err
	}

	realityPrivateKey, realityPublicKey, err := generateRealityKeypair()
	if err != nil {
		return nil, err
	}

	shortID, err := generateRealityShortID()
	if err != nil {
		return nil, err
	}

	tlsCertificatePEM, tlsKeyPEM, err := generateSelfSignedCertificate(publicHost)
	if err != nil {
		return nil, err
	}

	return &bootstrapArtifact{
		tempDir:           tempDir,
		tarballPath:       tarballPath,
		token:             uuid.NewString(),
		realityPublicKey:  realityPublicKey,
		realityPrivateKey: realityPrivateKey,
		realityShortID:    shortID,
		realityServerName: "www.cloudflare.com",
		tlsCertificatePEM: tlsCertificatePEM,
		tlsKeyPEM:         tlsKeyPEM,
	}, nil
}

func (s *NodeService) uploadBootstrapFiles(client *ssh.Client, input BootstrapNodeInput, artifact *bootstrapArtifact) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if err := runSimpleRemoteCommand(client, "mkdir -p /tmp/meimei-node-bootstrap"); err != nil {
		return err
	}

	if err := uploadFile(sftpClient, artifact.tarballPath, "/tmp/meimei-node-bootstrap/node_backend.tar.gz"); err != nil {
		return err
	}

	envContent := strings.Join([]string{
		fmt.Sprintf("PORT=%d", input.NodePort),
		fmt.Sprintf("NODE_NAME=%s", input.Name),
		fmt.Sprintf("NODE_TOKEN=%s", artifact.token),
		fmt.Sprintf("CONTROL_PLANE_SHARED_TOKEN=%s", s.sharedToken),
		"SINGBOX_CONFIG_PATH=/opt/meimei-node/sing-box.generated.json",
		fmt.Sprintf("SINGBOX_RELOAD_COMMAND=%s", shellEscapeEnv(input.SingboxReloadCommand)),
		fmt.Sprintf("PUBLIC_HOST=%s", input.PublicHost),
		fmt.Sprintf("VLESS_PORT=%d", input.VLESSPort),
		fmt.Sprintf("TUIC_PORT=%d", input.TUICPort),
		fmt.Sprintf("HYSTERIA2_PORT=%d", input.Hysteria2Port),
		fmt.Sprintf("VLESS_REALITY_PRIVATE_KEY=%s", artifact.realityPrivateKey),
		fmt.Sprintf("VLESS_REALITY_PUBLIC_KEY=%s", artifact.realityPublicKey),
		fmt.Sprintf("VLESS_REALITY_SHORT_ID=%s", artifact.realityShortID),
		fmt.Sprintf("VLESS_REALITY_SERVER_NAME=%s", artifact.realityServerName),
		fmt.Sprintf("VLESS_REALITY_HANDSHAKE_SERVER=%s", artifact.realityServerName),
		"VLESS_REALITY_HANDSHAKE_PORT=443",
		"TLS_CERTIFICATE_PATH=/opt/meimei-node/tls.crt",
		"TLS_KEY_PATH=/opt/meimei-node/tls.key",
		fmt.Sprintf("TLS_SERVER_NAME=%s", input.PublicHost),
		"",
	}, "\n")

	if err := writeRemoteFile(sftpClient, "/tmp/meimei-node-bootstrap/node_backend.env", envContent, 0o600); err != nil {
		return err
	}

	if err := writeRemoteFile(sftpClient, "/tmp/meimei-node-bootstrap/tls.crt", artifact.tlsCertificatePEM, 0o644); err != nil {
		return err
	}

	if err := writeRemoteFile(sftpClient, "/tmp/meimei-node-bootstrap/tls.key", artifact.tlsKeyPEM, 0o600); err != nil {
		return err
	}

	serviceContent := `[Unit]
Description=Meimei Node Backend
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/meimei-node
EnvironmentFile=/opt/meimei-node/.env
ExecStart=/opt/meimei-node/node_backend
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`

	if err := writeRemoteFile(sftpClient, "/tmp/meimei-node-bootstrap/meimei-node.service", serviceContent, 0o644); err != nil {
		return err
	}

	singBoxServiceContent := `[Unit]
Description=Meimei Sing-box
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/sing-box run -c /opt/meimei-node/sing-box.generated.json
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`

	return writeRemoteFile(sftpClient, "/tmp/meimei-node-bootstrap/meimei-sing-box.service", singBoxServiceContent, 0o644)
}

func (s *NodeService) waitForNodeStatus(baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	for attempt := 0; attempt < 15; attempt++ {
		resp, err := client.Get(baseURL + "/status")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
		}

		time.Sleep(2 * time.Second)
	}

	return errors.New("status endpoint did not become ready in time")
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

		wrapped := fmt.Sprintf("sudo -S -p '' sh -lc %q", command)
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
		if err := session.Run(fmt.Sprintf("sh -lc %q", command)); err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

func runSimpleRemoteCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return session.Run(fmt.Sprintf("sh -lc %q", command))
}

func uploadFile(client *sftp.Client, localPath, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return err
	}

	return nil
}

func writeRemoteFile(client *sftp.Client, remotePath, content string, mode os.FileMode) error {
	file, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write([]byte(content)); err != nil {
		return err
	}

	return client.Chmod(remotePath, mode)
}

func archiveSingleFile(sourcePath, targetPath, name string, mode int64) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer target.Close()

	gzipWriter := gzip.NewWriter(target)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	log.Printf("[archive] Archiving file: %s (size: %d bytes)", sourcePath, info.Size())

	header := &tar.Header{
		Name:    name,
		Mode:    mode,
		Size:    info.Size(),
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	written, err := io.Copy(tarWriter, source)
	if err != nil {
		return err
	}

	log.Printf("[archive] Written %d bytes to tar", written)

	if err := tarWriter.Close(); err != nil {
		return err
	}

	if err := gzipWriter.Close(); err != nil {
		return err
	}

	// Verify final size
	finalInfo, err := os.Stat(targetPath)
	if err != nil {
		return err
	}
	log.Printf("[archive] Final tarball size: %d bytes", finalInfo.Size())

	return nil
}

func shellEscapeEnv(value string) string {
	return strings.ReplaceAll(value, "\n", " ")
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

func generateSelfSignedCertificate(host string) (string, string, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	dnsNames := []string{"localhost"}
	ipAddresses := []net.IP{net.ParseIP("127.0.0.1")}
	if parsedIP := net.ParseIP(host); parsedIP != nil {
		ipAddresses = append(ipAddresses, parsedIP)
	} else if host != "" {
		dnsNames = append(dnsNames, host)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "meimei-node",
			Organization: []string{"Meimei"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", err
	}

	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	privateKeyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKeyDER})

	return string(certificatePEM), string(privateKeyPEM), nil
}
