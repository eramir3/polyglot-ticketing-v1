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
| NUI | `http://localhost:31311` | NATS JetStream UI |
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
