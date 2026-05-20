SHELL := /bin/zsh

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
DIST_DIR := $(ROOT_DIR)/dist
PANEL_BACKEND_DIR := $(ROOT_DIR)/panel_backend
NODE_BACKEND_DIR := $(ROOT_DIR)/node_backend
PROXY_DIR := $(ROOT_DIR)/proxy
FRONTEND_DIR := $(ROOT_DIR)/panel_frontend
PANEL_PORT := $(shell port=""; if [ -f "$(PANEL_BACKEND_DIR)/.env" ]; then port=$$(awk -F= '/^PORT=/{print $$2; exit}' "$(PANEL_BACKEND_DIR)/.env"); fi; echo $${port:-8080})
NODE_PORT := $(shell port=""; if [ -f "$(NODE_BACKEND_DIR)/.env" ]; then port=$$(awk -F= '/^PORT=/{print $$2; exit}' "$(NODE_BACKEND_DIR)/.env"); fi; echo $${port:-9090})
PROXY_PORT := $(shell port=""; if [ -f "$(PROXY_DIR)/.env" ]; then port=$$(awk -F= '/^PORT=/{print $$2; exit}' "$(PROXY_DIR)/.env"); fi; echo $${port:-9091})
FRONTEND_PORT := 5173

.PHONY: help panel-backend node-backend proxy-backend frontend stop-all install-panel install-node install-proxy install-frontend build-panel build-node build-proxy build-frontend release-panel release-panel-linux-amd64 release-panel-linux-arm64 release-node release-node-linux-amd64 release-node-linux-arm64 release-proxy release-proxy-linux-amd64 release-proxy-linux-arm64 release-frontend release-all clean-dist

help:
	@echo "Available targets:"
	@echo "  make panel-backend    Run the Go control plane"
	@echo "  make node-backend     Run the Go node agent"
	@echo "  make proxy-backend    Run the Go proxy server"
	@echo "  make frontend         Run the React frontend"
	@echo "  make stop-all         Stop all running services"
	@echo "  make install-panel    Install Go deps for panel_backend"
	@echo "  make install-node     Install Go deps for node_backend"
	@echo "  make install-proxy    Install Go deps for proxy"
	@echo "  make install-frontend Install npm deps for panel_frontend"
	@echo "  make build-panel      Build panel_backend"
	@echo "  make build-node       Build node_backend"
	@echo "  make build-frontend   Build panel_frontend"
	@echo "  make release-panel-linux-amd64 Build Linux AMD64 panel tarball"
	@echo "  make release-panel-linux-arm64 Build Linux ARM64 panel tarball"
	@echo "  make release-node     Build node_backend tarball"
	@echo "  make release-node-linux-amd64 Build Linux AMD64 node tarball"
	@echo "  make release-node-linux-arm64 Build Linux ARM64 node tarball"
	@echo "  make release-frontend Build panel_frontend tarball"
	@echo "  make release-all      Build tarballs for all apps"
	@echo "  make clean-dist       Remove generated release artifacts"

panel-backend:
	@cd $(PANEL_BACKEND_DIR) && go run cmd/server/main.go

node-backend:
	@cd $(NODE_BACKEND_DIR) && go run cmd/server/main.go

proxy-backend:
	@cd $(PROXY_DIR) && go run cmd/server/main.go

frontend:
	@cd $(FRONTEND_DIR) && npm run dev

