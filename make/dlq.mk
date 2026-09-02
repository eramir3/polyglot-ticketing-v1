.PHONY: replay-orders-dlq replay-tickets-dlq replay-payments-dlq replay-expiration-dlq

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
