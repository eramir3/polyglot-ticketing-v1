"""Read-only PostgreSQL access for validated concert queries."""

from __future__ import annotations

import psycopg
from psycopg.rows import dict_row


class ConcertRepository:
    def __init__(self, database_url: str, statement_timeout_ms: int) -> None:
        self._database_url = database_url
        self._statement_timeout_ms = statement_timeout_ms

    def query(self, sql: str) -> list[dict[str, object]]:
        with psycopg.connect(self._database_url, row_factory=dict_row) as connection:
            connection.set_read_only(True)
            with connection.cursor() as cursor:
                cursor.execute(
                    "SELECT set_config('statement_timeout', %s, true)",
                    (f"{self._statement_timeout_ms}ms",),
                )
                cursor.execute(sql)
                return list(cursor.fetchall())
