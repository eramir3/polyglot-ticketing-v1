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

| Method   | Path                               | Behavior                                                                 |
| -------- | ---------------------------------- | ------------------------------------------------------------------------ |
| `POST`   | `/api/auth/signup`                 | Validates a signup request and forwards it to identity through gRPC.     |
| `POST`   | `/api/auth/signin`                 | Signs in with email and password, then sets an HTTP-only session cookie. |
| `POST`   | `/api/auth/signout`                | Revokes the current session and expires the HTTP-only session cookie.    |
| `GET`    | `/api/auth/currentuser`            | Returns safe metadata for the authenticated user.                        |
| `GET`    | `/api/auth/verify-email?token=...` | Verifies an email token through identity gRPC.                           |
| `GET`    | `/api/tickets`                     | Retrieves all tickets through tickets gRPC.                              |
| `GET`    | `/api/tickets/:id`                 | Retrieves a ticket by ID through tickets gRPC.                           |
| `POST`   | `/api/tickets`                     | Creates a ticket for the authenticated user through tickets gRPC.        |
| `PUT`    | `/api/tickets/:id`                 | Updates an owned ticket through tickets gRPC.                            |
| `GET`    | `/api/orders`                      | Retrieves the authenticated user's orders through orders gRPC.           |
| `GET`    | `/api/orders/:id`                  | Retrieves one owned order through orders gRPC.                           |
| `POST`   | `/api/orders`                      | Creates an order for the authenticated user through orders gRPC.         |
| `DELETE` | `/api/orders/:id`                  | Cancels one owned active order through orders gRPC.                      |

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
`GET /api/tickets/:id`, plus `POST /api/tickets` and `PUT /api/tickets/:id`.

### Orders

`orders` is a Go service that exposes gRPC internally on `orders:50053` and
owns `orders-db`. It consumes retained ticket events from JetStream to maintain
its local ticket projection. The API gateway exposes authenticated
`GET /api/orders`, `GET /api/orders/:id`, `POST /api/orders`, and
`DELETE /api/orders/:id`. The list endpoint returns all orders
for the session user in descending expiration order. The create endpoint accepts
`{ "ticketId": "<uuid>" }`, creates a `Created`
order for the session user, and sets `expiresAt` to 15 minutes after creation.
It returns `404` when the ticket has not yet reached the Orders projection; it
does not read `tickets-db` or synchronously call Tickets. An active `Created`
or `AwaitingPayment` order reserves the ticket until expiry: a same-user retry
returns the existing order with `200`, while another user receives
`409 ALREADY_EXISTS`. `Canceled` releases the Orders reservation immediately;
Tickets removes its ticket edit lock asynchronously once it consumes the
cancellation event. `Complete` keeps the ticket unavailable permanently.

## Cancel Order Flow

1. A signed-in client calls `DELETE /api/orders/:id`.
2. The gateway obtains the session user ID and calls
   `orders.v1.OrdersService.CancelOrder` with it and the order ID.
3. Orders updates only an owned `Created` or `AwaitingPayment` order to
   `Canceled` and writes an `OrderCanceled` event to its outbox in the same
   transaction. Canceling an already canceled order succeeds unchanged and
   does not write a second event.
4. The gateway returns `200` with the canceled order. Missing and unowned
   orders return `404`; completed orders return `409 ALREADY_EXISTS`.

## Get Order Flow

1. A signed-in client calls `GET /api/orders/:id`.
2. The gateway obtains the session user ID and calls
   `orders.v1.OrdersService.GetOrder` with it and the order ID.
3. Orders retrieves the order only when both values match the same row.
4. The gateway returns `200`; missing and unowned orders both return `404`.

## List Orders Flow

1. A signed-in client calls `GET /api/orders`.
2. The gateway obtains the session user ID and calls
   `orders.v1.OrdersService.ListOrders` over gRPC.
3. Orders retrieves that user's orders, ordered by expiration time descending
   and then ID descending.
4. The gateway returns `200` with a JSON array of orders.

## List Tickets Flow

1. A client calls `GET /api/tickets` without pagination.
2. The gateway calls `tickets.v1.TicketsService.ListTickets` over gRPC.
3. Tickets retrieves all stored tickets ordered by title and ID.
4. The gateway returns `200` with a JSON array of tickets.

