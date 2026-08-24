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

| Method | Path                               | Behavior                                                                 |
| ------ | ---------------------------------- | ------------------------------------------------------------------------ |
| `POST` | `/api/auth/signup`                 | Validates a signup request and forwards it to identity through gRPC.     |
| `POST` | `/api/auth/signin`                 | Signs in with email and password, then sets an HTTP-only session cookie. |
| `POST` | `/api/auth/signout`                | Revokes the current session and expires the HTTP-only session cookie.    |
| `GET`  | `/api/auth/currentuser`            | Returns safe metadata for the authenticated user.                        |
| `GET`  | `/api/auth/verify-email?token=...` | Verifies an email token through identity gRPC.                           |
| `GET`  | `/api/tickets`                     | Retrieves all tickets through tickets gRPC.                              |
| `POST` | `/api/tickets`                     | Creates a ticket for the authenticated user through tickets gRPC.        |

The gateway validates HTTP payloads with NestJS DTOs, exposes public HTTP
errors, and translates structured gRPC errors returned by backend services.
GraphQL is planned but not implemented.

### Identity

`identity` is a NestJS/TypeScript gRPC-only service. It listens on
`identity:50051` inside local Docker Compose and is not published as an HTTP
service.

Identity implements `identity.v1.IdentityService.SignUp`,
`identity.v1.IdentityService.SignIn`,
`identity.v1.IdentityService.SignOut`, and
`identity.v1.IdentityService.CurrentUser`, and
`identity.v1.IdentityService.VerifyEmail`. It validates requests with
Protovalidate and invokes Better Auth internally to create users and verify
email tokens. It owns `identity-db`, a Postgres database, and no other service
may access that database directly.

### Tickets

`tickets` is a Go gRPC service on `tickets:50052` inside local Docker Compose.
It owns `tickets-db` and creates tickets with a generated UUID, non-blank
title, and the authenticated user's ID. Public REST prices are positive integer
minor units from `1` through `9007199254740991` (the JavaScript safe-integer
limit).
It has no public HTTP endpoint; the API gateway owns `GET /api/tickets` and
`POST /api/tickets`.

## List Tickets Flow

1. A client calls `GET /api/tickets` without pagination.
2. The gateway calls `tickets.v1.TicketsService.ListTickets` over gRPC.
3. Tickets retrieves all stored tickets ordered by title and ID.
4. The gateway returns `200` with a JSON array of tickets.

## Create Ticket Flow

1. A client calls `POST /api/tickets` with `title` and a positive integer
   `price` no larger than `9007199254740991`.
2. The gateway validates the request, resolves the current user from the Better
   Auth session, and calls `tickets.v1.TicketsService.CreateTicket` over gRPC.
3. Tickets validates the complete gRPC request and persists the ticket in
   `tickets-db`.
4. The gateway returns `201` with `id`, `title`, `price`, and `userId`.

## Signup Flow

1. A client calls `POST /api/auth/signup` with `name`, `email`, and `password`.
2. The gateway validates the HTTP request with `SignUpDto`.
3. The gateway calls `identity.v1.IdentityService.SignUp` over gRPC.
4. Identity validates the generated protobuf message with Protovalidate.
5. Identity delegates valid signup to Better Auth, sends a verification email,
   and returns `userId` and `email`.
6. Identity maps validation and Better Auth failures to a structured gRPC
   error. The gateway maps it to the public HTTP error response.

Gateway validation is an early client-facing guard. Identity protobuf
validation is authoritative for every gRPC caller.

Better Auth creates new users with an unverified email and sends a verification
message through Mailpit in local development. A user opens the emailed gateway
link, which forwards the token to `VerifyEmail` and returns JSON confirmation.
Duplicate signups retain Better Auth's generic successful response and do not
send another message.

## Signin Flow

1. A client calls `POST /api/auth/signin` with `email` and `password` from the
   configured `TICKETING_USER_APP_ORIGIN`.
2. The gateway validates the HTTP request, forwards the relevant browser
   security headers over gRPC metadata, and calls `IdentityService.SignIn`.
3. Identity validates the generated protobuf message with Protovalidate, then
   delegates credential verification and session creation to Better Auth.
4. Identity returns the session token and expiry only to the gateway. The
   gateway stores the token in the `better-auth.session_token` HTTP-only cookie
   and returns safe user metadata plus the ISO-8601 session expiry.
5. Invalid credentials return `401 INVALID_CREDENTIALS`. Unverified accounts
   return `401 EMAIL_NOT_VERIFIED`; no session is created and no verification
   email is resent during sign-in.

## Signout Flow

1. A client calls `POST /api/auth/signout` with its session cookie.
2. The gateway forwards only `better-auth.session_token` to identity through
   gRPC metadata; no other browser cookies cross the service boundary.
3. Identity reconstructs an in-memory `Headers` object and delegates current
   session revocation to Better Auth.
4. On success, including when the session is absent, expired, or already
   revoked, the gateway expires the session cookie and returns `204 No Content`.
5. If identity cannot revoke the session, the gateway returns its structured
   error and retains the cookie so the client can retry sign-out.

## Current User Flow

1. A client calls `GET /api/auth/currentuser` with its session cookie.
2. The gateway forwards only `better-auth.session_token` to identity through
   gRPC metadata.
3. Identity reconstructs an in-memory `Headers` object and asks Better Auth for
   the current session.
4. A valid session returns safe user metadata: `id`, `name`, `email`, and
   `emailVerified`.
5. A missing, expired, or invalid session returns `401 UNAUTHENTICATED`.

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
password from 8 through 128 characters. `identity.v1.SignInRequest` requires a
valid email address and a password from 8 through 128 characters.

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
- `EMAIL_VERIFICATION_URL`
- `TICKETING_USER_APP_ORIGIN`
- `SMTP_FROM`
- `TICKETS_DB_PASSWORD`

Common commands:

```bash
pnpm nx build identity
pnpm nx build api-gateway
pnpm nx build tickets
pnpm nx test tickets
pnpm nx serve tickets
docker compose up -d --build
```

Local ports:

| Component         | Address                              |
| ----------------- | ------------------------------------ |
| API gateway       | `http://localhost:3000`              |
| Identity gRPC     | `identity:50051` within Compose only |
| Identity Postgres | `localhost:5432`                     |
| Tickets gRPC      | `tickets:50052` within Compose only  |
| Tickets Postgres  | `localhost:5433`                     |
| Mailpit inbox     | `http://localhost:8025`              |

## Planned Services

| Service           | Technology        | Database               |
| ----------------- | ----------------- | ---------------------- |
| tickets           | Go                | `tickets-db`           |
| orders            | Go                | `orders-db`            |
| payments          | Go                | `payments-db`          |
| expiration        | NestJS and BullMQ | `expiration-db`        |
| concert-assistant | Python RAG        | `concert-assistant-db` |

NATS JetStream, event subjects, event schemas, CI/CD, observability, and
deployment environments remain open design and implementation work.
