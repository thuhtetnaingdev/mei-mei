package api

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"panel_backend/internal/auth"
	"panel_backend/internal/config"
	"panel_backend/internal/models"
	"panel_backend/internal/services"
	"panel_backend/internal/subscription"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	cfg                         config.Config
	jwt                         *auth.JWTManager
	adminService                *services.AdminService
	userService                 *services.UserService
	minerService                *services.MinerService
	nodeService                 *services.NodeService
	mintPoolService             *services.MintPoolService
	bandwidthReportService      *services.BandwidthReportService
	bandwidthCollector          *services.BandwidthCollectorService
	userClassificationService   *services.UserClassificationService
	userClassificationScheduler *services.UserClassificationScheduler
}

func NewRouter(cfg config.Config, db *gorm.DB) *gin.Engine {
	userService := services.NewUserService(db)
	nodeService := services.NewNodeService(db, cfg.NodeSharedToken, timeDurationSeconds(cfg.SyncTimeoutSeconds), userService)
	bandwidthCollector := services.NewBandwidthCollectorService(services.BandwidthCollectorConfig{
		DB:              db,
		NodeSharedToken: cfg.NodeSharedToken,
		CollectInterval: 10 * time.Second,
		RequestTimeout:  30 * time.Second,
		UserService:     userService,
		NodeService:     nodeService,
	})
	userClassificationService := services.NewUserClassificationService(db)
	userClassificationScheduler := services.NewUserClassificationScheduler(userClassificationService, 24*time.Hour)
	return NewRouterWithServices(cfg, db, userService, nodeService, bandwidthCollector, userClassificationService, userClassificationScheduler)
}

func NewRouterWithServices(cfg config.Config, db *gorm.DB, userService *services.UserService, nodeService *services.NodeService, bandwidthCollector *services.BandwidthCollectorService, userClassificationService *services.UserClassificationService, userClassificationScheduler *services.UserClassificationScheduler) *gin.Engine {
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins: cfg.AllowedOrigins,
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
		MaxAge:       12 * time.Hour,
	}))

	handler := &Handler{
		cfg:                         cfg,
		jwt:                         auth.NewJWTManager(cfg.JWTSecret),
		adminService:                services.NewAdminService(db, cfg.AdminUsername, cfg.AdminPassword),
		userService:                 userService,
		minerService:                services.NewMinerService(db),
		mintPoolService:             services.NewMintPoolService(db),
		bandwidthReportService:      services.NewBandwidthReportService(db),
		nodeService:                 nodeService,
		bandwidthCollector:          bandwidthCollector,
		userClassificationService:   userClassificationService,
		userClassificationScheduler: userClassificationScheduler,
	}

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.POST("/auth/login", handler.login)
	router.GET("/subscription/:userId", handler.getSubscription)
	router.GET("/profiles/singbox/:uuid", handler.getSingboxProfile)

	// Node API endpoints - authenticated via node tokens
	nodeAPI := router.Group("/api/nodes")
	{
		nodeAPI.POST("/bandwidth-report", handler.nodeBandwidthReport)
	}

	protected := router.Group("/api")
	protected.Use(handler.jwt.Middleware())
	{
		protected.POST("/users", handler.createUser)
		protected.GET("/users", handler.listUsers)
		protected.GET("/users/:id", handler.getUser)
		protected.GET("/users/:id/records", handler.listUserRecords)
		protected.POST("/users/:id/bandwidth-allocations", handler.addUserBandwidthAllocation)
		protected.PATCH("/users/:id/bandwidth-allocations/:allocationId", handler.updateUserBandwidthAllocation)
		protected.POST("/users/:id/bandwidth-allocations/:allocationId/adjust", handler.adjustUserBandwidthAllocation)
		protected.POST("/users/:id/bandwidth-reductions", handler.reduceUserBandwidthAllocation)
		protected.PATCH("/users/:id", handler.updateUser)
		protected.DELETE("/users/:id", handler.deleteUser)
		protected.POST("/miners", handler.createMiner)
		protected.GET("/miners", handler.listMiners)
		protected.GET("/miners/:id", handler.getMiner)
		protected.PATCH("/miners/:id", handler.updateMiner)
		protected.DELETE("/miners/:id", handler.deleteMiner)

		protected.POST("/nodes/register", handler.registerNode)
		protected.POST("/nodes/bootstrap", handler.bootstrapNode)
		protected.GET("/nodes/bootstrap/:jobId", handler.getBootstrapStatus)
		protected.GET("/nodes", handler.listNodes)
		protected.POST("/nodes/diagnostics", handler.runNodeDiagnostics)
		protected.PATCH("/nodes/:id", handler.updateNode)
		protected.DELETE("/nodes/:id", handler.deleteNode)
		protected.POST("/nodes/:id/uninstall", handler.uninstallNode)
		protected.POST("/nodes/:id/reinstall", handler.reinstallNode)
		protected.POST("/nodes/sync", handler.syncNodes)
		protected.POST("/bandwidth/collect", handler.triggerBandwidthCollection)
		protected.GET("/bandwidth/status", handler.getBandwidthCollectorStatus)
		protected.POST("/users/classify", handler.triggerUserClassification)
		protected.GET("/users/classification/stats", handler.getUserClassificationStats)
		protected.GET("/users/classification/status", handler.getUserClassificationStatus)
		protected.GET("/admin/profile", handler.getAdminProfile)
		protected.PUT("/admin/credentials", handler.updateAdminCredentials)
		protected.GET("/settings/distribution", handler.getDistributionSettings)
		protected.PUT("/settings/distribution", handler.updateDistributionSettings)
		protected.GET("/settings/protocols", handler.getProtocolSettings)
		protected.PUT("/settings/protocols", handler.updateProtocolSettings)
		protected.GET("/mint-pool", handler.getMintPool)
		protected.POST("/mint-pool/mint", handler.mintPool)
		protected.DELETE("/mint-pool", handler.resetMintPool)
	}

	registerFrontendRoutes(router, cfg)

	return router
}

