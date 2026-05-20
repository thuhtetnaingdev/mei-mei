package main

import (
	"log"
	"net/http"

	"proxy/internal/api"
	"proxy/internal/config"
)

func main() {
	cfg := config.Load()

	handler := api.New(cfg.ProxyURL, cfg.MapToken)

	http.Handle("/sub/", handler)
	http.Handle("/sub", handler)

	addr := ":" + cfg.Port
	log.Printf("Proxy server starting on %s", addr)
	log.Printf("Upstream: %s", cfg.ProxyURL)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
