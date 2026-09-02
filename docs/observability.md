# Local observability catalog

This catalog lists the intentional, high-signal logs and metrics in the local
observability stack. It is a troubleshooting reference, not an inventory of
every line a service writes or every metric a client library exports.

Start the opt-in stack with `make docker-up-tools`, then open Grafana at
`http://localhost:3002`. Grafana provisions Loki, Prometheus, and Tempo datasources.
The Prometheus UI is available only on `http://localhost:9090`.

## Tempo traces

Tempo receives local traces through Alloy. Services export OTLP to Alloy's
private Compose-network endpoint; Alloy batches and forwards traces to Tempo.
Grafana Explore is the supported trace UI. Traces include HTTP, gRPC, NATS,
outbox, BullMQ, and payment-processor operations, with W3C trace context
preserved across durable handoffs. They contain transport metadata only: no
aggregate IDs, users, payloads, SQL, or credentials. Tempo retains local data
for seven days. Trace-to-log correlation is intentionally deferred.

## Prometheus metrics

Prometheus scrapes the private `:9090/metrics` endpoint of `api-gateway`,
`identity`, `tickets`, `orders`, `payments`, and `expiration` over the Compose
network. Those endpoints have no host-port mapping and are not public API
routes. Prometheus itself retains local data for seven days.

k6 sends its short-lived load-test metrics to Prometheus's private remote-write
receiver. These are separate `k6_*` series, tagged with `source="k6"`,
`test_type`, and a unique `testid`; they are not application metrics. The
provisioned Grafana **Ticketing / k6 Load Tests** dashboard filters them by
test type and `testid`. `make k6-tickets-list-comparison` resets local Compose
data, produces the `tickets-list-empty` baseline, seeds the deterministic
100-ticket dataset, and produces the `tickets-list-seeded` run. Individual
commands remain available for running either scenario separately.

Every scraped series has the fixed `service` and `environment="local"` target
labels. Ticket, order, payment, event, and user identifiers must never be
metric labels.

### Metric format

Prometheus exposes metrics as plaintext time series at each service's private
`GET :9090/metrics` endpoint. The basic format is:

```text
metric_name{label="value", other_label="value"} number
```

For example, one completed gateway request could look like:

```text
ticketing_app_http_server_requests_total{
  environment="local",
  service="api-gateway",
  method="POST",
  route="/api/tickets",
  status="201",
  traffic_source="other"
} 42
```

`environment` and `service` are added by Prometheus when it scrapes each
target; the application provides the request labels. API gateway request
metrics also include `traffic_source`: it is `k6` only when the private
`X-Ticketing-Load-Test-Token` header matches `LOAD_TEST_METRICS_TOKEN`, and
`other` for every other request.

Counters are cumulative and conventionally end in `_total`:

```text
ticketing_app_background_operations_total{
  service="payments",
  component="payment_processor",
  operation="resolve_payment",
  outcome="failed"
} 3
```

Durations use histograms. A single logical metric becomes multiple exported
time series:

```text
ticketing_app_grpc_server_request_duration_seconds_bucket{
  service="orders",
  method="/orders.v1.OrdersService/CreateOrder",
  code="OK",
  le="0.1"
} 91

ticketing_app_grpc_server_request_duration_seconds_sum{...} 4.82
ticketing_app_grpc_server_request_duration_seconds_count{...} 96
```

- `_bucket`: number of observations at or below the `le` duration threshold.
- `_sum`: total duration observed.
- `_count`: total number of observations.

