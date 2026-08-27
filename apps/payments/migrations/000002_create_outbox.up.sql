CREATE TABLE outbox_events (
  event_id UUID PRIMARY KEY,
  subject TEXT NOT NULL,
  payload BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  next_attempt_at TIMESTAMPTZ,
  locked_until TIMESTAMPTZ
);

CREATE INDEX outbox_events_pending_idx
  ON outbox_events (created_at)
  WHERE published_at IS NULL;
