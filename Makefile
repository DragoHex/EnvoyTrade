DATABASE_URL ?= postgres://envoytrade:envoytrade@localhost:5432/envoytrade?sslmode=disable
PORT ?= 8080
FRONTEND_PORT ?= 5173
DB_CONTAINER ?= envoytrade-db
DB_VOLUME ?= envoytrade-db-data
BACKEND_LOG ?= /tmp/envoytrade-backend.log
FRONTEND_LOG ?= /tmp/envoytrade-frontend.log
CLOUDFLARED_LOG ?= /tmp/cloudflared.log

.PHONY: build build-backend build-frontend run-backend run-frontend test test-integration install \
	db-up db-down db-seed db-flush seed flush \
	backend-up backend-down frontend-up frontend-down tunnel-up tunnel-down \
	up up-all all-up up-everything down down-all all-down down-everything \
	check-log-dir setup-log-dir sqlc-generate

build: build-backend build-frontend

# Data lives in the named volume $(DB_VOLUME), not the container, so
# `db-down` (which removes the container) doesn't lose it. `db-up` reuses
# an existing (stopped) container if present instead of erroring on
# `--name` conflict.
db-up:
	podman start $(DB_CONTAINER) 2>/dev/null || podman run -d --name $(DB_CONTAINER) \
		-e POSTGRES_DB=envoytrade -e POSTGRES_USER=envoytrade -e POSTGRES_PASSWORD=envoytrade \
		-p 5432:5432 -v $(DB_VOLUME):/var/lib/postgresql/data postgres:16-alpine
	@echo "Waiting for PostgreSQL to be ready..."
	@for i in $$(seq 1 30); do \
		if podman exec $(DB_CONTAINER) pg_isready -U envoytrade -d envoytrade >/dev/null 2>&1; then \
			echo "PostgreSQL is ready."; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "Timeout waiting for PostgreSQL."; exit 1

db-down:
	podman rm -f $(DB_CONTAINER)

db-seed:
	DATABASE_URL="$(DATABASE_URL)" CONTAINER_NAME="$(DB_CONTAINER)" ./scripts/seed_test_data.sh

db-flush:
	DATABASE_URL="$(DATABASE_URL)" CONTAINER_NAME="$(DB_CONTAINER)" ./scripts/flush_data.sh

seed: db-seed
flush: db-flush
sync-instruments:
	DB_CONTAINER="$(DB_CONTAINER)" python3 ./scripts/sync_instruments.py


# ============================================================================
# LOGGING CONFIGURATION:
# By default, the backend writes structured logs to /var/log/envoytrade/app.log.
# On deployment / production, ensure the directory exists and is writable:
#   sudo mkdir -p /var/log/envoytrade && sudo chown -R $(whoami) /var/log/envoytrade
# Or run:
#   make setup-log-dir
# In development, you can override this by setting LOG_FILE:
#   export LOG_FILE="./app.log"
# ============================================================================
check-log-dir:
	@if [ -z "$$LOG_FILE" ]; then \
		if [ ! -d /var/log/envoytrade ] || [ ! -w /var/log/envoytrade ]; then \
			echo "================================================================================"; \
			echo " NOTICE: /var/log/envoytrade is not available or writable."; \
			echo " For persistent deployment logging, create the directory:"; \
			echo "   make setup-log-dir"; \
			echo " or manually:"; \
			echo "   sudo mkdir -p /var/log/envoytrade && sudo chown -R $$(whoami) /var/log/envoytrade"; \
			echo "================================================================================"; \
		fi; \
	fi

setup-log-dir:
	@echo "Creating /var/log/envoytrade..."
	sudo mkdir -p /var/log/envoytrade && sudo chown -R $$(whoami) /var/log/envoytrade
	@echo "Directory /var/log/envoytrade is ready."

# backend-up starts the backend server in the background and waits until it is listening
backend-up: db-up check-log-dir
	@if lsof -ti :$(PORT) >/dev/null 2>&1; then \
		echo "Backend is already running on port $(PORT) (PID: $$(lsof -ti :$(PORT) | tr '\n' ' '))"; \
	else \
		echo "Starting backend server on port $(PORT)..."; \
		DATABASE_URL="$(DATABASE_URL)" PORT="$(PORT)" nohup go run ./cmd/server > $(BACKEND_LOG) 2>&1 & \
		for i in $$(seq 1 30); do \
			if lsof -ti :$(PORT) >/dev/null 2>&1; then \
				echo "Backend started (PID: $$(lsof -ti :$(PORT) | tr '\n' ' '))"; \
				echo "Backend logs: $(BACKEND_LOG)"; \
				exit 0; \
			fi; \
			sleep 1; \
		done; \
		echo "Failed to start backend server. Check $(BACKEND_LOG):"; \
		tail -n 20 $(BACKEND_LOG); \
		exit 1; \
	fi

# backend-down stops the backend server
backend-down:
	@if lsof -ti :$(PORT) >/dev/null 2>&1; then \
		echo "Stopping backend server on port $(PORT)..."; \
		lsof -ti :$(PORT) | xargs -r kill; \
		echo "Backend server stopped."; \
	else \
		echo "Backend server is not running."; \
	fi

