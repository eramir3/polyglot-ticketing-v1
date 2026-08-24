CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tickets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL,
  price BIGINT NOT NULL CHECK (price > 0),
  user_id TEXT NOT NULL
);
