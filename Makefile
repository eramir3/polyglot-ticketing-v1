.DEFAULT_GOAL := help

.PHONY: help install generate-proto build build-api-gateway build-identity build-tickets test test-api-gateway test-tickets serve-api-gateway serve-identity serve-tickets docker-build docker-up docker-down docker-logs docker-ps

help: ## Show available commands.
	@awk 'BEGIN {FS = ":.*##"}; /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install workspace dependencies.
	pnpm install --frozen-lockfile

generate-proto: ## Generate TypeScript and Go protobuf bindings.
	pnpm proto:generate

build: build-api-gateway build-identity build-tickets ## Build every service.

build-api-gateway: ## Build the API gateway.
	pnpm nx build api-gateway

build-identity: ## Build the identity service.
	pnpm nx build identity

build-tickets: ## Build the tickets service.
	pnpm nx build tickets

test: test-api-gateway test-tickets ## Run all executable tests.

test-api-gateway: ## Run API gateway integration tests (requires Docker for Testcontainers).
	pnpm nx run api-gateway:integration

test-tickets: ## Run tickets Go tests.
	pnpm nx test tickets

serve-api-gateway: ## Run the API gateway locally.
	pnpm nx serve api-gateway

serve-identity: ## Run the identity gRPC service locally.
	pnpm nx serve identity

serve-tickets: ## Run the tickets gRPC service locally.
	pnpm nx serve tickets

docker-build: ## Build all Docker Compose service images.
	docker compose build

docker-up: ## Start the local Docker Compose stack and rebuild images.
	docker compose up -d --build

docker-down: ## Stop and remove the local Docker Compose stack.
	docker compose down

docker-logs: ## Follow logs for the local Docker Compose stack.
	docker compose logs -f

docker-ps: ## Show local Docker Compose service status.
	docker compose ps
