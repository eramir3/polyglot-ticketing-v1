CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE payment_order_status AS ENUM (
  'Created',
  'Canceled',
  'AwaitingPayment',
  'Complete'
);

CREATE TABLE orders (
  id UUID PRIMARY KEY,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version >= 0),
  user_id TEXT NOT NULL,
  price BIGINT NOT NULL CHECK (price > 0),
  status payment_order_status NOT NULL
);

CREATE TABLE payments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL UNIQUE REFERENCES orders (id)
);

CREATE TABLE processed_events (
  event_id UUID PRIMARY KEY,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
