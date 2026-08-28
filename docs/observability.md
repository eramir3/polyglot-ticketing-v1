# Local Loki log catalog

This catalog lists the intentional, high-signal logs added to the local
observability stack. It is a troubleshooting reference, not an inventory of
every line a service writes.

Start the opt-in stack with `make docker-up-tools`, then open Grafana at
`http://localhost:3002` and select the Loki datasource in **Explore**. Grafana
Alloy collects Docker stdout from `api-gateway`, `identity`, `tickets`,
`orders`, `payments`, and `expiration`.

## Labels and queries

Every collected stream has these fixed labels:

| Label | Value |
| --- | --- |
| `service` | The emitting Compose service, for example `tickets` or `payments`. |
| `environment` | `local` |

Identifiers such as ticket, order, payment, and event IDs deliberately remain
in the log content instead of becoming labels. This keeps Loki label
cardinality bounded. Start a query with a service selector and add a line
filter for an identifier when needed:

```logql
{service="orders", environment="local"} |= "order_id=01..."
```

The examples below are ready to paste into Grafana Explore. Loki retains this
local data for seven days.

## Intentional high-signal logs

| Source | Level | Message and trigger | Searchable values | LogQL |
| --- | --- | --- | --- | --- |
| Tickets gRPC server | Error | `ticket creation failed` — an unexpected failure while creating a ticket. | `operation=create_ticket`, `error` | `{service="tickets", environment="local"} |= "ticket creation failed"` |
| Tickets gRPC server | Error | `ticket update failed` — an unexpected failure while updating a ticket. | `operation=update_ticket`, `ticket_id`, `error` | `{service="tickets", environment="local"} |= "ticket update failed"` |
| Orders gRPC server | Error | `order creation failed` — an unexpected failure while reserving a ticket. | `operation=create_order`, `ticket_id`, `error` | `{service="orders", environment="local"} |= "order creation failed"` |
| Orders gRPC server | Error | `order cancellation failed` — an unexpected failure while canceling an order. | `operation=cancel_order`, `order_id`, `error` | `{service="orders", environment="local"} |= "order cancellation failed"` |
| Payments gRPC server | Error | `payment creation failed` — an unexpected failure while creating a payment. | `operation=create_payment`, `order_id`, `error` | `{service="payments", environment="local"} |= "payment creation failed"` |
| Payments processor | Warn | `payment failed` — a payment was intentionally resolved as failed by the simulated processor; this is a committed outcome, not an infrastructure error. | `operation=resolve_payment`, `payment_id`, `order_id` | `{service="payments", environment="local"} |= "payment failed"` |
| Expiration `OrderCreated` consumer | Error | `terminal OrderCreated event` — a malformed `OrderCreated` delivery was terminated and will not be retried. | `error` | `{service="expiration", environment="local"} |= "terminal OrderCreated event"` |
| Expiration `OrderCreated` consumer | Warn | `order-created handling failed; event will be retried` — scheduling the expiration job failed transiently and the JetStream delivery was negatively acknowledged. | `error` | `{service="expiration", environment="local"} |= "order-created handling failed; event will be retried"` |
| Expiration BullMQ processor | Error | `expiration-complete publish failed; job will be retried` — publishing `ExpirationComplete` failed, so BullMQ retries the job. | `order_id` in the message, `error` | `{service="expiration", environment="local"} |= "expiration-complete publish failed"` |
| Shared transactional outbox in Tickets, Orders, and Payments | Warn | `outbox event dispatch failed` — publishing an event or recording its publish result failed. | `operation` (`publish`, `mark_failed`, or `mark_published`), `event_id`, `subject`, `error` | `{environment="local", service=~"tickets|orders|payments"} |= "outbox event dispatch failed"` |

## Out of scope

The catalog intentionally excludes normal validation, authorization, and
expected domain responses; success and broad request logging; generic startup
or reconnect messages; metrics; tracing; dashboards; alerts; and production
observability configuration.
