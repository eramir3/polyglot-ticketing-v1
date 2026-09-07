#!/bin/sh

set -eu

: "${POSTGRES_DB:?POSTGRES_DB must be set}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}"
: "${POSTGRES_USER:?POSTGRES_USER must be set}"

export PGDATABASE="$POSTGRES_DB"
export PGHOST="127.0.0.1"
export PGPASSWORD="$POSTGRES_PASSWORD"
export PGUSER="$POSTGRES_USER"

pg_restore \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  --exit-on-error \
  --single-transaction \
  --dbname="$PGDATABASE" \
  /restore/concerts.dump

sh /restore/provision-reader.sh
