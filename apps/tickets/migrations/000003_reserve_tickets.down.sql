DROP TABLE processed_events;

ALTER TABLE tickets
  DROP COLUMN reserved_by_order_id;
