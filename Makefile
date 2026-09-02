.DEFAULT_GOAL := help

.PHONY: help install generate-proto build build-api-gateway build-identity build-tickets build-orders build-payments build-expiration test test-api-gateway test-tickets test-orders test-payments test-expiration serve-api-gateway serve-identity serve-tickets serve-orders serve-payments serve-expiration stress-tickets k6-tickets-list replay-orders-dlq replay-tickets-dlq replay-payments-dlq replay-expiration-dlq docker-build docker-up docker-up-tools docker-down docker-reset docker-logs docker-ps

K6_TEST_ID ?= tickets-list-$(shell date -u +%Y%m%dT%H%M%SZ)

help: ## Show available commands.
	@awk 'BEGIN {FS = ":.*##"}; /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install workspace dependencies.
	pnpm install --frozen-lockfile

generate-proto: ## Generate TypeScript and Go protobuf bindings.
	pnpm proto:generate

build: build-api-gateway build-identity build-tickets build-orders build-payments build-expiration ## Build every service.

build-api-gateway: ## Build the API gateway.
	pnpm nx build api-gateway

build-identity: ## Build the identity service.
	pnpm nx build identity

build-tickets: ## Build the tickets service.
	pnpm nx build tickets

build-orders: ## Build the orders service.
	pnpm nx build orders

build-payments: ## Build the payments service.
	pnpm nx build payments

build-expiration: ## Build the expiration service.
	pnpm nx build expiration

test: test-api-gateway test-tickets test-orders test-payments test-expiration ## Run all executable tests.

test-api-gateway: ## Run API gateway integration tests (requires Docker for Testcontainers).
	pnpm nx run api-gateway:integration

test-tickets: ## Run tickets Go tests.
	pnpm nx test tickets

test-orders: ## Run Orders Go tests (requires Docker for PostgreSQL Testcontainers).
	pnpm nx test orders

test-payments: ## Run Payments Go tests (requires Docker for PostgreSQL Testcontainers).
	pnpm nx test payments

test-expiration: ## Run expiration service tests.
	pnpm nx test expiration

serve-api-gateway: ## Run the API gateway locally.
	pnpm nx serve api-gateway

serve-identity: ## Run the identity gRPC service locally.
	pnpm nx serve identity

serve-tickets: ## Run the tickets gRPC service locally.
	pnpm nx serve tickets

serve-orders: ## Run the orders ticket-projection service locally.
	pnpm nx serve orders

serve-payments: ## Run the payments service locally.
	pnpm nx serve payments

serve-expiration: ## Run the expiration service locally.
	pnpm nx serve expiration

stress-tickets: ## Run sequential ticket create/update stress cycles (requires STRESS_COOKIE).
	node scripts/stress/tickets.js

k6-tickets-list: ## Run the k6 GET /api/tickets smoke and small-ramp test (requires docker-up-tools).
	@test -n "$${LOAD_TEST_METRICS_TOKEN:-$$(sed -n 's/^LOAD_TEST_METRICS_TOKEN=//p' .env 2>/dev/null | tail -n 1)}" || (echo "LOAD_TEST_METRICS_TOKEN is required in .env or the shell environment"; exit 1)
	docker compose --profile performance --profile tools run --rm -e K6_TEST_ID=$(K6_TEST_ID) k6 run -o experimental-prometheus-rw --tag source=k6 --tag test_type=tickets-list --tag environment=local --tag testid=$(K6_TEST_ID) /scripts/tickets-list.js

replay-orders-dlq: ## Replay one Orders DLQ message (requires DLQ_SEQUENCE).
	@test -n "$(DLQ_SEQUENCE)" || (echo "DLQ_SEQUENCE is required"; exit 1)
	DLQ_SEQUENCE=$(DLQ_SEQUENCE) go run ./apps/orders/cmd/orders-ticket-dlq-replay

replay-tickets-dlq: ## Replay one Tickets DLQ message (requires DLQ_SEQUENCE).
	@test -n "$(DLQ_SEQUENCE)" || (echo "DLQ_SEQUENCE is required"; exit 1)
	DLQ_SEQUENCE=$(DLQ_SEQUENCE) go run ./apps/tickets/cmd/tickets-order-dlq-replay

replay-payments-dlq: ## Replay one Payments DLQ message (requires DLQ_SEQUENCE).
	@test -n "$(DLQ_SEQUENCE)" || (echo "DLQ_SEQUENCE is required"; exit 1)
	DLQ_SEQUENCE=$(DLQ_SEQUENCE) go run ./apps/payments/cmd/payments-order-dlq-replay

replay-expiration-dlq: ## Replay one Expiration DLQ message (requires DLQ_SEQUENCE).
	@test -n "$(DLQ_SEQUENCE)" || (echo "DLQ_SEQUENCE is required"; exit 1)
	pnpm nx build expiration
	DLQ_SEQUENCE=$(DLQ_SEQUENCE) node dist/apps/expiration/apps/expiration/src/expiration/replay-expiration-dlq.js

docker-build: ## Build all Docker Compose service images.
	docker compose build

docker-up: ## Start the local Docker Compose stack and rebuild images.
	docker compose up -d --build

docker-up-tools: ## Start optional local development tools, including NUI, Redis Insight, Grafana, Loki, Prometheus, and Tempo.
	docker compose --profile tools up -d --build nui redisinsight loki prometheus tempo grafana alloy

docker-down: ## Stop and remove the local Docker Compose stack.
	docker compose down

docker-reset: ## Delete all application and optional-tool Compose data, then rebuild a fresh stack.
	docker compose --profile tools down --volumes --remove-orphans
	docker compose up -d --build

docker-logs: ## Follow logs for the local Docker Compose stack.
	docker compose logs -f

docker-ps: ## Show local Docker Compose service status.
	docker compose ps
