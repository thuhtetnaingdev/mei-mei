package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ReinstallService struct {
	binaryPath     string
	restartCommand string
	statusPath     string
	mu             sync.Mutex
	lastReinstall  time.Time
	lastError      string
}

func NewReinstallService(binaryPath, restartCommand string) *ReinstallService {
	service := &ReinstallService{
		binaryPath:      binaryPath,
		restartCommand:  restartCommand,
		statusPath:      binaryPath + ".reinstall-status.json",
	}
	service.loadStatus()
	return service
}

type ReinstallResult struct {
	Success     bool      `json:"success"`
	Message     string    `json:"message"`
	LastReinstall time.Time `json:"lastReinstall"`
	Error       string    `json:"error,omitempty"`
}

func (s *ReinstallService) ReinstallFromTarball(file io.Reader) (*ReinstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "node-reinstall-*")
	if err != nil {
		s.lastError = fmt.Sprintf("failed to create temp directory: %v", err)
		return &ReinstallResult{Success: false, Error: s.lastError}, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract the tarball
	if err := s.extractTarball(file, tempDir); err != nil {
		s.setLastError(fmt.Sprintf("failed to extract tarball: %v", err))
		return &ReinstallResult{Success: false, Error: s.lastError}, fmt.Errorf("failed to extract tarball: %w", err)
	}

	// Find the binary in the extracted files
	binaryFile, err := s.findBinary(tempDir)
	if err != nil {
		s.setLastError(fmt.Sprintf("failed to find binary: %v", err))
		return &ReinstallResult{Success: false, Error: s.lastError}, fmt.Errorf("failed to find binary: %w", err)
	}

	backupPath := s.binaryPath + ".backup"
	if err := s.backupBinary(backupPath); err != nil {
		s.setLastError(fmt.Sprintf("failed to backup binary: %v", err))
		return &ReinstallResult{Success: false, Error: s.lastError}, fmt.Errorf("failed to backup binary: %w", err)
	}

	stagedBinaryPath := s.binaryPath + ".incoming"
	if err := s.copyFile(binaryFile, stagedBinaryPath, 0o755); err != nil {
		s.setLastError(fmt.Sprintf("failed to stage binary: %v", err))
		return &ReinstallResult{Success: false, Error: s.lastError}, fmt.Errorf("failed to stage binary: %w", err)
	}

	if err := s.launchReplacement(stagedBinaryPath, backupPath); err != nil {
		s.setLastError(fmt.Sprintf("failed to launch replacement: %v", err))
		return &ReinstallResult{Success: false, Error: s.lastError}, fmt.Errorf("failed to launch replacement: %w", err)
	}

	s.setLastError("")

	return &ReinstallResult{
		Success:       true,
		Message:       "Reinstall scheduled successfully",
		LastReinstall: s.lastReinstall,
	}, nil
}

func (s *ReinstallService) extractTarball(reader io.Reader, destDir string) error {
	// First, read all data to ensure we have content
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read uploaded data: %w", err)
	}
	
	if len(data) == 0 {
		return fmt.Errorf("uploaded file is empty")
	}

	log.Printf("[reinstall] Received %d bytes of data", len(data))

	// Create gzip reader from the data
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader (invalid gzip data): %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		target := filepath.Join(destDir, header.Name)

		// Security check: prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			return fmt.Errorf("tarball contains path traversal attempt: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", header.Name, err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", header.Name, err)
			}

			outFile, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", header.Name, err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", header.Name, err)
			}
			outFile.Close()

		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("failed to create symlink %s -> %s: %w", header.Name, header.Linkname, err)
			}

		default:
			return fmt.Errorf("unsupported file type in tarball: %c for %s", header.Typeflag, header.Name)
		}
	}

	return nil
}

