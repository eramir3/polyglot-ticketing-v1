.PHONY: docker-build docker-up docker-up-tools docker-down docker-reset docker-logs docker-ps restore-concerts concert-assistant-up

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

restore-concerts: ## Replace the local Concert Assistant concert dataset from concerts.dump.
	docker compose up -d --wait concert-assistant-db
	docker compose exec -T concert-assistant-db sh /restore/restore-concerts.sh

concert-assistant-up: ## Start only Concert Assistant and its database dependency.
	docker compose up -d --build --wait concert-assistant

docker-logs: ## Follow logs for the local Docker Compose stack.
	docker compose logs -f

docker-ps: ## Show local Docker Compose service status.
	docker compose ps
