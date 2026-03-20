package api

import (
	"log"
	"net/http"
	"node_backend/internal/auth"
	"node_backend/internal/config"
	"node_backend/internal/services"
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

			if err := configService.Apply(req); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "applied"})
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
	}

	return router
}
