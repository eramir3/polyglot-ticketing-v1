#!/bin/sh

set -eu

: "${CONCERT_ASSISTANT_READER_DB_PASSWORD:?CONCERT_ASSISTANT_READER_DB_PASSWORD must be set}"
: "${POSTGRES_DB:?POSTGRES_DB must be set}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}"
: "${POSTGRES_USER:?POSTGRES_USER must be set}"

export PGDATABASE="$POSTGRES_DB"
export PGHOST="127.0.0.1"
export PGPASSWORD="$POSTGRES_PASSWORD"
export PGUSER="$POSTGRES_USER"

psql --set=ON_ERROR_STOP=1 --set=reader_password="$CONCERT_ASSISTANT_READER_DB_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', 'concert_assistant_reader', :'reader_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'concert_assistant_reader')
\gexec

ALTER ROLE concert_assistant_reader PASSWORD :'reader_password';
GRANT CONNECT ON DATABASE "concert-assistant-db" TO concert_assistant_reader;
GRANT USAGE ON SCHEMA public TO concert_assistant_reader;
GRANT SELECT ON TABLE public.concerts TO concert_assistant_reader;
SQL
