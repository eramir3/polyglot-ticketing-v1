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
- Shared protobuf contracts in `proto/`, generated with Buf and Protobuf-ES.
- Protovalidate request validation for identity gRPC requests.
- Standardized errors across the gateway and identity service.
- Local Docker Compose infrastructure for the gateway, identity, `identity-db`,
  and Mailpit. The Mailpit inbox is available on `localhost:8025`.

Planned but not implemented: orders, payments, expiration,
concert-assistant, NATS JetStream, Kubernetes manifests, GraphQL, and a
Kubernetes Gateway API controller.

## Architecture Rules

- UI clients communicate with the API gateway through REST and, when added,
  GraphQL.
- The API gateway communicates with backend services synchronously through
  gRPC.
- Backend services will communicate asynchronously through NATS JetStream.
- Each service owns its database. A service must not read or write another
  service's database.
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
- Build or run an individual service with `make build-<service>` or
  `make serve-<service>` (for example, `make serve-tickets`).
- Generate contracts with `make generate-proto`.
- Start the local stack with `make docker-up` after configuring the required
  Better Auth and database environment variables.
- The gateway is published on `localhost:3000`; identity gRPC is internal to
  the Compose network on `identity:50051`; Postgres is published on
  `localhost:5432` for local database tooling. Tickets gRPC is internal on
  `tickets:50052`; its Postgres database is published on `localhost:5433`.

## Planned Services And Databases

- `tickets` (Go) owns `tickets-db`.
- `orders` (Go) owns `orders-db`.
- `payments` (Go) owns `payments-db`.
- `expiration` (NestJS/BullMQ) owns `expiration-db`.
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
