package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"panel_backend/internal/api"
	"panel_backend/internal/config"
	"panel_backend/internal/db"
	"panel_backend/internal/services"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "reset-accounting" {
		runResetAccounting(os.Args[2:])
		return
	}

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

func runResetAccounting(args []string) {
	resetFlags := flag.NewFlagSet("reset-accounting", flag.ExitOnError)
	assumeYes := resetFlags.Bool("yes", false, "confirm destructive accounting reset")
	databasePath := resetFlags.String("database-path", "", "override database path")
	resetFlags.Parse(args)

	if !*assumeYes {
		log.Fatal("refusing to reset accounting without --yes")
	}

	_ = godotenv.Load()
	if *databasePath == "" {
		*databasePath = os.Getenv("DATABASE_PATH")
	}
	if *databasePath == "" {
		*databasePath = "./panel.sqlite3"
	}

	if err := os.MkdirAll(filepath.Dir(*databasePath), 0o755); err != nil && filepath.Dir(*databasePath) != "." {
		log.Fatalf("failed to create database directory: %v", err)
	}

	database, err := db.Connect(*databasePath)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	summary, err := services.ResetAccounting(database)
	if err != nil {
		log.Fatalf("failed to reset accounting: %v", err)
	}

	fmt.Printf("accounting reset complete\n")
	fmt.Printf("users reset: %d\n", summary.UsersReset)
	fmt.Printf("nodes reset: %d\n", summary.NodesReset)
	fmt.Printf("miners reset: %d\n", summary.MinersReset)
	fmt.Printf("allocations reset: %d\n", summary.AllocationsReset)
	fmt.Printf("node usage entries deleted: %d\n", summary.NodeUsageEntriesDeleted)
	fmt.Printf("miner rewards deleted: %d\n", summary.MinerRewardsDeleted)
	fmt.Printf("mint events deleted: %d\n", summary.MintEventsDeleted)
	fmt.Printf("mint transfers deleted: %d\n", summary.MintTransfersDeleted)
}