func registerFrontendRoutes(router *gin.Engine, cfg config.Config) {
	if cfg.FrontendDistDir == "" {
		return
	}

	indexPath := filepath.Join(cfg.FrontendDistDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		log.Printf("frontend dist not found at %s: %v", indexPath, err)
		return
	}

	serveIndex := func(c *gin.Context) {
		c.File(indexPath)
	}

	assetsDir := filepath.Join(cfg.FrontendDistDir, "assets")
	if _, err := os.Stat(assetsDir); err == nil {
		router.StaticFS("/assets", http.Dir(assetsDir))
	}

	for _, name := range []string{"favicon.ico", "manifest.webmanifest", "robots.txt"} {
		filePath := filepath.Join(cfg.FrontendDistDir, name)
		if _, err := os.Stat(filePath); err == nil {
			router.StaticFile("/"+name, filePath)
		}
	}

	router.GET("/", serveIndex)
	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/auth/") || strings.HasPrefix(path, "/subscription/") || strings.HasPrefix(path, "/profiles/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		serveIndex(c)
	})
}

func (h *Handler) getMintPool(c *gin.Context) {
	snapshot, err := h.mintPoolService.GetSnapshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

func (h *Handler) mintPool(c *gin.Context) {
	var input services.MintPoolMintInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	snapshot, err := h.mintPoolService.Mint(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, snapshot)
}

func (h *Handler) resetMintPool(c *gin.Context) {
	snapshot, err := h.mintPoolService.Reset()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

func (h *Handler) login(c *gin.Context) {
	var body struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !h.adminService.ValidateCredentials(body.Username, body.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := h.jwt.GenerateToken(body.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *Handler) getAdminProfile(c *gin.Context) {
	c.JSON(http.StatusOK, h.adminService.GetProfile())
}

func (h *Handler) updateAdminCredentials(c *gin.Context) {
	var input services.UpdateAdminCredentialsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	profile, err := h.adminService.UpdateCredentials(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, profile)
}

func (h *Handler) getDistributionSettings(c *gin.Context) {
	settings, err := h.adminService.GetDistributionSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *Handler) updateDistributionSettings(c *gin.Context) {
	var input services.DistributionSettings
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	settings, err := h.adminService.UpdateDistributionSettings(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func (h *Handler) getProtocolSettings(c *gin.Context) {
	settings, err := h.adminService.GetProtocolSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func (h *Handler) updateProtocolSettings(c *gin.Context) {
	var input services.ProtocolSettings
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	settings, err := h.adminService.UpdateProtocolSettings(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response := services.ProtocolSettingsUpdateResponse{
		RealitySNIs:          settings.RealitySNIs,
		Hysteria2Masquerades: settings.Hysteria2Masquerades,
	}

	activeUsers, err := h.userService.ActiveUsers()
	if err != nil {
		response.SyncError = "settings saved, but loading active users for sync failed: " + err.Error()
		c.JSON(http.StatusOK, response)
		return
	}

	results, err := h.nodeService.SyncAllUsers(activeUsers)
	if err != nil {
		response.SyncError = "settings saved, but node sync failed: " + err.Error()
		c.JSON(http.StatusOK, response)
		return
	}

	failures := make([]string, 0)
	for _, result := range results {
		status, _ := result["status"].(string)
		if status == "success" {
			response.SyncedNodes++
			continue
		}

		nodeName, _ := result["node"].(string)
		errorMessage, _ := result["error"].(string)
		if errorMessage == "" {
			errorMessage = "sync failed"
		}
		failures = append(failures, strings.TrimSpace(nodeName+": "+errorMessage))
	}
	if len(failures) > 0 {
		response.SyncError = "settings saved, but some nodes failed to sync: " + strings.Join(failures, "; ")
	}

	c.JSON(http.StatusOK, response)
}

func (h *Handler) createUser(c *gin.Context) {
	var input services.CreateUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.Create(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.syncActiveUsersBestEffort()
	c.JSON(http.StatusCreated, user)
}

func (h *Handler) createMiner(c *gin.Context) {
	var input services.CreateMinerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	miner, err := h.minerService.Create(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, miner)
}

func (h *Handler) listMiners(c *gin.Context) {
	miners, err := h.minerService.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, miners)
}

func (h *Handler) getMiner(c *gin.Context) {
	miner, err := h.minerService.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "miner not found"})
		return
	}
	c.JSON(http.StatusOK, miner)
}

func (h *Handler) updateMiner(c *gin.Context) {
	var input services.UpdateMinerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	miner, err := h.minerService.Update(c.Param("id"), input)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "miner not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, miner)
}

func (h *Handler) deleteMiner(c *gin.Context) {
	if err := h.minerService.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "miner not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listUsers(c *gin.Context) {
	users, err := h.userService.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *Handler) getUser(c *gin.Context) {
	user, err := h.userService.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *Handler) listUserRecords(c *gin.Context) {
	records, err := h.userService.ListRecords(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}

func (h *Handler) addUserBandwidthAllocation(c *gin.Context) {
	var input services.UserBandwidthAllocationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.AddBandwidthAllocation(c.Param("id"), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.syncActiveUsersBestEffort()
	c.JSON(http.StatusCreated, user)
}

func (h *Handler) updateUserBandwidthAllocation(c *gin.Context) {
	var input services.UserBandwidthAllocationUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.UpdateBandwidthAllocation(c.Param("id"), c.Param("allocationId"), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.syncActiveUsersBestEffort()
	c.JSON(http.StatusOK, user)
}

func (h *Handler) adjustUserBandwidthAllocation(c *gin.Context) {
	var input services.UserBandwidthAllocationAdjustmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.AdjustBandwidthAllocation(c.Param("id"), c.Param("allocationId"), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.syncActiveUsersBestEffort()
	c.JSON(http.StatusOK, user)
}

func (h *Handler) reduceUserBandwidthAllocation(c *gin.Context) {
	var input services.UserBandwidthReductionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.ReduceBandwidthAllocation(c.Param("id"), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.syncActiveUsersBestEffort()
	c.JSON(http.StatusOK, user)
}

func (h *Handler) updateUser(c *gin.Context) {
	var input services.UpdateUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.Update(c.Param("id"), input)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	h.syncActiveUsersBestEffort()
	c.JSON(http.StatusOK, user)
}

func (h *Handler) deleteUser(c *gin.Context) {
	if err := h.userService.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	h.syncActiveUsersBestEffort()
	c.Status(http.StatusNoContent)
}

func (h *Handler) syncActiveUsersBestEffort() {
	users, err := h.userService.ActiveUsers()
	if err != nil {
		log.Printf("[user-sync] loading active users failed: %v", err)
		return
	}

	results, err := h.nodeService.SyncAllUsers(users)
	if err != nil {
		log.Printf("[user-sync] syncing nodes failed: %v", err)
		return
	}

	for _, result := range results {
		status, _ := result["status"].(string)
		if status == "success" {
			continue
		}
		nodeName, _ := result["node"].(string)
		errorMessage, _ := result["error"].(string)
		if errorMessage == "" {
			errorMessage = "sync failed"
		}
		log.Printf("[user-sync] node %s sync failed: %s", nodeName, errorMessage)
	}
}

func (h *Handler) registerNode(c *gin.Context) {
	var input services.RegisterNodeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.RegistrationToken != h.cfg.NodeSharedToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid registration token"})
		return
	}

	node, err := h.nodeService.Register(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, node)
}

func (h *Handler) listNodes(c *gin.Context) {
	nodes, err := h.nodeService.ListWithRuntimeStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

func (h *Handler) updateNode(c *gin.Context) {
	var input services.UpdateNodeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	node, err := h.nodeService.Update(c.Param("id"), input)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, node)
}

func (h *Handler) deleteNode(c *gin.Context) {
	if err := h.nodeService.Delete(c.Param("id")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		var remoteErr *services.NodeDeleteRemoteError
		if errors.As(err, &remoteErr) {
			c.JSON(http.StatusBadGateway, gin.H{"error": remoteErr.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) uninstallNode(c *gin.Context) {
	if err := h.nodeService.Uninstall(c.Param("id")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		var remoteErr *services.NodeDeleteRemoteError
		if errors.As(err, &remoteErr) {
			c.JSON(http.StatusBadGateway, gin.H{"error": remoteErr.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) reinstallNode(c *gin.Context) {
	var input services.ReinstallNodeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		// Allow empty body, just set default empty input
		log.Printf("[reinstall] JSON bind error (may be empty body): %v", err)
		input = services.ReinstallNodeInput{}
	}

	log.Printf("[reinstall] Handler called for node %s with input: %+v", c.Param("id"), input)

	result, err := h.nodeService.Reinstall(c.Param("id"), input)
	if err != nil {
		log.Printf("[reinstall] Service error: %v", err)
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !result.Success {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": result.Success,
			"message": result.Message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": result.Success,
		"message": result.Message,
	})
}

func (h *Handler) bootstrapNode(c *gin.Context) {
	var input services.BootstrapNodeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job := h.nodeService.StartBootstrap(input)
	c.JSON(http.StatusAccepted, job)
}

func (h *Handler) getBootstrapStatus(c *gin.Context) {
	job, err := h.nodeService.GetBootstrapJob(c.Param("jobId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bootstrap job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

func (h *Handler) syncNodes(c *gin.Context) {
	users, err := h.userService.ActiveUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	results, err := h.nodeService.SyncAllUsers(users)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"syncedUsers": len(users),
		"results":     results,
	})
}

func (h *Handler) runNodeDiagnostics(c *gin.Context) {
	results, err := h.nodeService.RunDiagnostics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

func (h *Handler) getSubscription(c *gin.Context) {
	user, err := h.userService.GetByID(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	nodes, err := h.nodeService.ListWithRuntimeStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	remoteProfileURL := h.cfg.BasePublicURL + "/profiles/singbox/" + user.UUID
	clashProfileURL := remoteProfileURL + "?format=clash"
	importURL := "sing-box://import-remote-profile?url=" + url.QueryEscape(remoteProfileURL) + "&name=" + url.QueryEscape(user.Email)
	protocolSettings, err := h.adminService.GetProtocolSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"userId":           user.ID,
		"uuid":             user.UUID,
		"subscription":     subscription.Generate(*user, nodes, protocolSettings),
		"nodeLinks":        subscription.GenerateNodeLinks(*user, nodes, protocolSettings),
		"url":              h.cfg.BaseSubscriptionURL + "/" + c.Param("userId"),
		"remoteProfileUrl": remoteProfileURL,
		"clashProfileUrl":  clashProfileURL,
		"singboxImportUrl": importURL,
	})
}

func (h *Handler) getSingboxProfile(c *gin.Context) {
	user, err := h.userService.GetByUUID(c.Param("uuid"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	nodes, err := h.nodeService.ListWithRuntimeStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	protocolSettings, err := h.adminService.GetProtocolSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if strings.EqualFold(c.Query("format"), "clash") {
		profile, err := subscription.GenerateClashProfile(*user, nodes, protocolSettings)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Data(http.StatusOK, "text/plain; charset=utf-8", profile)
		return
	}

	profile, err := subscription.GenerateSingboxProfile(*user, nodes, protocolSettings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "inline; filename=singbox-"+user.UUID+".json")
	c.Data(http.StatusOK, "application/json", profile)
}

// nodeBandwidthReport handles bandwidth usage reports from nodes
func (h *Handler) nodeBandwidthReport(c *gin.Context) {
	// Authenticate the node using tokens
	nodeToken := c.GetHeader("Authorization")
	controlToken := c.GetHeader("X-Control-Plane-Token")
	nodeName := c.GetHeader("X-Node-Name")

	if !strings.HasPrefix(nodeToken, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
		return
	}

	token := strings.TrimPrefix(nodeToken, "Bearer ")

	// Verify node token by looking up the node in database
	var node models.Node
	if err := h.nodeService.GetDB().Where("protocol_token = ? AND name = ?", token, nodeName).First(&node).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid node token or node name"})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Verify control plane token
	if controlToken != h.cfg.NodeSharedToken {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid control plane token"})
		return
	}

	// Parse the bandwidth report
	var report services.BandwidthReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Process the report
	if err := h.bandwidthReportService.ProcessReport(report, nodeName); err != nil {
		log.Printf("[bandwidth-report] processing error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process report: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "received",
		"nodeName": nodeName,
		"message":  "bandwidth report processed successfully",
	})
}

// triggerBandwidthCollection manually triggers bandwidth collection from all nodes
func (h *Handler) triggerBandwidthCollection(c *gin.Context) {
	if h.bandwidthCollector == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "bandwidth collector not initialized"})
		return
	}

	go func() {
		if err := h.bandwidthCollector.ForceCollect(); err != nil {
			log.Printf("[bandwidth-collector] manual collection error: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "collecting",
		"message": "bandwidth collection triggered successfully",
	})
}

// getBandwidthCollectorStatus returns the status of the bandwidth collector service
func (h *Handler) getBandwidthCollectorStatus(c *gin.Context) {
	if h.bandwidthCollector == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "bandwidth collector not initialized"})
		return
	}

	c.JSON(http.StatusOK, h.bandwidthCollector.GetStatus())
}

// triggerUserClassification manually triggers user classification for all users
func (h *Handler) triggerUserClassification(c *gin.Context) {
	if h.userClassificationService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user classification service not initialized"})
		return
	}

	classifications, err := h.userClassificationService.ForceClassify()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "completed",
		"classifiedUsers": len(classifications),
		"classifications": classifications,
	})
}

// getUserClassificationStats returns statistics about user classifications
func (h *Handler) getUserClassificationStats(c *gin.Context) {
	if h.userClassificationService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user classification service not initialized"})
		return
	}

	stats, err := h.userClassificationService.GetClassificationStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// getUserClassificationStatus returns the status of the user classification scheduler
func (h *Handler) getUserClassificationStatus(c *gin.Context) {
	if h.userClassificationScheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user classification scheduler not initialized"})
		return
	}

	status := h.userClassificationScheduler.GetStatus()
	serviceStatus := h.userClassificationService.GetStatus()

	// Merge the statuses
	for k, v := range serviceStatus {
		status[k] = v
	}

	c.JSON(http.StatusOK, status)
}

func timeDurationSeconds(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