## Get Ticket Flow

1. A client calls `GET /api/tickets/:id`.
2. The gateway calls `tickets.v1.TicketsService.GetTicket` with the ticket ID.
3. Tickets retrieves the matching ticket.
4. The gateway returns `200`; missing tickets return `404`.

## Create Ticket Flow

1. A client calls `POST /api/tickets` with `title` and a positive integer
   `price` no larger than `9007199254740991`.
2. The gateway validates the request, resolves the current user from the Better
   Auth session, and calls `tickets.v1.TicketsService.CreateTicket` over gRPC.
3. Tickets validates the complete gRPC request and persists the ticket in
   `tickets-db`.
4. The gateway returns `201` with `id`, `title`, `price`, and `userId`.

## Update Ticket Flow

1. A client calls `PUT /api/tickets/:id` with the replacement `title` and a
   positive integer `price` no larger than `9007199254740991`.
2. The gateway validates the request, resolves the current user from the Better
   Auth session, and calls `tickets.v1.TicketsService.UpdateTicket` over gRPC.
3. Tickets validates the complete request and updates the ticket only when the
   supplied user ID owns it and the ticket has not been reserved by an order.
4. The gateway returns `200` with the updated ticket. Missing tickets return
   `404`; a non-owner or a reserved ticket receives `403 FORBIDDEN`.

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
| Backend service to backend service | NATS JetStream            | Ticket and order projections    |

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
to `protogen/ts` plus Go contracts to `protogen/go`. Identity, API gateway, and
tickets builds depend on this generation step.

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
- `ORDERS_DB_PASSWORD`
- `NATS_URL` (defaults to `nats://localhost:4222` when running tickets locally)

Common commands:

```bash
make help
make build
make test
make generate-proto
make serve-tickets
make docker-up
make docker-up-tools
```

The Makefile delegates to the existing Nx, Go, Buf, and Docker Compose
commands. Gateway integration tests use Testcontainers and require a working
Docker container runtime.

Local ports:

| Component         | Address                              |
| ----------------- | ------------------------------------ |
| API gateway       | `http://localhost:3000`              |
| Identity gRPC     | `identity:50051` within Compose only |
| Identity Postgres | `localhost:5432`                     |
| Tickets gRPC      | `tickets:50052` within Compose only  |
| Tickets Postgres  | `localhost:5433`                     |
| Orders Postgres   | `localhost:5434`                     |
| NATS JetStream    | `nats://localhost:4222`              |
| NATS monitoring   | `http://localhost:8222`              |
| NUI               | `http://localhost:31311`             |
| Mailpit inbox     | `http://localhost:8025`              |

NUI is optional local development tooling. Start it with `make docker-up-tools`,
then add a connection to `nats://nats:4222` from the NUI web interface. Its
configuration persists in the local `nui-data` Docker volume.

## Planned Services

| Service           | Technology        | Database               |
| ----------------- | ----------------- | ---------------------- |
| tickets           | Go                | `tickets-db`           |
| orders            | Go                | `orders-db`            |
| payments          | Go                | `payments-db`          |
| expiration        | NestJS and BullMQ | `expiration-db`        |
| concert-assistant | Python RAG        | `concert-assistant-db` |

Tickets publishes `tickets.ticket.created.v1` and `tickets.ticket.updated.v1`
events to the `TICKETS_EVENTS` JetStream stream. Ticket creation and updates
write their respective events to the tickets-owned Postgres outbox in the same
transaction as the ticket mutation, then a background dispatcher publishes them
at least once with bounded retry backoff. Consumers must be durable, explicitly
acknowledge messages, and deduplicate by `event_id`.

Tickets owns an internal `aggregate_version` for each ticket. It starts at `0`
when the ticket is created and increments after every successful owner ticket
update. Reservation and cancellation do not change it. The version is carried
by ticket events but is not exposed through the public ticket gRPC or HTTP API.

`tickets.ticket.created.v1` carries the protobuf
`tickets.v1.TicketCreated` payload. Its JSON representation is:

