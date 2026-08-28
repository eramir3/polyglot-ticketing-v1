# Polyglot Ticketing V1

## Repository Purpose

This repository is a polyglot ticketing microservices system built incrementally
in an Nx monorepo. It contains backend services and shared contracts only.
`ticketing-user-app` is a Next.js architectural dependency, but no frontend
application, routes, components, or UI code belong in this repository.

## Current State

Implemented foundations:

- `api-gateway`: NestJS/TypeScript HTTP backend-for-frontend. It currently
  exposes authentication endpoints and calls identity through gRPC.
- `identity`: NestJS/TypeScript gRPC-only service using Better Auth and
  Postgres. It owns `identity-db`, implements signup, and requires email
  verification.
- `tickets`: Go gRPC service with Postgres-backed ticket creation,
  owner-authorized updates, and public listing and retrieval. It owns
  `tickets-db`.
- `orders`: Go gRPC service with a Postgres-backed ticket projection and order
  reservation lifecycle. It consumes payment events to move accepted
  reservations to `AwaitingPayment`, `Complete`, or `Canceled`, and owns
  `orders-db`.
- `payments`: Go gRPC service with a Postgres-backed Orders projection. It
  creates one payment per eligible, owned order, then its simulated processor
  publishes exactly one payment result through the transactional outbox. It
  owns `payments-db`.
- `expiration`: NestJS worker that consumes `OrderCreated` events from NATS
  JetStream, schedules 15-minute expiry jobs in BullMQ/Redis, and publishes
  `ExpirationComplete` events. Orders consumes those events to cancel `Created`
  reservations. Expiration has no Postgres database or HTTP/gRPC API.
- Shared protobuf contracts in `proto/`, generated with Buf and Protobuf-ES.
- Protovalidate request validation for identity gRPC requests.
- Standardized errors across the gateway and identity service.
- Local Docker Compose infrastructure includes NATS JetStream, Redis for the
  expiration worker, the gateway, identity, orders, payments, their service
  databases, and Mailpit. Optional local tools include NUI, Redis Insight, and
  Grafana with Loki-backed application logs and Prometheus metrics. The Mailpit
  inbox is available on `localhost:8025`, Grafana on `localhost:3002`, and the
  Prometheus UI on `localhost:9090`.

Planned but not implemented: concert-assistant, Kubernetes manifests, GraphQL,
and a Kubernetes Gateway API controller.

Deferred, low-priority work: replace the Payments simulated outcome worker with
a real provider; publish an `OrderCompleted` event or expose payment status
publicly; and have Expiration consume `OrderCanceled` to remove queued BullMQ
expiration jobs.

## Architecture Rules

- UI clients communicate with the API gateway through REST and, when added,
  GraphQL.
- The API gateway communicates with backend services synchronously through
  gRPC.
- Backend services will communicate asynchronously through NATS JetStream.
- Each service owns its persistence, when it has any. A service must not read
  or write another service's database or Redis data.
- Shared protobuf and event contracts are public interfaces. Version them
  carefully and preserve backward compatibility once consumers exist.
- The API gateway is the public HTTP error boundary. Identity is gRPC-only;
  do not add HTTP routes or ports to it without an explicit requirement.

## Contracts And Validation

- Protobuf sources live in `proto/`. Run `pnpm proto:generate` after changing
  protobuf sources; it generates TypeScript contracts in `protogen/ts` and
  exports the pinned Protovalidate schema to `proto-deps/`.
- Identity and API gateway builds run protobuf generation first through their
  respective `generate-proto` targets.
- Tickets builds and tests run protobuf generation first; Go bindings are
  generated in `protogen/go` for tickets and future Go services.
- Gateway DTO validation provides an early HTTP guard. Protovalidate remains
  authoritative for all identity gRPC callers.
- Public errors use `{ "errors": [{ "code", "message", "field"? }] }`.
  See `docs/error-contract.md` for the full contract.
- gRPC services return the relevant gRPC status code and serialize the error
  response JSON in gRPC `details`. The gateway translates that payload to the
  public HTTP response without reclassifying domain errors.
- Verification links target the API gateway's public
  `GET /api/auth/verify-email?token=...` endpoint, which delegates the token
  verification to identity over gRPC. Keep identity gRPC-only.

## Local Development

- Required local configuration is documented in `.env.example`.
- Run `make help` to list the supported local commands.
- Build all services: `make build`; test all services: `make test`.
- Orders and Payments tests include PostgreSQL Testcontainers integration tests
  and require a working Docker container runtime.
- Payments simulation is deterministic by default through
  `PAYMENT_PROCESSOR_OUTCOME`; set `PAYMENT_PROCESSOR_RANDOM_FAILURES=true`
  for local stress testing with an independent 10% failure probability per
  payment.
- Build or run an individual service with `make build-<service>` or
  `make serve-<service>` (for example, `make serve-tickets`).
- Generate contracts with `make generate-proto`.
- Start the local stack with `make docker-up` after configuring the required
  Better Auth and database environment variables.
- Start optional local tooling with `make docker-up-tools`; Grafana is
  available at `localhost:3002` and provisions Loki and Prometheus. Loki
  contains logs from the six application services, and Prometheus scrapes their
  private Compose-network `:9090/metrics` endpoints; both use `service` and
  `environment=local` labels.
- The gateway is published on `localhost:3000`; identity gRPC is internal to
  the Compose network on `identity:50051`; Postgres is published on
  `localhost:5432` for local database tooling. Tickets gRPC is internal on
  `tickets:50052`; its Postgres database is published on `localhost:5433`.
  Orders and Payments gRPC are internal on `orders:50053` and `payments:50054`;
  their Postgres databases are published on `localhost:5434` and `localhost:5435`.

## Services And Persistence

- `tickets` (Go) owns `tickets-db`.
- `orders` (Go) owns `orders-db`.
- `payments` (Go) owns `payments-db`.
- `expiration` (NestJS/BullMQ) uses Redis exclusively for delayed jobs; it
  owns no Postgres database.
- `identity` (NestJS/Better Auth) owns `identity-db`.
- `concert-assistant` (Python RAG) owns `concert-assistant-db`.

## Engineering Guidelines

- Inspect the existing project structure and contracts before changing code.
- Keep changes scoped to the requested service and boundary.
- Prefer existing Nx, NestJS, Buf, Docker Compose, and error-contract patterns.
- Do not introduce shared database access, frontend code, Kubernetes manifests,
  NATS subjects, or production-hardening work unless explicitly requested.
- Add focused executable tests when a changed behavior has test coverage or a
  practical test harness.
- Update `SPEC.md` and contract documentation when architecture, public APIs,
  protobuf contracts, validation, or error behavior changes.
