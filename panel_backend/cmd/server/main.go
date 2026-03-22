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

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	collector.Start(ctx)

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()

		log.Println("shutting down...")
		collector.Stop()

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
