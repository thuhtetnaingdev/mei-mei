package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"node_backend/internal/api"
	"node_backend/internal/config"
	"node_backend/internal/services"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create config service (which includes bandwidth tracker)
	configService := services.NewConfigService(cfg)
	configService.StartBandwidthMonitoring(ctx, 10*time.Second)

	// Create router with config service
	router := api.NewRouterWithConfigService(cfg, configService)

	// Create HTTP server
	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("shutting down...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	log.Printf("node_backend listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}

	log.Println("node_backend stopped")
}