```json
{
  "eventId": "e6b565df-10dc-4a8e-baf7-12bed1e9e9d2",
  "occurredAt": "2026-08-25T15:42:18.123Z",
  "aggregateVersion": "0",
  "ticket": {
    "id": "8c91c1d3-910b-4dc4-b6f2-efb060b2a0ac",
    "title": "Metallica — Bogotá",
    "price": "10000",
    "userId": "user_01K..."
  }
}
```

JetStream receives protobuf binary, not JSON. The `int64` `price` is shown as
a string in protobuf JSON to preserve JavaScript integer precision.

`tickets.ticket.updated.v1` carries `tickets.v1.TicketUpdated`, which has the
same JSON shape and represents the ticket snapshot after the update with its
incremented `aggregateVersion`.

Orders consumes both ticket event subjects through its durable
`orders-ticket-projection-v1` JetStream consumer, replaying retained events to
maintain an orders-owned local `tickets` projection. Its `orders.ticket_id`
foreign key references that local table in `orders-db`, never `tickets-db`.
Orders applies a ticket event only when its `aggregateVersion` is contiguous:
version `0` creates a missing projection and later events must be exactly one
greater than the projected version. A future version gap is negatively
acknowledged so JetStream retries it; duplicate and delayed older snapshots are
acknowledged as no-ops.
The `orders` table has `id`, `expires_at`, `user_id`, `ticket_id`, and a
`status` enum with `Created`, `Canceled`, `AwaitingPayment`, and `Complete`.
Orders owns an internal `aggregate_version` that starts at `0` when the order
is created and increments after every successful order state transition. It is
carried by order events but is not exposed through the public order gRPC or
HTTP API.
Order creation locks the Orders-owned ticket projection while it checks and
creates a reservation, so concurrent callers cannot both reserve the ticket.
Expiration processing and payment remain future work.

Orders publishes `orders.order.created.v1` and `orders.order.canceled.v1`
events to the `ORDERS_EVENTS` JetStream stream. An order state change and its
event are written to the Orders-owned Postgres outbox in the same transaction;
the dispatcher publishes at least once with bounded retry backoff. The created
event carries an `event_id` and `occurred_at` delivery envelope, plus the
order and ticket reservation:

```json
{
  "eventId": "e6b565df-10dc-4a8e-baf7-12bed1e9e9d2",
  "occurredAt": "2026-08-26T15:42:18.123Z",
  "orderId": "8c91c1d3-910b-4dc4-b6f2-efb060b2a0ac",
  "orderStatus": "ORDER_STATUS_CREATED",
  "userId": "user_01K...",
  "expiresAt": "2026-08-26T15:57:18.123Z",
  "aggregateVersion": "0",
  "ticket": {
    "id": "7f301729-a359-4f4f-b71b-ea0a55b6ee71",
    "price": "10000"
  }
}
```

The cancellation event has the same delivery envelope and identifies the
canceled order and ticket:

```json
{
  "eventId": "e88c1272-0ff4-4f8f-bd1d-64f322ef7b6a",
  "occurredAt": "2026-08-26T15:43:02.456Z",
  "orderId": "8c91c1d3-910b-4dc4-b6f2-efb060b2a0ac",
  "aggregateVersion": "1",
  "ticket": {
    "id": "7f301729-a359-4f4f-b71b-ea0a55b6ee71"
  }
}
```

Tickets consumes the subjects through separate durable
`tickets-order-reservation-v1` and `tickets-order-cancellation-v1` consumers,
deduplicating `event_id` values in its own database. `OrderCreated` records its
order ID in `reserved_by_order_id`, which causes later ticket updates to return
`403 FORBIDDEN`. `OrderCanceled` clears that lock only when the same order owns
it. If cancellation arrives before creation, Tickets sends a delayed negative
acknowledgment so JetStream retries it after creation is processed. These
changes are asynchronous and take effect after event consumption.

Cancellation retries do not yet have a maximum-delivery policy, dead-letter
stream, or durable parking/reconciliation workflow. Those mechanisms are
required before cancellation messages whose matching reservation never arrives
can be discarded safely and remain future work.

Additional event subjects, consumers, CI/CD, observability, and deployment
environments remain open design and implementation work.