func (s *ReinstallService) findBinary(extractDir string) (string, error) {
	var binaryPath string

	// Look for the binary by walking the directory
	err := filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Check if file is executable or named similar to the target binary
		baseName := filepath.Base(path)
		targetBaseName := filepath.Base(s.binaryPath)

		// Match by name or if it's an executable file
		if baseName == targetBaseName || s.isExecutable(info) {
			binaryPath = path
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	if binaryPath == "" {
		return "", fmt.Errorf("no binary file found in tarball")
	}

	return binaryPath, nil
}

func (s *ReinstallService) isExecutable(info os.FileInfo) bool {
	// Check if file has executable permissions
	mode := info.Mode()
	return mode&0o111 != 0
}

func (s *ReinstallService) backupBinary(backupPath string) error {
	source, err := os.Open(s.binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing binary to backup
			return nil
		}
		return fmt.Errorf("failed to open current binary: %w", err)
	}
	defer source.Close()

	dest, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("failed to copy binary to backup: %w", err)
	}

	return nil
}

func (s *ReinstallService) restoreBinary(backupPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return nil // No backup to restore
	}

	source, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer source.Close()

	dest, err := os.Create(s.binaryPath)
	if err != nil {
		return fmt.Errorf("failed to create binary file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("failed to restore binary: %w", err)
	}

	if err := os.Chmod(s.binaryPath, 0o755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	return nil
}

func (s *ReinstallService) copyFile(sourcePath, destPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer func() {
		_ = dest.Close()
	}()

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	if err := dest.Close(); err != nil {
		return fmt.Errorf("failed to close target file: %w", err)
	}

	return os.Chmod(destPath, mode)
}

func (s *ReinstallService) launchReplacement(stagedBinaryPath, backupPath string) error {
	logPath := filepath.Join(filepath.Dir(s.binaryPath), "reinstall-helper.log")
	restartCmd := s.restartCommand
	if restartCmd == "" {
		restartCmd = s.binaryPath
	}

	quotedBinary := shellQuote(s.binaryPath)
	quotedIncoming := shellQuote(stagedBinaryPath)
	quotedBackup := shellQuote(backupPath)
	quotedStatus := shellQuote(s.statusPath)
	quotedRestart := shellQuote(restartCmd)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	statusJSON, err := json.Marshal(map[string]string{
		"lastReinstall": timestamp,
		"lastError":     "",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal helper status: %w", err)
	}

	scriptPath := filepath.Join(filepath.Dir(s.binaryPath), "reinstall-helper.sh")
	script := fmt.Sprintf(`#!/bin/sh
sleep 1
set -eu
mv %s %s
chmod 755 %s
rm -f %s
printf '%%s\n' %s > %s
exec sh -c %s
`, quotedIncoming, quotedBinary, quotedBinary, quotedBackup, shellQuote(string(statusJSON)), quotedStatus, quotedRestart)
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		return fmt.Errorf("failed to write helper script: %w", err)
	}

	cmd := exec.Command("nohup", "sh", scriptPath)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open helper log: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}

func (s *ReinstallService) loadStatus() {
	data, err := os.ReadFile(s.statusPath)
	if err != nil {
		return
	}

	var status struct {
		LastReinstall string `json:"lastReinstall"`
		LastError     string `json:"lastError"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return
	}

	s.lastError = status.LastError
	if status.LastReinstall != "" {
		if parsed, err := time.Parse(time.RFC3339, status.LastReinstall); err == nil {
			s.lastReinstall = parsed
		}
	}
}

func (s *ReinstallService) persistStatus() {
	payload := map[string]string{
		"lastReinstall": "",
		"lastError":     s.lastError,
	}
	if !s.lastReinstall.IsZero() {
		payload["lastReinstall"] = s.lastReinstall.UTC().Format(time.RFC3339)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[reinstall] failed to marshal status: %v", err)
		return
	}
	if err := os.WriteFile(s.statusPath, data, 0o644); err != nil {
		log.Printf("[reinstall] failed to write status: %v", err)
	}
}

func (s *ReinstallService) setLastError(message string) {
	s.lastError = message
	s.persistStatus()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (s *ReinstallService) GetStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loadStatus()

	return map[string]interface{}{
		"binaryPath":    s.binaryPath,
		"lastReinstall": s.lastReinstall,
		"lastError":     s.lastError,
	}
}
