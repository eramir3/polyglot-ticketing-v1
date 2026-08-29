ALTER TABLE payments DROP COLUMN tracestate, DROP COLUMN traceparent;
ALTER TABLE outbox_events DROP COLUMN tracestate, DROP COLUMN traceparent;
