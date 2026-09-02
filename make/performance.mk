.PHONY: stress-tickets check-load-test-token validate-k6-profile validate-k6-tickets-create-profile prepare-k6-tickets-list k6-tickets-list seed-performance-tickets prepare-k6-tickets-create k6-tickets-create

K6_PROFILE ?= smoke
K6_TEST_TYPE ?= tickets-list-$(K6_PROFILE)
K6_EXPECT_TICKET_COUNT ?= 100
K6_TEST_ID ?= $(K6_TEST_TYPE)-$(shell date -u +%Y%m%dT%H%M%SZ)
K6_TICKETS_CREATE_TEST_TYPE ?= tickets-create-$(K6_PROFILE)
K6_TICKETS_CREATE_TEST_ID ?= $(K6_TICKETS_CREATE_TEST_TYPE)-$(shell date -u +%Y%m%dT%H%M%SZ)

stress-tickets: ## Run sequential ticket create/update stress cycles (requires STRESS_COOKIE).
	node scripts/stress/tickets.js

check-load-test-token:
	@test -n "$${LOAD_TEST_METRICS_TOKEN:-$$(sed -n 's/^LOAD_TEST_METRICS_TOKEN=//p' .env 2>/dev/null | tail -n 1)}" || (echo "LOAD_TEST_METRICS_TOKEN is required in .env or the shell environment"; exit 1)

validate-k6-profile:
	@node -e 'const configs = require("./tests/performance/k6/tickets-list-configs.json"); const profile = process.argv[1]; if (!Object.prototype.hasOwnProperty.call(configs, profile)) { console.error("K6_PROFILE must be one of: " + Object.keys(configs).join(", ")); process.exit(1); }' "$(K6_PROFILE)"

validate-k6-tickets-create-profile:
	@node -e 'const configs = require("./tests/performance/k6/tickets-create-configs.json"); const profile = process.argv[1]; if (!Object.prototype.hasOwnProperty.call(configs, profile)) { console.error("K6_PROFILE must be one of: " + Object.keys(configs).join(", ")); process.exit(1); }' "$(K6_PROFILE)"

prepare-k6-tickets-list: ## Reset local data, start tools, and seed the 100-ticket k6 dataset.
	$(MAKE) docker-reset
	$(MAKE) docker-up-tools
	$(MAKE) seed-performance-tickets

k6-tickets-list: check-load-test-token validate-k6-profile ## Run the selected k6 ticket-list profile (K6_PROFILE=smoke|load|stress).
	docker compose --profile performance --profile tools run --rm -e K6_TEST_ID=$(K6_TEST_ID) k6 run -e K6_EXPECT_TICKET_COUNT=$(K6_EXPECT_TICKET_COUNT) -e K6_PROFILE=$(K6_PROFILE) -e LOAD_TEST_METRICS_TOKEN="$${LOAD_TEST_METRICS_TOKEN:-$$(sed -n 's/^LOAD_TEST_METRICS_TOKEN=//p' .env 2>/dev/null | tail -n 1)}" -o experimental-prometheus-rw --tag source=k6 --tag test_type=$(K6_TEST_TYPE) --tag environment=local --tag testid=$(K6_TEST_ID) /scripts/tickets-list.js

prepare-k6-tickets-create: ## Reset local data and start tools for ticket-create k6 tests.
	$(MAKE) docker-reset
	$(MAKE) docker-up-tools

k6-tickets-create: check-load-test-token validate-k6-tickets-create-profile ## Run the selected k6 ticket-create profile (K6_PROFILE=smoke|load|stress).
	docker compose --profile performance --profile tools run --rm -e K6_TEST_ID=$(K6_TICKETS_CREATE_TEST_ID) k6 run -e K6_PROFILE=$(K6_PROFILE) -e K6_TEST_ID=$(K6_TICKETS_CREATE_TEST_ID) -e LOAD_TEST_METRICS_TOKEN="$${LOAD_TEST_METRICS_TOKEN:-$$(sed -n 's/^LOAD_TEST_METRICS_TOKEN=//p' .env 2>/dev/null | tail -n 1)}" -o experimental-prometheus-rw --tag source=k6 --tag test_type=$(K6_TICKETS_CREATE_TEST_TYPE) --tag environment=local --tag testid=$(K6_TICKETS_CREATE_TEST_ID) /scripts/tickets-create.js

seed-performance-tickets: ## Seed exactly 100 tickets into an empty local tickets-db.
	docker compose --profile performance --profile tools run --rm tickets-performance-seed
