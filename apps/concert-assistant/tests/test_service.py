from dataclasses import dataclass

import pytest

from concert_assistant.app import create_planner
from concert_assistant.config import Settings
from concert_assistant.models import QueryPlan
from concert_assistant.service import ConcertAssistantService, UNSUPPORTED_ANSWER
from concert_assistant.sql_validation import SQLValidator


@dataclass
class FakePlanner:
    query_plan: QueryPlan
    answer_text: str = "Radiohead played 12 concerts in France."
    fail_synthesis: bool = False

    def plan(self, _question: str, _history: list[dict[str, str]]) -> QueryPlan:
        return self.query_plan

    def synthesize(self, _question: str, _sql: str, _rows: list[dict[str, object]]) -> str:
        if self.fail_synthesis:
            raise RuntimeError("OpenAI unavailable")
        return self.answer_text


@dataclass
class FakeRepository:
    rows: list[dict[str, object]]
    fail: bool = False
    received_sql: str | None = None

    def query(self, sql: str) -> list[dict[str, object]]:
        self.received_sql = sql
        if self.fail:
            raise RuntimeError("database unavailable")
        return self.rows


def make_service(planner: FakePlanner | None, repository: FakeRepository | None) -> ConcertAssistantService:
    return ConcertAssistantService(
        planner=planner,
        repository=repository,
        validator=SQLValidator(max_result_rows=100),
        history_messages=6,
    )


def test_answers_analytical_question_and_returns_validated_sql() -> None:
    planner = FakePlanner(
        QueryPlan(
            kind="analytical",
            sql="SELECT COUNT(*) AS concert_count FROM concerts WHERE artist = 'Radiohead'",
            explanation="Counts Radiohead rows.",
        )
    )
    repository = FakeRepository(rows=[{"concert_count": 12}])

    response = make_service(planner, repository).answer("How many Radiohead concerts?", [])

    assert response.answer == "Radiohead played 12 concerts in France."
    assert response.sql == "SELECT COUNT(*) AS concert_count FROM concerts WHERE artist = 'Radiohead'"
    assert repository.received_sql is not None
    assert repository.received_sql.endswith("LIMIT 100")


def test_returns_semantic_data_limitation_for_unsupported_questions() -> None:
    planner = FakePlanner(
        QueryPlan(kind="unsupported", explanation="Venue descriptions are unavailable.")
    )

    response = make_service(planner, FakeRepository(rows=[])).answer("Find intimate venues", [])

    assert response.answer == UNSUPPORTED_ANSWER
    assert response.sql is None


def test_does_not_execute_rejected_sql() -> None:
    planner = FakePlanner(
        QueryPlan(
            kind="analytical",
            sql="DELETE FROM concerts",
            explanation="Invalid operation.",
        )
    )
    repository = FakeRepository(rows=[])

    response = make_service(planner, repository).answer("Delete a concert", [])

    assert response.sql is None
    assert "safely form" in response.answer
    assert repository.received_sql is None


def test_hides_database_failure() -> None:
    planner = FakePlanner(
        QueryPlan(
            kind="analytical",
            sql="SELECT artist FROM concerts",
            explanation="Lists artists.",
        )
    )

    response = make_service(planner, FakeRepository(rows=[], fail=True)).answer("List artists", [])

    assert response.sql is None
    assert response.answer == "I couldn't retrieve concert data right now. Please try again."


def test_reports_missing_openai_configuration() -> None:
    response = make_service(None, FakeRepository(rows=[])).answer("List artists", [])

    assert response.answer == "Concert Assistant is not configured with an OpenAI API key."


def test_selects_ollama_without_an_openai_key(monkeypatch) -> None:
    created: dict[str, str] = {}

    class FakeOllamaPlanner:
        def __init__(self, base_url: str, model: str) -> None:
            created["base_url"] = base_url
            created["model"] = model

    monkeypatch.setattr("concert_assistant.app.OllamaConcertPlanner", FakeOllamaPlanner)
    settings = Settings(
        database_url=None,
        llm_provider="ollama",
        openai_api_key=None,
        openai_model="gpt-4.1-mini",
        ollama_base_url="http://127.0.0.1:11434",
        ollama_model="qwen3:1.7b",
        server_host="0.0.0.0",
        server_port=7860,
        statement_timeout_ms=3000,
        max_result_rows=100,
        history_messages=6,
    )

    create_planner(settings)

    assert created == {"base_url": "http://127.0.0.1:11434", "model": "qwen3:1.7b"}


def test_selects_openai_by_default_when_an_api_key_is_present(monkeypatch) -> None:
    created: dict[str, str] = {}

    class FakeOpenAIPlanner:
        def __init__(self, api_key: str, model: str) -> None:
            created["api_key"] = api_key
            created["model"] = model

    monkeypatch.setattr("concert_assistant.app.OpenAIConcertPlanner", FakeOpenAIPlanner)
    settings = Settings(
        database_url=None,
        llm_provider="openai",
        openai_api_key="test-key",
        openai_model="gpt-4.1-mini",
        ollama_base_url="http://127.0.0.1:11434",
        ollama_model="qwen3:1.7b",
        server_host="0.0.0.0",
        server_port=7860,
        statement_timeout_ms=3000,
        max_result_rows=100,
        history_messages=6,
    )

    create_planner(settings)

    assert created == {"api_key": "test-key", "model": "gpt-4.1-mini"}


def test_rejects_an_unknown_llm_provider() -> None:
    settings = Settings(
        database_url=None,
        llm_provider="unknown",
        openai_api_key=None,
        openai_model="gpt-4.1-mini",
        ollama_base_url="http://127.0.0.1:11434",
        ollama_model="qwen3:1.7b",
        server_host="0.0.0.0",
        server_port=7860,
        statement_timeout_ms=3000,
        max_result_rows=100,
        history_messages=6,
    )

    with pytest.raises(ValueError, match="LLM_PROVIDER"):
        create_planner(settings)
