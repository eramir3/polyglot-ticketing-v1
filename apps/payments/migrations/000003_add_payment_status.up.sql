CREATE TYPE payment_status AS ENUM (
  'Pending',
  'Succeeded',
  'Failed'
);

ALTER TABLE payments
  ADD COLUMN status payment_status NOT NULL DEFAULT 'Pending';
