from dataclasses import dataclass, field

from concert_assistant.repository import ConcertRepository


@dataclass
class FakeCursor:
    commands: list[tuple[str, tuple[str, ...] | None]] = field(default_factory=list)

    def __enter__(self) -> "FakeCursor":
        return self

    def __exit__(self, *_args: object) -> None:
        return None

    def execute(self, sql: str, parameters: tuple[str, ...] | None = None) -> None:
        self.commands.append((sql, parameters))

    def fetchall(self) -> list[dict[str, object]]:
        return [{"concert_count": 12}]


@dataclass
class FakeConnection:
    cursor_instance: FakeCursor = field(default_factory=FakeCursor)
    read_only: bool = False

    def __enter__(self) -> "FakeConnection":
        return self

    def __exit__(self, *_args: object) -> None:
        return None

    def set_read_only(self, value: bool) -> None:
        self.read_only = value

    def cursor(self) -> FakeCursor:
        return self.cursor_instance


def test_uses_a_parameter_safe_local_statement_timeout(monkeypatch) -> None:
    connection = FakeConnection()
    monkeypatch.setattr("concert_assistant.repository.psycopg.connect", lambda *_args, **_kwargs: connection)

    rows = ConcertRepository("postgresql://reader@localhost/concerts", 3000).query(
        "SELECT COUNT(*) AS concert_count FROM concerts"
    )

    assert rows == [{"concert_count": 12}]
    assert connection.read_only is True
    assert connection.cursor_instance.commands == [
        ("SELECT set_config('statement_timeout', %s, true)", ("3000ms",)),
        ("SELECT COUNT(*) AS concert_count FROM concerts", None),
    ]
