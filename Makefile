.PHONY: proto-gen build build-linux docker-build docker-up docker-down test infra-up infra-down migrate help

SERVICES = auth-service user-service venue-service booking-service review-service payment-service api-gateway

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

proto-gen: ## Generate Go code from proto files
	buf generate

build: ## Build all services (native)
	@mkdir -p bin
	@for svc in $(SERVICES); do \
		echo "Building $$svc..."; \
		cd services/$$svc && CGO_ENABLED=0 go build -o ../../bin/$$svc ./cmd/ && cd ../..; \
	done

build-linux: ## Cross-compile all services for Linux (Docker)
	@mkdir -p bin/linux
	@for svc in $(SERVICES); do \
		echo "Building $$svc for linux..."; \
		cd services/$$svc && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/linux/$$svc ./cmd/ && cd ../..; \
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
		cd services/$$svc && go test ./... && cd ../..; \
	done

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
		cd services/$$svc && go mod tidy && cd ../..; \
	done

lint: ## Run golangci-lint on all services
	@for svc in $(SERVICES); do \
		echo "Linting $$svc..."; \
		cd services/$$svc && golangci-lint run ./... && cd ../..; \
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

run-gateway: ## Run api-gateway locally
	cd services/api-gateway && go run ./cmd/

run-frontend: ## Run Next.js frontend
	cd frontend && npm run dev
