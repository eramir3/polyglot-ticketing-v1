.DEFAULT_GOAL := help

.PHONY: help install generate-proto build build-api-gateway build-identity build-tickets build-orders test test-api-gateway test-tickets test-orders serve-api-gateway serve-identity serve-tickets serve-orders docker-build docker-up docker-up-tools docker-down docker-logs docker-ps

help: ## Show available commands.
	@awk 'BEGIN {FS = ":.*##"}; /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install workspace dependencies.
	pnpm install --frozen-lockfile

generate-proto: ## Generate TypeScript and Go protobuf bindings.
	pnpm proto:generate

build: build-api-gateway build-identity build-tickets build-orders ## Build every service.

build-api-gateway: ## Build the API gateway.
	pnpm nx build api-gateway

build-identity: ## Build the identity service.
	pnpm nx build identity

build-tickets: ## Build the tickets service.
	pnpm nx build tickets

build-orders: ## Build the orders service.
	pnpm nx build orders

test: test-api-gateway test-tickets test-orders ## Run all executable tests.

test-api-gateway: ## Run API gateway integration tests (requires Docker for Testcontainers).
	pnpm nx run api-gateway:integration

test-tickets: ## Run tickets Go tests.
	pnpm nx test tickets

test-orders: ## Run orders Go tests.
	pnpm nx test orders

serve-api-gateway: ## Run the API gateway locally.
	pnpm nx serve api-gateway

serve-identity: ## Run the identity gRPC service locally.
	pnpm nx serve identity

serve-tickets: ## Run the tickets gRPC service locally.
	pnpm nx serve tickets

serve-orders: ## Run the orders ticket-projection service locally.
	pnpm nx serve orders

docker-build: ## Build all Docker Compose service images.
	docker compose build

docker-up: ## Start the local Docker Compose stack and rebuild images.
	docker compose up -d --build

docker-up-tools: ## Start optional local development tools, including NUI.
	docker compose --profile tools up -d nui

docker-down: ## Stop and remove the local Docker Compose stack.
	docker compose down

docker-logs: ## Follow logs for the local Docker Compose stack.
	docker compose logs -f

docker-ps: ## Show local Docker Compose service status.
	docker compose ps
