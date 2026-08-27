ALTER TABLE orders
  ADD COLUMN aggregate_version BIGINT NOT NULL DEFAULT 0
  CHECK (aggregate_version >= 0);
