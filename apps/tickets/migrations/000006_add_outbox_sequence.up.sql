ALTER TABLE outbox_events
  ADD COLUMN sequence BIGINT GENERATED ALWAYS AS IDENTITY;

DROP INDEX outbox_events_pending_idx;

CREATE INDEX outbox_events_pending_idx
  ON outbox_events (sequence)
  WHERE published_at IS NULL;
