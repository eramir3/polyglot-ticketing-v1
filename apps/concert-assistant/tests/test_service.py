from dataclasses import dataclass

import pytest

from concert_assistant.app import create_demo, create_planner
from concert_assistant.config import Settings
from concert_assistant.models import QueryPlan
from concert_assistant.service import ConcertAssistantService, UNSUPPORTED_ANSWER
from concert_assistant.sql_validation import SQLValidator
from concert_assistant.vector_repository import ArtistNotFoundError


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

    def synthesize_similarity(
        self, _question: str, _artist_name: str, _matches: list[dict[str, object]]
    ) -> str:
        if self.fail_synthesis:
            raise RuntimeError("Ollama unavailable")
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


@dataclass
class FakeArtistProfiles:
    matches: list[dict[str, object]]
    fail: bool = False
    received_artist: str | None = None

    def find_similar(self, artist_name: str) -> list[dict[str, object]]:
        self.received_artist = artist_name
        if self.fail:
            raise ArtistNotFoundError(artist_name)
        return self.matches


def make_service(
    planner: FakePlanner | None,
    repository: FakeRepository | None,
    artist_profiles: FakeArtistProfiles | None = None,
) -> ConcertAssistantService:
    return ConcertAssistantService(
        planner=planner,
        repository=repository,
        artist_profiles=artist_profiles or FakeArtistProfiles(matches=[{"artist": "Muse"}]),
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
    assert response.details == "SELECT COUNT(*) AS concert_count FROM concerts WHERE artist = 'Radiohead'"
    assert repository.received_sql is not None
    assert repository.received_sql.endswith("LIMIT 100")


def test_returns_semantic_data_limitation_for_unsupported_questions() -> None:
    planner = FakePlanner(
        QueryPlan(kind="unsupported", explanation="Venue descriptions are unavailable.")
    )

    response = make_service(planner, FakeRepository(rows=[])).answer("Find intimate venues", [])

    assert response.answer == UNSUPPORTED_ANSWER
    assert response.details is None


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

    assert response.details is None
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

    assert response.details is None
    assert response.answer == "I couldn't retrieve concert data right now. Please try again."


def test_reports_missing_openai_configuration() -> None:
    response = make_service(None, FakeRepository(rows=[])).answer("List artists", [])

    assert response.answer == "Concert Assistant is not configured with an OpenAI API key."


def test_creates_the_demo_with_plain_text_query_details() -> None:
    demo = create_demo(make_service(None, FakeRepository(rows=[])))

    assert demo is not None


def test_answers_artist_similarity_without_running_sql() -> None:
    planner = FakePlanner(
        QueryPlan(kind="artist_similarity", artist_name="Radiohead", explanation="Find touring-profile matches."),
        answer_text="Based on recorded touring history, Muse and Coldplay are the closest matches.",
    )
    repository = FakeRepository(rows=[])
    artist_profiles = FakeArtistProfiles(
        matches=[
            {"artist": "Muse", "profile_text": "...", "similarity": 0.91},
            {"artist": "Coldplay", "profile_text": "...", "similarity": 0.88},
        ]
    )

    response = make_service(planner, repository, artist_profiles).answer("Artists similar to Radiohead", [])

    assert response.answer.startswith("Based on recorded touring history")
    assert response.details == "Touring-profile similarity for Radiohead"
    assert artist_profiles.received_artist == "Radiohead"
    assert repository.received_sql is None


def test_reports_an_unknown_artist_similarity_source() -> None:
    planner = FakePlanner(
        QueryPlan(kind="artist_similarity", artist_name="Unknown Artist", explanation="Find touring-profile matches.")
    )
    artist_profiles = FakeArtistProfiles(matches=[], fail=True)

    response = make_service(planner, FakeRepository(rows=[]), artist_profiles).answer(
        "Artists similar to Unknown Artist", []
    )

    assert response.answer == "I couldn't find an indexed touring profile for Unknown Artist."
    assert response.details is None


def test_selects_ollama_without_an_openai_key(monkeypatch) -> None:
    created: dict[str, str] = {}

    class FakeOllamaPlanner:
        def __init__(self, base_url: str, model: str) -> None:
            created["base_url"] = base_url
            created["model"] = model

    monkeypatch.setattr("concert_assistant.app.OllamaConcertPlanner", FakeOllamaPlanner)
    settings = Settings(
        database_url=None,
        admin_database_url=None,
        llm_provider="ollama",
        openai_api_key=None,
        openai_model="gpt-4.1-mini",
        ollama_base_url="http://127.0.0.1:11434",
        ollama_model="qwen3:1.7b",
        ollama_embedding_model="nomic-embed-text",
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
        admin_database_url=None,
        llm_provider="openai",
        openai_api_key="test-key",
        openai_model="gpt-4.1-mini",
        ollama_base_url="http://127.0.0.1:11434",
        ollama_model="qwen3:1.7b",
        ollama_embedding_model="nomic-embed-text",
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
        admin_database_url=None,
        llm_provider="unknown",
        openai_api_key=None,
        openai_model="gpt-4.1-mini",
        ollama_base_url="http://127.0.0.1:11434",
        ollama_model="qwen3:1.7b",
        ollama_embedding_model="nomic-embed-text",
        server_host="0.0.0.0",
        server_port=7860,
        statement_timeout_ms=3000,
        max_result_rows=100,
        history_messages=6,
    )

    with pytest.raises(ValueError, match="LLM_PROVIDER"):
        create_planner(settings)
