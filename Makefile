.PHONY: proto-gen build build-linux docker-build docker-up docker-down test test-handler test-integration loadtest infra-up infra-down migrate help

SERVICES = analytics-service auth-service user-service venue-service booking-service review-service payment-service master-service crm-service notification-service api-gateway

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

proto-gen: ## Generate Go code from proto files
	buf generate

build: ## Build all services (native)
	@mkdir -p bin
	@for svc in $(SERVICES); do \
		echo "Building $$svc..."; \
		(cd services/$$svc && CGO_ENABLED=0 go build -o ../../bin/$$svc ./cmd/) || exit $$?; \
	done

build-linux: ## Cross-compile all services for Linux (Docker)
	@mkdir -p bin/linux
	@for svc in $(SERVICES); do \
		echo "Building $$svc for linux..."; \
		(cd services/$$svc && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/linux/$$svc ./cmd/) || exit $$?; \
	done
	@echo "All services built in bin/linux/"

docker-build: build-linux ## Build Docker images (fast: local compile + copy)
	docker compose -f deploy/docker-compose.yml build

docker-up: docker-build ## Build and start everything
	docker compose -f deploy/docker-compose.yml up -d

docker-down: ## Stop all containers
	docker compose -f deploy/docker-compose.yml down

test: ## Run tests for all services
	@for svc in $(SERVICES); do \
		echo "Testing $$svc..."; \
		(cd services/$$svc && go test ./...) || exit $$?; \
	done

# golang:latest может быть старше 1.25; GOTOOLCHAIN=auto позволяет Go
# скачать нужную версию самому (требует выход в сеть из контейнера).
# Переопределить образ: make test-handler GO_DOCKER_IMAGE=golang:1.25rc2
GO_DOCKER_IMAGE ?= golang:latest

test-handler: ## Run api-gateway handler tests inside Docker (no local Go required)
	docker run --rm \
		-v "$(shell pwd)":/workspace \
		-w /workspace \
		-e GOTOOLCHAIN=auto \
		-e GOFLAGS=-mod=mod \
		$(GO_DOCKER_IMAGE) \
		go test -v -count=1 ./services/api-gateway/internal/handler/...

# Repository-layer DB integration tests. Behind the `integration` build tag so
# the default `make test` (plain `go test ./...`) skips them — they need a live
# Docker daemon to spin up throwaway Postgres containers (testcontainers-go).
test-integration: ## Run DB integration tests (requires Docker)
	cd services/payment-service && go test -tags=integration -race -count=1 ./internal/repository/...
	cd services/venue-service && go test -tags=integration -race -count=1 ./internal/repository/...

# k6-нагрузка на публичный каталог api-gateway. Требует установленного k6.
# Настройка: make loadtest PROFILE=stress BASE_URL=http://localhost:8080
LOADTEST_BASE_URL ?= http://localhost:8080
PROFILE ?= load
loadtest: ## Run k6 load test against public read endpoints (PROFILE=smoke|load|stress|spike)
	BASE_URL=$(LOADTEST_BASE_URL) PROFILE=$(PROFILE) k6 run deploy/loadtest/public-read.js

infra-up: ## Start infrastructure (PG, Redis, NATS, MinIO)
	docker compose -f deploy/docker-compose.infra.yml up -d

infra-down: ## Stop infrastructure
	docker compose -f deploy/docker-compose.infra.yml down

infra-reset: ## Reset infrastructure (destroy volumes)
	docker compose -f deploy/docker-compose.infra.yml down -v

migrate: ## Run DB migrations via docker exec
	bash deploy/migrate.sh

tidy: ## Run go mod tidy for all modules
	cd pkg && go mod tidy
	@for svc in $(SERVICES); do \
		echo "Tidying $$svc..."; \
		(cd services/$$svc && go mod tidy) || exit $$?; \
	done

lint: ## Run golangci-lint on all services
	@for svc in $(SERVICES); do \
		echo "Linting $$svc..."; \
		(cd services/$$svc && golangci-lint run ./...) || exit $$?; \
	done

run-auth: ## Run auth-service locally
	cd services/auth-service && JWT_SECRET=dev-secret-key go run ./cmd/

run-user: ## Run user-service locally
	cd services/user-service && go run ./cmd/

run-venue: ## Run venue-service locally
	cd services/venue-service && go run ./cmd/

run-booking: ## Run booking-service locally
	cd services/booking-service && go run ./cmd/

run-review: ## Run review-service locally
	cd services/review-service && go run ./cmd/

run-payment: ## Run payment-service locally
	cd services/payment-service && go run ./cmd/

run-master: ## Run master-service locally
	cd services/master-service && go run ./cmd/

run-crm: ## Run crm-service locally
	cd services/crm-service && go run ./cmd/

run-notification: ## Run notification-service locally
	cd services/notification-service && go run ./cmd/

run-gateway: ## Run api-gateway locally
	cd services/api-gateway && go run ./cmd/

run-frontend: ## Run Next.js frontend
	cd frontend && npm run dev
