"""Build the owner-managed pgvector projection for artist touring profiles."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import date
import hashlib
import logging

from ollama import Client
from pgvector import Vector
from pgvector.psycopg import register_vector
import psycopg
from psycopg.rows import dict_row

from concert_assistant.config import Settings


logger = logging.getLogger(__name__)

EMBEDDING_DIMENSIONS = 768

PROFILE_SCHEMA_SQL = """
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS public.artist_profiles (
    artist text PRIMARY KEY,
    profile_text text NOT NULL,
    embedding vector(768) NOT NULL,
    embedding_model text NOT NULL,
    source_fingerprint text NOT NULL,
    refreshed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS artist_profiles_embedding_hnsw_idx
    ON public.artist_profiles USING hnsw (embedding vector_cosine_ops);

GRANT SELECT ON TABLE public.artist_profiles TO concert_assistant_reader;
"""

ARTIST_PROFILE_QUERY = """
SELECT artist,
       COUNT(*) AS concert_count,
       MIN(date) AS first_concert,
       MAX(date) AS last_concert,
       COUNT(DISTINCT country) AS country_count,
       COUNT(DISTINCT city) AS city_count,
       COUNT(*) FILTER (WHERE is_festival) AS festival_count,
       string_agg(DISTINCT country, ', ' ORDER BY country) AS countries,
       string_agg(DISTINCT season, ', ' ORDER BY season) AS seasons,
       AVG(days_since_previous_concert) FILTER (WHERE days_since_previous_concert IS NOT NULL) AS average_gap_days,
       AVG(concerts_last_year) AS average_concerts_last_year,
       BOOL_OR(is_currently_touring) AS observed_currently_touring
FROM public.concerts
GROUP BY artist
ORDER BY artist
"""


@dataclass(frozen=True)
class ArtistProfile:
    artist: str
    profile_text: str
    source_fingerprint: str


def build_profile(row: dict[str, object]) -> ArtistProfile:
    artist = str(row["artist"])
    concert_count = int(row["concert_count"])
    festival_count = int(row["festival_count"])
    festival_percentage = round((festival_count / concert_count) * 100)
    first_concert = row["first_concert"]
    last_concert = row["last_concert"]
    if not isinstance(first_concert, date) or not isinstance(last_concert, date):
        raise ValueError(f"Artist profile dates are invalid for {artist}.")

    average_gap = row["average_gap_days"]
    average_last_year = row["average_concerts_last_year"]
    profile_text = "\n".join(
        [
            f"Artist: {artist}",
            f"Recorded touring history: {concert_count} concerts from {first_concert.isoformat()} to {last_concert.isoformat()}.",
            f"Geographic footprint: {row['country_count']} countries and {row['city_count']} cities.",
            f"Countries visited: {row['countries'] or 'not recorded'}.",
            f"Festival appearances: {festival_count} ({festival_percentage}% of recorded concerts).",
            f"Seasons represented: {row['seasons'] or 'not recorded'}.",
            f"Average gap between recorded concerts: {float(average_gap):.1f} days."
            if average_gap is not None
            else "Average gap between recorded concerts: not available.",
            f"Average concerts in the preceding year: {float(average_last_year):.1f}."
            if average_last_year is not None
            else "Average concerts in the preceding year: not available.",
            "Observed as currently touring in the source data."
            if row["observed_currently_touring"]
            else "Not observed as currently touring in the source data.",
        ]
    )
    return ArtistProfile(
        artist=artist,
        profile_text=profile_text,
        source_fingerprint=hashlib.sha256(profile_text.encode()).hexdigest(),
    )


class ArtistProfileIndexer:
    def __init__(self, database_url: str, ollama_base_url: str, embedding_model: str) -> None:
        self._database_url = database_url
        self._ollama = Client(host=ollama_base_url)
        self._embedding_model = embedding_model

    def index(self) -> int:
        with psycopg.connect(self._database_url, row_factory=dict_row) as connection:
            with connection.cursor() as schema_cursor:
                schema_cursor.execute(PROFILE_SCHEMA_SQL)
            connection.commit()
            register_vector(connection)

            with connection.cursor() as cursor:
                cursor.execute(ARTIST_PROFILE_QUERY)
                profiles = [build_profile(row) for row in cursor.fetchall()]

                response = self._ollama.embed(
                    model=self._embedding_model,
                    input=[profile.profile_text for profile in profiles],
                )
                embeddings = response.embeddings
                validate_embeddings(profiles, embeddings, self._embedding_model)

                for profile, embedding in zip(profiles, embeddings, strict=True):
                    cursor.execute(
                        """
                        INSERT INTO public.artist_profiles
                            (artist, profile_text, embedding, embedding_model, source_fingerprint, refreshed_at)
                        VALUES (%s, %s, %s, %s, %s, now())
                        ON CONFLICT (artist) DO UPDATE SET
                            profile_text = EXCLUDED.profile_text,
                            embedding = EXCLUDED.embedding,
                            embedding_model = EXCLUDED.embedding_model,
                            source_fingerprint = EXCLUDED.source_fingerprint,
                            refreshed_at = EXCLUDED.refreshed_at
                        """,
                        (
                            profile.artist,
                            profile.profile_text,
                            Vector(embedding),
                            self._embedding_model,
                            profile.source_fingerprint,
                        ),
                    )
                cursor.execute(
                    "DELETE FROM public.artist_profiles WHERE NOT (artist = ANY(%s))",
                    ([profile.artist for profile in profiles],),
                )
                connection.commit()
        return len(profiles)


def validate_embeddings(
    profiles: list[ArtistProfile], embeddings: list[list[float]], embedding_model: str
) -> None:
    if len(embeddings) != len(profiles):
        raise RuntimeError("Ollama returned a different number of artist embeddings.")
    if any(len(embedding) != EMBEDDING_DIMENSIONS for embedding in embeddings):
        raise RuntimeError(f"{embedding_model} must return {EMBEDDING_DIMENSIONS}-dimension embeddings.")


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    settings = Settings.from_environment()
    if not settings.admin_database_url:
        raise ValueError("CONCERT_ASSISTANT_ADMIN_DATABASE_URL is required to index artist profiles.")
    count = ArtistProfileIndexer(
        settings.admin_database_url,
        settings.ollama_base_url,
        settings.ollama_embedding_model,
    ).index()
    logger.info("indexed %s artist touring profiles", count)
