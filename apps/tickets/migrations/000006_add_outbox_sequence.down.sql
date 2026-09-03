DROP INDEX outbox_events_pending_idx;

ALTER TABLE outbox_events DROP COLUMN sequence;

CREATE INDEX outbox_events_pending_idx
  ON outbox_events (created_at)
  WHERE published_at IS NULL;
