package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"panel_backend/internal/api"
	"panel_backend/internal/config"
	"panel_backend/internal/db"
	"panel_backend/internal/services"
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
		CollectInterval: 10 * time.Second,
		RequestTimeout:  30 * time.Second,
		UserService:     userService,
		NodeService:     nodeService,
	})

	router := api.NewRouterWithServices(cfg, database, userService, nodeService, collector)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector.Start(ctx)

	// Handle graceful shutdown
	go func() {
		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("shutting down...")
		cancel()
		collector.Stop()
	}()

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("panel_backend listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
