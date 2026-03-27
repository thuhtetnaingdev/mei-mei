package api

import (
	"io"
	"log"
	"net/http"
	"node_backend/internal/auth"
	"node_backend/internal/config"
	"node_backend/internal/services"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func NewRouter(cfg config.Config) *gin.Engine {
	configService := services.NewConfigService(cfg)
	return NewRouterWithConfigService(cfg, configService)
}

func NewRouterWithConfigService(cfg config.Config, configService *services.ConfigService) *gin.Engine {
	router := gin.Default()
	reinstallService := services.NewReinstallService(cfg.NodeBinaryPath, cfg.NodeRestartCommand)
	uninstallService := services.NewUninstallService(cfg)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, configService.Status())
	})

	// Bandwidth usage endpoint - authenticated for control plane collection
	router.GET("/bandwidth-usage", func(c *gin.Context) {
		controlToken := c.GetHeader("X-Control-Plane-Token")
		if controlToken != cfg.ControlPlaneSharedToken {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid control plane token"})
			return
		}

		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if token != "" && token != cfg.NodeToken {
			log.Printf("[bandwidth-usage] accepting request with stale bearer token because control plane token is valid")
		}

		tracker := configService.GetBandwidthTracker()
		usage := tracker.GetAndResetUsage()

		// Format response as required: { "users": [{"uuid": "...", "bytes": 12345}, ...] }
		userUsage := make([]gin.H, 0, len(usage))
		for _, u := range usage {
			userUsage = append(userUsage, gin.H{
				"uuid":  u.UUID,
				"bytes": u.BytesUsed,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"nodeName":   cfg.NodeName,
			"reportTime": time.Now(),
			"totalUsers": len(userUsage),
			"users":      userUsage,
		})
	})

	router.GET("/reinstall/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, reinstallService.GetStatus())
	})

	protected := router.Group("/")
	protected.Use(auth.ControlPlaneAuth(cfg.NodeToken, cfg.ControlPlaneSharedToken))
	{
		protected.POST("/apply-config", func(c *gin.Context) {
			var req services.ApplyConfigRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Use debounced apply for better resource efficiency
			// Multiple rapid config changes will be coalesced into a single apply
			if err := configService.ApplyDebounced(req); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "config applied (debounced)"})
		})

		protected.POST("/reinstall", func(c *gin.Context) {
			file, header, err := c.Request.FormFile("tarball")
			if err != nil {
				log.Printf("[reinstall] FormFile error: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to get tarball file: " + err.Error()})
				return
			}
			defer file.Close()

			log.Printf("[reinstall] Received file: %s, size: %d bytes", header.Filename, header.Size)

			if header.Size > 100*1024*1024 { // 100MB limit
				c.JSON(http.StatusBadRequest, gin.H{"error": "tarball file too large (max 100MB)"})
				return
			}

			result, err := reinstallService.ReinstallFromTarball(file)
			if err != nil {
				log.Printf("[reinstall] Reinstall error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   err.Error(),
				})
				return
			}

			status := http.StatusOK
			if !result.Success {
				status = http.StatusInternalServerError
			}

			log.Printf("[reinstall] Result: success=%v, message=%s", result.Success, result.Message)
			c.JSON(status, result)
		})

		protected.POST("/uninstall", func(c *gin.Context) {
			result, err := uninstallService.Schedule()
			if err != nil {
				log.Printf("[uninstall] schedule error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusAccepted, result)
		})

		protected.POST("/update-reality-keys", func(c *gin.Context) {
			var req struct {
				PrivateKey string `json:"privateKey"`
				PublicKey  string `json:"publicKey"`
				ShortID    string `json:"shortId"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
				return
			}

			// Validate required fields
			if req.PrivateKey == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "privateKey is required"})
				return
			}
			if req.PublicKey == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "publicKey is required"})
				return
			}
			if req.ShortID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "shortId is required"})
				return
			}

			if err := configService.UpdateRealityKeys(req.PrivateKey, req.PublicKey, req.ShortID); err != nil {
				log.Printf("[update-reality-keys] failed: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "reality keys updated successfully",
			})
		})

		protected.GET("/speed-test/download", func(c *gin.Context) {
			// Parse size parameter with validation
			size := parseSpeedTestSize(c.Query("bytes"), 1024*1024, 256*1024, 2*1024*1024)

			// Set headers for streaming
			c.Header("Content-Type", "application/octet-stream")
			c.Header("Cache-Control", "no-store")
			c.Header("X-Content-Type-Options", "nosniff")

			// Stream in chunks instead of allocating full buffer
			pattern := []byte("MEIMEI_SPEED_TEST_")
			chunkSize := 8192 // 8KB chunks
			chunk := make([]byte, chunkSize)
			remaining := size

			// Flush headers immediately
			c.Status(http.StatusOK)
			w := c.Writer

			for remaining > 0 {
				n := chunkSize
				if remaining < n {
					n = remaining
				}

				// Fill chunk with pattern
				for i := 0; i < n; i++ {
					chunk[i] = pattern[i%len(pattern)]
				}

				// Write chunk
				if _, err := w.Write(chunk[:n]); err != nil {
					log.Printf("[speed-test] write error: %v", err)
					return
				}

				remaining -= n

				// Flush to client
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}

			log.Printf("[speed-test] completed streaming %d bytes", size)
		})

		protected.POST("/speed-test/upload", func(c *gin.Context) {
			size, err := io.Copy(io.Discard, c.Request.Body)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"receivedBytes": size,
				"status":        "ok",
			})
		})
	}

	return router
}

func parseSpeedTestSize(value string, fallback, min, max int) int {
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}
