# Polyglot Ticketing V1 Specification

## Purpose

Polyglot Ticketing V1 is a backend microservices system for concert ticketing.
The repository uses an Nx monorepo for shared tooling and contracts. A separate
Next.js user application is part of the architecture but is intentionally not
implemented in this repository.

## Current Implementation

### API Gateway

`api-gateway` is a NestJS/TypeScript HTTP backend-for-frontend running on
port `3000` in local Docker Compose.

Implemented endpoint:

| Method | Path               | Behavior                                                             |
| ------ | ------------------ | -------------------------------------------------------------------- |
| `POST` | `/api/auth/signup` | Validates a signup request and forwards it to identity through gRPC. |

The gateway validates HTTP payloads with NestJS DTOs, exposes public HTTP
errors, and translates structured gRPC errors returned by backend services.
GraphQL is planned but not implemented.

### Identity

`identity` is a NestJS/TypeScript gRPC-only service. It listens on
`identity:50051` inside local Docker Compose and is not published as an HTTP
service.

Identity implements `identity.v1.IdentityService.SignUp`. It validates requests
with Protovalidate and invokes Better Auth internally to create users. It owns
`identity-db`, a Postgres database, and no other service may access that
database directly.

## Signup Flow

1. A client calls `POST /api/auth/signup` with `name`, `email`, and `password`.
2. The gateway validates the HTTP request with `SignUpDto`.
3. The gateway calls `identity.v1.IdentityService.SignUp` over gRPC.
4. Identity validates the generated protobuf message with Protovalidate.
5. Identity delegates valid signup to Better Auth and returns `userId` and
   `email`.
6. Identity maps validation and Better Auth failures to a structured gRPC
   error. The gateway maps it to the public HTTP error response.

Gateway validation is an early client-facing guard. Identity protobuf
validation is authoritative for every gRPC caller.

## Service Communication

| Direction                          | Transport                 | Status                          |
| ---------------------------------- | ------------------------- | ------------------------------- |
| User application to API gateway    | REST now; GraphQL planned | Gateway REST signup implemented |
| API gateway to backend services    | gRPC                      | Identity signup implemented     |
| Backend service to backend service | NATS JetStream            | Planned                         |

Kubernetes Gateway API will provide ingress and routing in a later deployment
phase. No Kubernetes controller or manifests are implemented yet.

## Protobuf And Validation

Protobuf source files are stored in `proto/` and use versioned packages such
as `identity.v1` and `common.v1`.

Run the following after changing protobuf files:

```bash
pnpm proto:generate
```

The command uses Buf to resolve the pinned Protovalidate dependency, export its
runtime schema to `proto-deps/`, and generate TypeScript Protobuf-ES contracts
to `protogen/ts`. Identity's Nx build depends on this generation step.

`identity.v1.SignUpRequest` requires a non-blank name, an email address, and a
password from 8 through 128 characters.

## Error Contract

The public REST error response is:

```json
{
  "errors": [
    {
      "code": "INVALID_EMAIL",
      "message": "Email must be valid.",
      "field": "email"
    }
  ]
}
```

Identity encodes the same object in gRPC `details` and pairs it with an
appropriate gRPC status code, such as `INVALID_ARGUMENT`, `ALREADY_EXISTS`, or
`INTERNAL`. The gateway parses `details` and retains the error items while
translating the status to HTTP. The full public code list and handling rules
are in `docs/error-contract.md`.

## Local Development

Required values are listed in `.env.example`:

- `IDENTITY_DB_PASSWORD`
- `BETTER_AUTH_SECRET`
- `BETTER_AUTH_URL`

Common commands:

```bash
pnpm nx build identity
pnpm nx build api-gateway
docker compose up -d --build
```

Local ports:

| Component         | Address                              |
| ----------------- | ------------------------------------ |
| API gateway       | `http://localhost:3000`              |
| Identity gRPC     | `identity:50051` within Compose only |
| Identity Postgres | `localhost:5432`                     |

## Planned Services

| Service           | Technology        | Database               |
| ----------------- | ----------------- | ---------------------- |
| ticketing         | Go                | `ticketing-db`         |
| orders            | Go                | `orders-db`            |
| payments          | Go                | `payments-db`          |
| expiration        | NestJS and BullMQ | `expiration-db`        |
| concert-assistant | Python RAG        | `concert-assistant-db` |

NATS JetStream, event subjects, event schemas, CI/CD, observability, and
deployment environments remain open design and implementation work.
