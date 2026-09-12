DATABASE_URL ?= postgres://envoytrade:envoytrade@localhost:5432/envoytrade?sslmode=disable
PORT ?= 8080
FRONTEND_PORT ?= 5173
DB_CONTAINER ?= envoytrade-db
DB_VOLUME ?= envoytrade-db-data

.PHONY: build build-backend build-frontend run-backend run-frontend test test-integration install db-up db-down db-seed db-flush seed flush down sqlc-generate

build: build-backend build-frontend

# Data lives in the named volume $(DB_VOLUME), not the container, so
# `db-down` (which removes the container) doesn't lose it. `db-up` reuses
# an existing (stopped) container if present instead of erroring on
# `--name` conflict.
db-up:
	podman start $(DB_CONTAINER) 2>/dev/null || podman run -d --name $(DB_CONTAINER) \
		-e POSTGRES_DB=envoytrade -e POSTGRES_USER=envoytrade -e POSTGRES_PASSWORD=envoytrade \
		-p 5432:5432 -v $(DB_VOLUME):/var/lib/postgresql/data postgres:16-alpine

db-down:
	podman rm -f $(DB_CONTAINER)

db-seed:
	DATABASE_URL="$(DATABASE_URL)" CONTAINER_NAME="$(DB_CONTAINER)" ./scripts/seed_test_data.sh

db-flush:
	DATABASE_URL="$(DATABASE_URL)" CONTAINER_NAME="$(DB_CONTAINER)" ./scripts/flush_data.sh

seed: db-seed
flush: db-flush

# down stops the backend/frontend dev servers (by port) and the db
# container. run-backend/run-frontend run in the foreground, so this is
# for killing them if left running in the background or another shell.
down: db-down
	-lsof -ti :$(PORT) | xargs -r kill
	-lsof -ti :$(FRONTEND_PORT) | xargs -r kill

build-backend:
	go build ./...

build-frontend:
	cd frontend && pnpm build

run-backend:
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
	DOCKER_HOST="unix://$$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')" \
	TESTCONTAINERS_RYUK_DISABLED=true \
	go test -tags integration ./...

sqlc-generate:
	sqlc generate