This is the standard Prometheus exposition model. [Prometheus's
exposition-format documentation](https://prometheus.io/docs/instrumenting/exposition_formats/)
describes the same counter and histogram structure.

| Metric family                                                                                      | Emitted by                            | Labels beyond target labels            | Purpose                                                                          |
| -------------------------------------------------------------------------------------------------- | ------------------------------------- | -------------------------------------- | -------------------------------------------------------------------------------- |
| `go_*`, `process_*`                                                                                | Tickets, Orders, Payments             | Client-defined runtime labels only     | Go runtime and process health.                                                   |
| `nodejs_*`, `process_*`                                                                            | API Gateway, Identity, Expiration     | Client-defined runtime labels only     | Node.js runtime and process health.                                              |
| `ticketing_app_http_server_requests_total`, `ticketing_app_http_server_request_duration_seconds`   | API Gateway                           | `method`, normalized `route`, `status`, `traffic_source` | HTTP request rate, errors, and latency, split into verified k6 and other traffic. |
| `k6_*`                                                                                              | k6 load tests                         | `source`, `test_type`, `testid`, endpoint-specific tags | Load-test request rate, failures, checks, latency, and virtual users. |
| `ticketing_app_grpc_server_requests_total`, `ticketing_app_grpc_server_request_duration_seconds`   | Tickets, Orders, Payments, Identity   | `method`, `code`                       | gRPC request rate, errors, and latency.                                          |
| `ticketing_app_background_operations_total`, `ticketing_app_background_operation_duration_seconds` | Tickets, Orders, Payments, Expiration | `component`, `operation`, `outcome`    | JetStream consumer, outbox, payment processor, and BullMQ outcomes and duration. |

The **Ticketing / k6 Load Tests** dashboard includes per-service application
CPU usage derived from `process_cpu_seconds_total`. It is CPU time as a
percentage of one core, not CPU usage relative to a container limit; a
multithreaded process can exceed 100%.

Useful PromQL examples:

```promql
sum by (service, code) (
  rate(ticketing_app_grpc_server_requests_total{environment="local"}[5m])
)
```

```promql
histogram_quantile(
  0.95,
  sum by (le, service, route) (
    rate(ticketing_app_http_server_request_duration_seconds_bucket{environment="local"}[5m])
  )
)
```

```promql
sum by (service, component, operation, outcome) (
  increase(ticketing_app_background_operations_total{environment="local"}[15m])
)
```

## Loki logs

Grafana Alloy collects Docker stdout from `api-gateway`, `identity`, `tickets`,
`orders`, `payments`, and `expiration`. Every collected stream has these fixed
labels:

| Label         | Value                                                              |
| ------------- | ------------------------------------------------------------------ |
| `service`     | The emitting Compose service, for example `tickets` or `payments`. |
| `environment` | `local`                                                            |

Identifiers such as ticket, order, payment, and event IDs deliberately remain
in the log content instead of becoming labels. This keeps Loki label
cardinality bounded. Start a query with a service selector and add a line
filter for an identifier when needed:

```logql
{service="orders", environment="local"} |= "order_id=01..."
```

Loki retains local data for seven days.

## Intentional high-signal logs

| Source                                                       | Level | Message and trigger                                                                                                                                               | Searchable values                                                                           |
| ------------------------------------------------------------ | ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Tickets gRPC server                                          | Error | `ticket creation failed` — an unexpected failure while creating a ticket.                                                                                         | `operation=create_ticket`, `error`                                                          |
| Tickets gRPC server                                          | Error | `ticket update failed` — an unexpected failure while updating a ticket.                                                                                           | `operation=update_ticket`, `ticket_id`, `error`                                             |
| Orders gRPC server                                           | Error | `order creation failed` — an unexpected failure while reserving a ticket.                                                                                         | `operation=create_order`, `ticket_id`, `error`                                              |
| Orders gRPC server                                           | Error | `order cancellation failed` — an unexpected failure while canceling an order.                                                                                     | `operation=cancel_order`, `order_id`, `error`                                               |
| Payments gRPC server                                         | Error | `payment creation failed` — an unexpected failure while creating a payment.                                                                                       | `operation=create_payment`, `order_id`, `error`                                             |
| Payments processor                                           | Warn  | `payment failed` — a payment was intentionally resolved as failed by the simulated processor; this is a committed outcome, not an infrastructure error.           | `operation=resolve_payment`, `payment_id`, `order_id`                                       |
| Expiration `OrderCreated` consumer                           | Warn  | `order-created handling failed; event will be retried` — scheduling the expiration job failed transiently and the JetStream delivery was negatively acknowledged. | `error`                                                                                     |
| Expiration `OrderCreated` consumer                           | Warn  | `OrderCreated event parked in dead letter queue` — a malformed delivery, or a transient failure after five retries, was retained in `EXPIRATION_DLQ`.             | `subject`, `streamSequence`, `deliveryCount`, `failureClass`, `error`                      |
| Expiration `OrderCreated` consumer                           | Error | `failed to park OrderCreated event in dead letter queue; event will be retried` — the source event remains unacknowledged until it can be parked safely.          | `error`                                                                                     |
| Expiration BullMQ processor                                  | Error | `expiration-complete publish failed; job will be retried` — publishing `ExpirationComplete` failed, so BullMQ retries the job.                                    | `order_id` in the message, `error`                                                          |
| Orders JetStream consumers                                   | Warn  | `… event parked in dead letter queue` — a malformed delivery, or a transient failure after five retries, was retained in `ORDERS_DLQ`.                              | `subject`, `stream_sequence`, `delivery_count`, `failure_class`, `error`                    |
| Orders JetStream consumers                                   | Error | `failed to park … event in dead letter queue; event will be retried` — the source event remains unacknowledged until it can be parked safely.                        | `subject`, `error`                                                                          |
| Tickets order-event consumers                                | Warn  | `order event parked in dead letter queue` — a malformed delivery, or a transient failure after five retries, was retained in `TICKETS_DLQ`.                         | `subject`, `stream_sequence`, `delivery_count`, `failure_class`, `error`                    |
| Tickets order-event consumers                                | Error | `failed to park order event in dead letter queue; event will be retried` — the source event remains unacknowledged until it can be parked safely.                   | `subject`, `error`                                                                          |
| Payments order-projection consumer                           | Warn  | `payments order event parked in dead letter queue` — a malformed delivery, or a transient failure after five retries, was retained in `PAYMENTS_DLQ`.              | `subject`, `stream_sequence`, `delivery_count`, `failure_class`, `error`                    |
| Payments order-projection consumer                           | Error | `failed to park payments order event in dead letter queue; event will be retried` — the source event remains unacknowledged until it can be parked safely.          | `subject`, `error`                                                                          |
| Shared transactional outbox in Tickets, Orders, and Payments | Warn  | `outbox event dispatch failed` — publishing an event or recording its publish result failed.                                                                      | `operation` (`publish`, `mark_failed`, or `mark_published`), `event_id`, `subject`, `error` |

### LogQL queries

- Tickets creation: `{service="tickets", environment="local"} |= "ticket creation failed"`
- Tickets update: `{service="tickets", environment="local"} |= "ticket update failed"`
- Orders creation: `{service="orders", environment="local"} |= "order creation failed"`
- Orders cancellation: `{service="orders", environment="local"} |= "order cancellation failed"`
- Payments creation: `{service="payments", environment="local"} |= "payment creation failed"`
- Payments simulated failure: `{service="payments", environment="local"} |= "payment failed"`
- Expiration retryable scheduling failure: `{service="expiration", environment="local"} |= "order-created handling failed; event will be retried"`
- Expiration DLQ: `{service="expiration", environment="local"} |= "OrderCreated event parked in dead letter queue"`
- Expiration DLQ publication failure: `{service="expiration", environment="local"} |= "failed to park OrderCreated event in dead letter queue"`
- Expiration retryable publishing failure: `{service="expiration", environment="local"} |= "expiration-complete publish failed"`
- Orders DLQ: `{service="orders", environment="local"} |= "event parked in dead letter queue"`
- Orders DLQ publication failure: `{service="orders", environment="local"} |= "dead letter queue; event will be retried"`
- Tickets DLQ: `{service="tickets", environment="local"} |= "order event parked in dead letter queue"`
- Tickets DLQ publication failure: `{service="tickets", environment="local"} |= "failed to park order event in dead letter queue"`
- Payments DLQ: `{service="payments", environment="local"} |= "payments order event parked in dead letter queue"`
- Payments DLQ publication failure: `{service="payments", environment="local"} |= "failed to park payments order event in dead letter queue"`
- Transactional outbox: `{environment="local", service=~"tickets|orders|payments"} |= "outbox event dispatch failed"`

## Out of scope

The catalog intentionally excludes normal validation, authorization, and
expected domain responses; success and broad request logging; generic startup
or reconnect messages; business lifecycle metrics; tracing and Tempo; alerts;
and production observability configuration.
