package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"panel_backend/internal/api"
	"panel_backend/internal/config"
	"panel_backend/internal/db"
	"panel_backend/internal/services"
	"strconv"
	"syscall"
	"time"
)

func main() {
	cfg := config.Load()

	database, err := db.Connect(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	userService := services.NewUserService(database)
	nodeService := services.NewNodeService(database, cfg.NodeSharedToken, time.Duration(cfg.SyncTimeoutSeconds)*time.Second, userService)

	// Start bandwidth collector service
	collector := services.NewBandwidthCollectorService(services.BandwidthCollectorConfig{
		DB:              database,
		NodeSharedToken: cfg.NodeSharedToken,
		CollectInterval: 60 * time.Second,
		RequestTimeout:  30 * time.Second,
		UserService:     userService,
		NodeService:     nodeService,
	})

	// Start user classification scheduler
	userClassificationService := services.NewUserClassificationService(database)
	userClassificationScheduler := services.NewUserClassificationScheduler(userClassificationService, 24*time.Hour)

	// Start REALITY key verification scheduler
	realityKeyVerificationService := services.NewRealityKeyVerificationService(
		database,
		nodeService,
		cfg.NodeSharedToken,
		30*time.Second,
	)

	// Get interval from env (default: 6 hours)
	verificationIntervalHours := 6
	if val := os.Getenv("REALITY_KEY_VERIFICATION_INTERVAL_HOURS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			verificationIntervalHours = parsed
		}
	}

	// Get auto-fix setting from env (default: true)
	autoFixEnabled := true
	if val := os.Getenv("REALITY_KEY_AUTO_FIX_ENABLED"); val != "" {
		autoFixEnabled = val == "true" || val == "1"
	}

	realityKeyScheduler := services.NewRealityKeyVerificationScheduler(
		realityKeyVerificationService,
		verificationIntervalHours,
		autoFixEnabled,
	)

	router := api.NewRouterWithServices(cfg, database, userService, nodeService, collector, userClassificationService, userClassificationScheduler, realityKeyVerificationService, realityKeyScheduler)

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	collector.Start(ctx)
	userClassificationScheduler.Start(ctx)
	realityKeyScheduler.Start(ctx)

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()

		log.Println("shutting down...")
		collector.Stop()
		userClassificationScheduler.Stop()
		realityKeyScheduler.Stop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http shutdown failed: %v", err)
		}

		sqlDB, err := database.DB()
		if err != nil {
			log.Printf("database shutdown failed: %v", err)
			return
		}
		if err := sqlDB.Close(); err != nil {
			log.Printf("database close failed: %v", err)
		}
	}()

	log.Printf("panel_backend listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}

	<-shutdownDone
}
