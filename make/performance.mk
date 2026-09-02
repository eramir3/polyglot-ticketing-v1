.PHONY: stress-tickets check-load-test-token k6-tickets-list k6-tickets-list-seeded k6-tickets-list-comparison seed-performance-tickets

K6_TEST_TYPE ?= tickets-list-empty
K6_EXPECT_TICKET_COUNT ?= 0
K6_TEST_ID ?= $(K6_TEST_TYPE)-$(shell date -u +%Y%m%dT%H%M%SZ)

stress-tickets: ## Run sequential ticket create/update stress cycles (requires STRESS_COOKIE).
	node scripts/stress/tickets.js

check-load-test-token:
	@test -n "$${LOAD_TEST_METRICS_TOKEN:-$$(sed -n 's/^LOAD_TEST_METRICS_TOKEN=//p' .env 2>/dev/null | tail -n 1)}" || (echo "LOAD_TEST_METRICS_TOKEN is required in .env or the shell environment"; exit 1)

k6-tickets-list: check-load-test-token ## Run the empty-list k6 GET /api/tickets baseline (requires docker-up-tools).
	docker compose --profile performance --profile tools run --rm -e K6_EXPECT_TICKET_COUNT=$(K6_EXPECT_TICKET_COUNT) -e K6_TEST_ID=$(K6_TEST_ID) k6 run -o experimental-prometheus-rw --tag source=k6 --tag test_type=$(K6_TEST_TYPE) --tag environment=local --tag testid=$(K6_TEST_ID) /scripts/tickets-list.js

k6-tickets-list-seeded: K6_EXPECT_TICKET_COUNT = 100
k6-tickets-list-seeded: K6_TEST_TYPE = tickets-list-seeded
k6-tickets-list-seeded: k6-tickets-list ## Run the seeded 100-ticket k6 GET /api/tickets load test.

k6-tickets-list-comparison: check-load-test-token ## Reset local data and run the empty and 100-ticket k6 comparison.
	$(MAKE) docker-reset
	$(MAKE) docker-up-tools
	$(MAKE) k6-tickets-list
	$(MAKE) seed-performance-tickets
	$(MAKE) k6-tickets-list-seeded

seed-performance-tickets: ## Seed exactly 100 tickets into an empty local tickets-db.
	docker compose --profile performance --profile tools run --rm tickets-performance-seed
