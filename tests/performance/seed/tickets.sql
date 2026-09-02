BEGIN;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM tickets) THEN
    RAISE EXCEPTION 'tickets-db is not empty; run docker-reset or use a fresh database before seeding performance tickets';
  END IF;
END
$$;

INSERT INTO tickets (id, title, price, user_id, aggregate_version)
SELECT
  (
    '00000000-0000-4000-8000-' ||
    lpad(ticket_number::text, 12, '0')
  )::uuid,
  'performance-ticket-' || lpad(ticket_number::text, 3, '0'),
  10000 + ticket_number,
  'performance-user',
  0
FROM generate_series(1, 100) AS ticket_number;

COMMIT;