# frontend-up starts the frontend dev server in the background and waits until it is listening
frontend-up:
	@if lsof -ti :$(FRONTEND_PORT) >/dev/null 2>&1; then \
		echo "Frontend is already running on port $(FRONTEND_PORT) (PID: $$(lsof -ti :$(FRONTEND_PORT) | tr '\n' ' '))"; \
	else \
		echo "Starting frontend dev server on port $(FRONTEND_PORT)..."; \
		if [ ! -f frontend/node_modules/.bin/vite ]; then cd frontend && pnpm install; fi; \
		python3 -c 'import subprocess; subprocess.Popen(["./node_modules/.bin/vite"], cwd="frontend", start_new_session=True, stdin=subprocess.DEVNULL, stdout=open("$(FRONTEND_LOG)", "w"), stderr=subprocess.STDOUT)'; \
		for i in $$(seq 1 30); do \
			if lsof -ti :$(FRONTEND_PORT) >/dev/null 2>&1; then \
				echo "Frontend started (PID: $$(lsof -ti :$(FRONTEND_PORT) | tr '\n' ' '))"; \
				echo "Frontend logs: $(FRONTEND_LOG)"; \
				exit 0; \
			fi; \
			sleep 1; \
		done; \
		echo "Failed to start frontend dev server. Check $(FRONTEND_LOG):"; \
		tail -n 20 $(FRONTEND_LOG); \
		exit 1; \
	fi

# frontend-down stops the frontend dev server
frontend-down:
	@if lsof -ti :$(FRONTEND_PORT) >/dev/null 2>&1; then \
		echo "Stopping frontend dev server on port $(FRONTEND_PORT)..."; \
		lsof -ti :$(FRONTEND_PORT) | xargs -r kill; \
		echo "Frontend dev server stopped."; \
	else \
		echo "Frontend dev server is not running."; \
	fi

# tunnel-up starts the cloudflared tunnel in the background
tunnel-up:
	@if pgrep -f "cloudflared tunnel.*run" >/dev/null 2>&1; then \
		echo "Cloudflared tunnel is already running (PID: $$(pgrep -f "cloudflared tunnel.*run" | tr '\n' ' '))"; \
	else \
		echo "Starting cloudflared tunnel..."; \
		nohup cloudflared tunnel run > $(CLOUDFLARED_LOG) 2>&1 & \
		for i in $$(seq 1 15); do \
			if pgrep -f "cloudflared tunnel.*run" >/dev/null 2>&1; then \
				echo "Cloudflared tunnel started (PID: $$(pgrep -f "cloudflared tunnel.*run" | tr '\n' ' '))"; \
				echo "Tunnel logs: $(CLOUDFLARED_LOG)"; \
				exit 0; \
			fi; \
			sleep 1; \
		done; \
		echo "Failed to start cloudflared tunnel. Check $(CLOUDFLARED_LOG)"; \
		cat $(CLOUDFLARED_LOG); \
		exit 1; \
	fi

# tunnel-down stops the cloudflared tunnel process
tunnel-down:
	@if pgrep -f "cloudflared tunnel.*run" >/dev/null 2>&1; then \
		echo "Stopping cloudflared tunnel..."; \
		pkill -INT -f "cloudflared tunnel.*run" 2>/dev/null || true; \
		sleep 1; \
		if pgrep -f "cloudflared tunnel.*run" >/dev/null 2>&1; then \
			pkill -9 -f "cloudflared tunnel.*run" 2>/dev/null || true; \
		fi; \
		echo "Cloudflared tunnel stopped."; \
	else \
		echo "Cloudflared tunnel is not running."; \
	fi

# up-all brings up all services in order: db -> backend -> frontend -> tunnel
up-all: db-up backend-up frontend-up tunnel-up
	@echo ""
	@echo "All services are up and running!"
	@echo "  Database: $(DATABASE_URL)"
	@echo "  Backend:  http://localhost:$(PORT)"
	@echo "  Frontend: http://localhost:$(FRONTEND_PORT)"
	@echo "  Tunnel:   https://envoytrade.in (and https://app.envoytrade.in)"

up: up-all
all-up: up-all
up-everything: up-all

# down-all brings down everything in reverse order: tunnel -> frontend -> backend -> db
down-all: tunnel-down frontend-down backend-down db-down
	@echo "All services stopped."

down: backend-down frontend-down db-down
all-down: down-all
down-everything: down-all

build-backend:
	go build ./...

build-frontend:
	cd frontend && pnpm build

run-backend: check-log-dir
	DATABASE_URL="$(DATABASE_URL)" PORT="$(PORT)" go run ./cmd/server

run-frontend:
	cd frontend && pnpm dev

install:
	cd frontend && pnpm install

test:
	go test ./...
	cd frontend && pnpm test

# Integration tests (internal/store/postgres, internal/engine, ...) spin up
# their own throwaway Postgres via testcontainers-go — this only points it
# at podman's machine socket instead of Docker.
test-integration:
	DOCKER_HOST="$$(podman system connection list --format '{{.URI}}' 2>/dev/null | head -n 1 || echo "unix://$$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null)")" \
	TESTCONTAINERS_RYUK_DISABLED=true \
	go test -tags integration ./...

sqlc-generate:
	sqlc generate
