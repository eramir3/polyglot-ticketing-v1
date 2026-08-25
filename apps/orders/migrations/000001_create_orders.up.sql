CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE order_status AS ENUM (
  'Created',
  'Canceled',
  'AwaitingPayment',
  'Complete'
);

CREATE TABLE tickets (
  id UUID PRIMARY KEY,
  title TEXT NOT NULL,
  price BIGINT NOT NULL CHECK (price > 0)
);

CREATE TABLE orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  expires_at TIMESTAMPTZ NOT NULL,
  user_id TEXT NOT NULL,
  ticket_id UUID NOT NULL REFERENCES tickets (id),
  status order_status NOT NULL DEFAULT 'Created'
);

CREATE TABLE processed_events (
  event_id UUID PRIMARY KEY,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
