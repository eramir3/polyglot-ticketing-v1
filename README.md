# Polyglot Ticketing V1

Polyglot Ticketing V1 is a backend microservices system for concert ticketing.
It is an Nx monorepo containing backend services and shared contracts. The
separate Next.js user application is an architectural dependency, but no
frontend application or UI code belongs in this repository.

## Architecture

The API gateway is the public REST boundary. It calls backend services over
gRPC; backend services exchange lifecycle events asynchronously through NATS
JetStream. Each service owns its own persistence and never reads another
service's database or Redis data.

| Service | Language | Responsibility | Persistence |
| --- | --- | --- | --- |
| `api-gateway` | TypeScript / NestJS | Public REST API, session boundary, and gRPC client | None |
| `identity` | TypeScript / NestJS | Signup, sign-in, email verification, and session management | `identity-db` |
| `tickets` | Go | Ticket creation, ownership-authorized updates, and public reads | `tickets-db` |
| `orders` | Go | Ticket reservation, order lifecycle, and ticket projection | `orders-db` |
| `payments` | Go | Order projection, payment creation, and simulated processing | `payments-db` |
| `expiration` | TypeScript / NestJS | Schedules 15-minute order expirations and emits completion events | Redis / BullMQ |

Shared protobuf contracts live in [`proto/`](proto/). Generated TypeScript and
Go bindings are produced through Buf.

## Quick start

Prerequisites: Docker (including Docker Compose), Node.js with pnpm, Go, and
the required Better Auth configuration values.

```bash
cp .env.example .env
# Edit .env: set database passwords and a random BETTER_AUTH_SECRET.

make install
make docker-up
```

The stack builds service images, applies database migrations, and starts the
gateway, services, Postgres databases, NATS JetStream, Redis, and Mailpit.

Run `make help` for the complete command list. Common commands are:

```bash
make build                 # Build every service
make test                  # Run all executable tests
make generate-proto        # Regenerate protobuf bindings
make serve-tickets         # Run one service locally
make serve-payments
make serve-expiration
make docker-logs           # Follow Compose logs
make docker-down           # Stop the stack
```

Gateway integration tests and the Orders and Payments integration tests use
Testcontainers, so they require a working Docker container runtime.

For local payment stress testing, set
`PAYMENT_PROCESSOR_RANDOM_FAILURES=true` in `.env`. The simulated processor
then resolves each payment with an independent 10% failure probability;
otherwise `PAYMENT_PROCESSOR_OUTCOME` controls a deterministic result.

Prepare the deterministic ticket-list performance dataset:

```bash
make prepare-k6-tickets-list
```

This command removes local application and observability data, recreates the
databases, starts the observability tools, and inserts exactly 100 deterministic
read-only ticket records. The seed refuses to run if `tickets-db` is not empty.

Then run exactly one ticket-list profile at a time:

```bash
make k6-tickets-list K6_PROFILE=smoke
make k6-tickets-list K6_PROFILE=load
make k6-tickets-list K6_PROFILE=stress
```

All profiles use a Dockerized k6 runner on the Compose network and exercise the
public `GET /api/tickets` route against the 100-ticket dataset. `smoke` retains
the short one-to-five virtual-user ramp. `load` sustains 20 requests per second
for five minutes and requires successful checks, fewer than 1% HTTP failures,
a p95 below one second, and no dropped iterations. `stress` holds 20, 40, 80,
and 160 requests per second for one minute each, reporting saturation without
failing on those results. To target another reachable gateway, set `K6_BASE_URL`,
for example:

```bash
K6_BASE_URL=http://api-gateway:3000 make k6-tickets-list K6_PROFILE=load
```

Profile descriptions, scenarios, thresholds, and think time are versioned in
[`tests/performance/k6/tickets/list-configs.json`](tests/performance/k6/tickets/list-configs.json).

Ticket creation has the same profile names with conservative write rates. Its
setup creates, verifies through Mailpit, and signs in one disposable user; the
measured requests then share that session. It does not seed tickets because the
test writes them:

```bash
make prepare-k6-tickets-create
make k6-tickets-create K6_PROFILE=smoke
make k6-tickets-create K6_PROFILE=load
make k6-tickets-create K6_PROFILE=stress
```

The create load profile runs at five creates per second for five minutes. Its
stress profile holds 5, 10, 20, and 40 creates per second for one minute each.
The generated tickets remain in the local database until the next reset.
Profile definitions are in
[`tests/performance/k6/tickets/create-configs.json`](tests/performance/k6/tickets/create-configs.json).

Set a random `LOAD_TEST_METRICS_TOKEN` in `.env` before running either test. k6
uses that token only to mark its gateway requests in local metrics. Grafana's
**Tickets API Performance** dashboard filters smoke, load, and stress runs by
profile, run ID, and endpoint, and shows their dropped-iteration rate alongside
normal gateway traffic.

## Public API

The API gateway runs at `http://localhost:3000`. Authentication endpoints set
and read an HTTP-only session cookie. The implemented routes are:

| Area | Routes |
| --- | --- |
| Authentication | `POST /api/auth/signup`, `POST /api/auth/signin`, `POST /api/auth/signout`, `GET /api/auth/currentuser`, `GET /api/auth/verify-email?token=...` |
| Tickets | `GET /api/tickets`, `GET /api/tickets/:id`, `POST /api/tickets`, `PUT /api/tickets/:id` |
| Orders | `GET /api/orders`, `GET /api/orders/:id`, `POST /api/orders`, `DELETE /api/orders/:id` |
| Payments | `POST /api/payments` |

Identity and the Tickets, Orders, and Payments gRPC services are private to the
Compose network. They are not public HTTP endpoints.

## Local tools and observability

Start the optional local tooling with:

```bash
make docker-up-tools
```

| Tool | Address | Purpose |
| --- | --- | --- |
| Mailpit | `http://localhost:8025` | Local email inbox |
| NATS monitoring | `http://localhost:8222` | NATS server monitoring |
| NUI | `http://localhost:31311` | NATS JetStream UI with the current event protobuf schemas |
| Redis Insight | `http://localhost:5540` | Expiration Redis / BullMQ inspection |
| Grafana | `http://localhost:3002` | Explore Loki logs, Prometheus metrics, and Tempo traces |
| Prometheus | `http://localhost:9090` | Metrics query UI |

Grafana provisions Loki, Prometheus, and Tempo automatically. Application
metrics endpoints remain private to the Compose network. See the
[observability catalog](docs/observability.md) for tracked logs, metrics,
queries, retention, and tracing details.

## Further documentation

- [System specification](SPEC.md): service behavior, flows, contracts, and local ports.
- [Public error contract](docs/error-contract.md): REST and gRPC error format.
- [Observability catalog](docs/observability.md): intentional high-signal logs, metrics, and tracing.

## Deferred work

The following are intentionally deferred and low priority: replacing the
simulated payment processor with a real provider, publishing an
`OrderCompleted` event or exposing payment status publicly, and removing a
queued expiration job after an order cancellation.