stop-all:
	@for service in "panel-backend $(PANEL_PORT)" "node-backend $(NODE_PORT)" "proxy-backend $(PROXY_PORT)" "frontend $(FRONTEND_PORT)"; do \
		name=$${service% *}; \
		port=$${service##* }; \
		pids=$$(lsof -tiTCP:$$port -sTCP:LISTEN 2>/dev/null); \
		if [ -n "$$pids" ]; then \
			kill $$pids >/dev/null 2>&1 || true; \
			sleep 1; \
			remaining=$$(lsof -tiTCP:$$port -sTCP:LISTEN 2>/dev/null); \
			if [ -n "$$remaining" ]; then \
				kill -9 $$remaining >/dev/null 2>&1 || true; \
				sleep 1; \
				remaining=$$(lsof -tiTCP:$$port -sTCP:LISTEN 2>/dev/null); \
			fi; \
			if [ -n "$$remaining" ]; then \
				echo "Failed to stop $$name on port $$port (still running: $$remaining)"; \
			else \
				echo "Stopped $$name on port $$port"; \
			fi; \
		else \
			echo "$$name is not running on port $$port"; \
		fi; \
	done
	@pkill -f "go run cmd/server/main.go" >/dev/null 2>&1 || true
	@pkill -f "vite" >/dev/null 2>&1 || true

install-panel:
	@cd $(PANEL_BACKEND_DIR) && go mod tidy

install-node:
	@cd $(NODE_BACKEND_DIR) && go mod tidy

install-proxy:
	@cd $(PROXY_DIR) && go mod tidy

install-frontend:
	@cd $(FRONTEND_DIR) && npm install

build-panel:
	@cd $(PANEL_BACKEND_DIR) && go build ./...

build-node:
	@cd $(NODE_BACKEND_DIR) && go build ./...

build-proxy:
	@cd $(PROXY_DIR) && go build ./...

build-frontend:
	@cd $(FRONTEND_DIR) && npm run build

clean-dist:
	@rm -rf $(DIST_DIR)

release-panel:
	@mkdir -p $(DIST_DIR)/panel_backend
	@cd $(PANEL_BACKEND_DIR) && CGO_ENABLED=0 go build -o $(DIST_DIR)/panel_backend/panel_backend ./cmd/server
	@cp $(PANEL_BACKEND_DIR)/.env.example $(DIST_DIR)/panel_backend/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/panel_backend.tar.gz panel_backend
	@echo "Created $(DIST_DIR)/panel_backend.tar.gz"

release-panel-linux-amd64:
	@mkdir -p $(DIST_DIR)/panel_backend-linux-amd64
	@cd $(PANEL_BACKEND_DIR) && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(DIST_DIR)/panel_backend-linux-amd64/panel_backend ./cmd/server
	@cp $(PANEL_BACKEND_DIR)/.env.example $(DIST_DIR)/panel_backend-linux-amd64/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/panel_backend-linux-amd64.tar.gz panel_backend-linux-amd64
	@echo "Created $(DIST_DIR)/panel_backend-linux-amd64.tar.gz"

release-panel-linux-arm64:
	@mkdir -p $(DIST_DIR)/panel_backend-linux-arm64
	@cd $(PANEL_BACKEND_DIR) && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(DIST_DIR)/panel_backend-linux-arm64/panel_backend ./cmd/server
	@cp $(PANEL_BACKEND_DIR)/.env.example $(DIST_DIR)/panel_backend-linux-arm64/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/panel_backend-linux-arm64.tar.gz panel_backend-linux-arm64
	@echo "Created $(DIST_DIR)/panel_backend-linux-arm64.tar.gz"

release-node:
	@mkdir -p $(DIST_DIR)/node_backend
	@cd $(NODE_BACKEND_DIR) && CGO_ENABLED=0 go build -o $(DIST_DIR)/node_backend/node_backend ./cmd/server
	@cp $(NODE_BACKEND_DIR)/.env.example $(DIST_DIR)/node_backend/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/node_backend.tar.gz node_backend
	@echo "Created $(DIST_DIR)/node_backend.tar.gz"

release-node-linux-amd64:
	@mkdir -p $(DIST_DIR)/node_backend-linux-amd64
	@cd $(NODE_BACKEND_DIR) && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(DIST_DIR)/node_backend-linux-amd64/node_backend ./cmd/server
	@cp $(NODE_BACKEND_DIR)/.env.example $(DIST_DIR)/node_backend-linux-amd64/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/node_backend-linux-amd64.tar.gz node_backend-linux-amd64
	@echo "Created $(DIST_DIR)/node_backend-linux-amd64.tar.gz"

release-node-linux-arm64:
	@mkdir -p $(DIST_DIR)/node_backend-linux-arm64
	@cd $(NODE_BACKEND_DIR) && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(DIST_DIR)/node_backend-linux-arm64/node_backend ./cmd/server
	@cp $(NODE_BACKEND_DIR)/.env.example $(DIST_DIR)/node_backend-linux-arm64/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/node_backend-linux-arm64.tar.gz node_backend-linux-arm64
	@echo "Created $(DIST_DIR)/node_backend-linux-arm64.tar.gz"

release-frontend:
	@mkdir -p $(DIST_DIR)/panel_frontend
	@cd $(FRONTEND_DIR) && npm run build
	@cp -R $(FRONTEND_DIR)/dist/. $(DIST_DIR)/panel_frontend/
	@cp $(FRONTEND_DIR)/.env.example $(DIST_DIR)/panel_frontend/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/panel_frontend.tar.gz panel_frontend
	@echo "Created $(DIST_DIR)/panel_frontend.tar.gz"

release-proxy:
	@mkdir -p $(DIST_DIR)/proxy
	@cd $(PROXY_DIR) && CGO_ENABLED=0 go build -o $(DIST_DIR)/proxy/proxy ./cmd/server
	@cp $(PROXY_DIR)/.env.example $(DIST_DIR)/proxy/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/proxy.tar.gz proxy
	@echo "Created $(DIST_DIR)/proxy.tar.gz"

release-proxy-linux-amd64:
	@mkdir -p $(DIST_DIR)/proxy-linux-amd64
	@cd $(PROXY_DIR) && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(DIST_DIR)/proxy-linux-amd64/proxy ./cmd/server
	@cp $(PROXY_DIR)/.env.example $(DIST_DIR)/proxy-linux-amd64/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/proxy-linux-amd64.tar.gz proxy-linux-amd64
	@echo "Created $(DIST_DIR)/proxy-linux-amd64.tar.gz"

release-proxy-linux-arm64:
	@mkdir -p $(DIST_DIR)/proxy-linux-arm64
	@cd $(PROXY_DIR) && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(DIST_DIR)/proxy-linux-arm64/proxy ./cmd/server
	@cp $(PROXY_DIR)/.env.example $(DIST_DIR)/proxy-linux-arm64/.env.example
	@tar -C $(DIST_DIR) -czf $(DIST_DIR)/proxy-linux-arm64.tar.gz proxy-linux-arm64
	@echo "Created $(DIST_DIR)/proxy-linux-arm64.tar.gz"

release-all: release-panel-linux-amd64 release-panel-linux-arm64 release-node-linux-amd64 release-node-linux-arm64 release-proxy-linux-amd64 release-proxy-linux-arm64 release-frontend
