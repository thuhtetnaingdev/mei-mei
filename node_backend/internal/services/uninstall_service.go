package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"node_backend/internal/config"
)

const (
	defaultNodeServiceName    = "meimei-node.service"
	defaultSingboxServiceName = "meimei-sing-box.service"
	uninstallLogPath          = "/tmp/meimei-node-uninstall.log"
)

type UninstallService struct {
	cfg config.Config
}

type UninstallResult struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
}

func NewUninstallService(cfg config.Config) *UninstallService {
	return &UninstallService{cfg: cfg}
}

func (s *UninstallService) Schedule() (*UninstallResult, error) {
	installDir := filepath.Clean(filepath.Dir(s.cfg.NodeBinaryPath))
	if !filepath.IsAbs(installDir) || installDir == "/" || installDir == "." {
		return nil, fmt.Errorf("refusing to uninstall from unsafe install directory %q", installDir)
	}

	logFile, err := os.OpenFile(uninstallLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open uninstall log: %w", err)
	}

	cmd := exec.Command("sh", "-c", s.buildScript(installDir))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start uninstall worker: %w", err)
	}

	go func() {
		defer logFile.Close()
		_ = cmd.Wait()
	}()

	return &UninstallResult{
		Accepted: true,
		Message:  "node uninstall scheduled",
	}, nil
}

func (s *UninstallService) buildScript(installDir string) string {
	nodeUnitPath := "/etc/systemd/system/" + defaultNodeServiceName
	singboxUnitPath := "/etc/systemd/system/" + defaultSingboxServiceName

	return strings.Join([]string{
		"sleep 2",
		"if command -v systemctl >/dev/null 2>&1; then",
		"  systemctl disable --now " + shellQuote(defaultNodeServiceName) + " >/dev/null 2>&1 || true",
		"  systemctl disable --now " + shellQuote(defaultSingboxServiceName) + " >/dev/null 2>&1 || true",
		"  rm -f " + shellQuote(nodeUnitPath) + " " + shellQuote(singboxUnitPath),
		"  systemctl daemon-reload >/dev/null 2>&1 || true",
		"  systemctl reset-failed >/dev/null 2>&1 || true",
		"fi",
		"if command -v ufw >/dev/null 2>&1; then",
		"  ufw --force delete allow " + shellQuote(s.cfg.Port+"/tcp") + " >/dev/null 2>&1 || true",
		s.firewallCleanupCommands(),
		"fi",
		"rm -f " + shellQuote(s.cfg.SingboxConfigPath) + " " + shellQuote(s.cfg.TLSCertificatePath) + " " + shellQuote(s.cfg.TLSKeyPath),
		"rm -f " + shellQuote(s.cfg.NodeBinaryPath) + " " + shellQuote(s.cfg.NodeBinaryPath+".backup") + " " + shellQuote(s.cfg.NodeBinaryPath+".incoming") + " " + shellQuote(s.cfg.NodeBinaryPath+".reinstall-status.json"),
		"rm -f " + shellQuote(filepath.Join(installDir, ".env")) + " " + shellQuote(filepath.Join(installDir, ".env.example")),
		"rm -rf " + shellQuote(installDir),
	}, "\n")
}

func (s *UninstallService) firewallCleanupCommands() string {
	payload, err := os.ReadFile(s.cfg.SingboxConfigPath)
	if err != nil {
		return "  true"
	}

	var parsed struct {
		Inbounds []struct {
			Type       string `json:"type"`
			ListenPort int    `json:"listen_port"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "  true"
	}

	commands := make([]string, 0, len(parsed.Inbounds))
	seen := make(map[string]struct{}, len(parsed.Inbounds))
	for _, inbound := range parsed.Inbounds {
		if inbound.ListenPort <= 0 {
			continue
		}

		protocol := "tcp"
		if inbound.Type == "tuic" || inbound.Type == "hysteria2" {
			protocol = "udp"
		}

		rule := fmt.Sprintf("%d/%s", inbound.ListenPort, protocol)
		if _, exists := seen[rule]; exists {
			continue
		}
		seen[rule] = struct{}{}
		commands = append(commands, "  ufw --force delete allow "+shellQuote(rule)+" >/dev/null 2>&1 || true")
	}

	if len(commands) == 0 {
		return "  true"
	}

	return strings.Join(commands, "\n")
}
