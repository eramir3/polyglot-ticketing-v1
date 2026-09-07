from dataclasses import dataclass, field

import pytest

from concert_assistant.vector_repository import ArtistNotFoundError, ArtistProfileRepository


@dataclass
class FakeCursor:
    rows: list[dict[str, object]]
    commands: list[tuple[str, tuple[object, ...]]] = field(default_factory=list)

    def __enter__(self) -> "FakeCursor":
        return self

    def __exit__(self, *_args: object) -> None:
        return None

    def execute(self, sql: str, parameters: tuple[object, ...]) -> None:
        self.commands.append((sql, parameters))

    def fetchall(self) -> list[dict[str, object]]:
        return self.rows


@dataclass
class FakeConnection:
    cursor_instance: FakeCursor
    read_only: bool = False

    def __enter__(self) -> "FakeConnection":
        return self

    def __exit__(self, *_args: object) -> None:
        return None

    def set_read_only(self, value: bool) -> None:
        self.read_only = value

    def cursor(self) -> FakeCursor:
        return self.cursor_instance


def test_retrieves_nearest_profiles_and_excludes_the_source(monkeypatch) -> None:
    cursor = FakeCursor(rows=[{"artist": "Muse", "profile_text": "...", "similarity": 0.91}])
    connection = FakeConnection(cursor)
    monkeypatch.setattr("concert_assistant.vector_repository.psycopg.connect", lambda *_args, **_kwargs: connection)

    matches = ArtistProfileRepository("postgresql://reader@localhost/concerts", 3000).find_similar("Radiohead")

    assert matches == [{"artist": "Muse", "profile_text": "...", "similarity": 0.91}]
    assert connection.read_only is True
    assert cursor.commands[0] == ("SELECT set_config('statement_timeout', %s, true)", ("3000ms",))
    similarity_sql, parameters = cursor.commands[1]
    assert "profile.embedding <=> source.embedding" in similarity_sql
    assert "lower(profile.artist) <> lower(source.artist)" in similarity_sql
    assert parameters == ("Radiohead", 5)


def test_reports_a_missing_source_artist(monkeypatch) -> None:
    cursor = FakeCursor(rows=[])
    connection = FakeConnection(cursor)
    monkeypatch.setattr("concert_assistant.vector_repository.psycopg.connect", lambda *_args, **_kwargs: connection)

    with pytest.raises(ArtistNotFoundError):
        ArtistProfileRepository("postgresql://reader@localhost/concerts", 3000).find_similar("Unknown Artist")
