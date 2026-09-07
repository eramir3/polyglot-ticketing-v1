"""Read-only nearest-neighbor retrieval for artist touring profiles."""

from __future__ import annotations

import psycopg
from psycopg.rows import dict_row


class ArtistNotFoundError(LookupError):
    """The requested source artist has no indexed touring profile."""


class ArtistProfileRepository:
    def __init__(self, database_url: str, statement_timeout_ms: int) -> None:
        self._database_url = database_url
        self._statement_timeout_ms = statement_timeout_ms

    def find_similar(self, artist_name: str, limit: int = 5) -> list[dict[str, object]]:
        with psycopg.connect(self._database_url, row_factory=dict_row) as connection:
            connection.set_read_only(True)
            with connection.cursor() as cursor:
                cursor.execute(
                    "SELECT set_config('statement_timeout', %s, true)",
                    (f"{self._statement_timeout_ms}ms",),
                )
                cursor.execute(
                    """
                    WITH source AS (
                        SELECT artist, embedding
                        FROM public.artist_profiles
                        WHERE lower(artist) = lower(%s)
                    )
                    SELECT profile.artist,
                           profile.profile_text,
                           1 - (profile.embedding <=> source.embedding) AS similarity
                    FROM public.artist_profiles AS profile
                    CROSS JOIN source
                    WHERE lower(profile.artist) <> lower(source.artist)
                    ORDER BY profile.embedding <=> source.embedding
                    LIMIT %s
                    """,
                    (artist_name, limit),
                )
                matches = list(cursor.fetchall())
        if not matches:
            raise ArtistNotFoundError(artist_name)
        return matches
