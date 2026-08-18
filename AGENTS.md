# Polyglot Ticketing V1

## Repository Purpose

This repository is planned as a polyglot ticketing microservices application.
The system will be built step by step, starting with documentation and
architecture alignment before any application scaffolding or service code.

The intended architecture includes:

- `ticketing`: Go service for tickets and concert inventory.
- `orders`: Go service for order lifecycle management.
- `payments`: Go service for payment workflows.
- `expiration`: NestJS service using BullMQ for order expiration workflows.
- `identity`: NestJS service using Better Auth for authentication and user identity.
- `api-gateway`: Kubernetes Gateway API based gateway and backend-for-frontend layer.
- `concert-assistant`: Python RAG service for AI-powered concert questions.
- `ticketing-user-app`: Next.js user interface. This is architectural context
  only; this repository is not intended to contain frontend implementation code.

## Current Project Phase

The project is in the documentation/specification phase.

Do not scaffold the Nx workspace, create service code, add Kubernetes manifests,
or introduce infrastructure files until the next implementation step is
explicitly requested.

The first concrete deliverables are:

- `AGENTS.md`: contributor and agent working instructions.
- `SPEC.md`: project specification and architecture description.

## Planned Technology Stack

- Monorepo: Nx.
- User interface: Next.js, tracked as architecture context only. Do not create
  frontend apps, components, routes, or UI code in this repository.
- Identity service: NestJS, TypeScript, Better Auth, Postgres.
- Expiration service: NestJS, TypeScript, BullMQ.
- Ticketing, orders, and payments services: Go.
- Concert assistant service: Python RAG system.
- Event bus: NATS JetStream.
- Databases: Postgres with database per microservice.
- Kubernetes networking: Gateway API.
- UI to gateway communication: REST and GraphQL.
- Gateway to microservice communication: gRPC.
- Async service-to-service communication: NATS JetStream events.

## Database Plan

Each service owns its own database. Planned databases:

- `ticketing-db`
- `orders-db`
- `payments-db`
- `identity-db`
- `expiration-db`
- `concert-assistant-db`

Services must not directly read or write another service's database.
Cross-service state changes should happen through gRPC calls or NATS JetStream
events, depending on whether the workflow is synchronous or asynchronous.

## Communication Model

- The user app communicates with the API gateway synchronously through REST and
  GraphQL.
- The API gateway communicates with backend microservices synchronously through
  gRPC.
- Backend microservices communicate asynchronously through NATS JetStream.
- Event contracts should be treated as shared public interfaces and versioned
  carefully once implementation begins.

## Initial Implementation Sequence

Build the project incrementally:

1. Create repository documentation: `AGENTS.md` and `SPEC.md`.
2. Scaffold the Nx monorepo.
3. Add shared contract/tooling foundations, including protobuf and event schema
   locations.
4. Create the identity service and a minimal authentication flow.
5. Create the first core ticketing and ordering happy path.
6. Add payments, expiration, and concert assistant capabilities after the core
   flow is stable.
7. Add Kubernetes manifests and local development infrastructure once service
   boundaries are clearer.

## Open Decisions

These decisions are intentionally deferred:

- Concrete Kubernetes Gateway API controller.
- Exact GraphQL schema and ownership boundaries.
- Exact protobuf package layout.
- Event subject naming and schema format.
- Local development database orchestration.
- CI/CD and deployment environments.
- Observability, tracing, metrics, and logging stack.

## Engineering Guidelines

- Inspect the existing repository before making changes.
- Keep changes scoped to the current requested step.
- Prefer the existing project structure once it exists.
- Preserve service boundaries.
- Do not couple services through shared database access.
- Do not introduce production-hardening work before the foundational structure
  exists unless explicitly requested.
- Add tests when implementation begins and the changed behavior has executable
  surface area.
