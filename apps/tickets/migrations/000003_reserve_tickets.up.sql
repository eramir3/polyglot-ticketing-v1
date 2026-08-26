ALTER TABLE tickets
  ADD COLUMN reserved_by_order_id UUID;

CREATE TABLE processed_events (
  event_id UUID PRIMARY KEY,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
