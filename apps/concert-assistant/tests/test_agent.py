from types import SimpleNamespace
from unittest.mock import ANY

import pytest

from concert_assistant.agent import OllamaConcertPlanner


class FakeOllamaClient:
    def __init__(self, responses: list[str]) -> None:
        self._responses = iter(responses)
        self.calls: list[dict[str, object]] = []

    def chat(self, **kwargs: object) -> SimpleNamespace:
        self.calls.append(kwargs)
        return SimpleNamespace(message=SimpleNamespace(content=next(self._responses)))


def test_ollama_plans_with_a_json_schema_and_thinking_disabled(monkeypatch) -> None:
    fake_client = FakeOllamaClient(
        ['{"kind":"analytical","sql":"SELECT COUNT(*) AS concert_count FROM concerts","explanation":"Counts concerts."}']
    )
    monkeypatch.setattr("concert_assistant.agent.Client", lambda **_kwargs: fake_client)

    planner = OllamaConcertPlanner("http://127.0.0.1:11434", "qwen3:1.7b")
    plan = planner.plan("How many concerts?", [{"role": "user", "content": "Earlier question"}])

    assert plan.kind == "analytical"
    assert plan.sql == "SELECT COUNT(*) AS concert_count FROM concerts"
    assert fake_client.calls == [
        {
            "model": "qwen3:1.7b",
            "messages": [
                {
                    "role": "system",
                    "content": ANY,
                },
                {
                    "role": "user",
                    "content": '{"history": [{"role": "user", "content": "Earlier question"}], "question": "How many concerts?"}',
                },
            ],
            "format": ANY,
            "options": {"temperature": 0},
            "stream": False,
            "think": False,
        }
    ]


def test_ollama_synthesizes_a_non_empty_answer(monkeypatch) -> None:
    fake_client = FakeOllamaClient(["Radiohead played 12 concerts in France."])
    monkeypatch.setattr("concert_assistant.agent.Client", lambda **_kwargs: fake_client)

    answer = OllamaConcertPlanner("http://127.0.0.1:11434", "qwen3:1.7b").synthesize(
        "How many Radiohead concerts?",
        "SELECT COUNT(*) AS concert_count FROM concerts WHERE artist = 'Radiohead'",
        [{"concert_count": 12}],
    )

    assert answer == "Radiohead played 12 concerts in France."
    assert fake_client.calls[0]["think"] is False
    assert fake_client.calls[0]["stream"] is False


def test_ollama_rejects_empty_response(monkeypatch) -> None:
    fake_client = FakeOllamaClient([""])
    monkeypatch.setattr("concert_assistant.agent.Client", lambda **_kwargs: fake_client)

    with pytest.raises(RuntimeError, match="empty query plan"):
        OllamaConcertPlanner("http://127.0.0.1:11434", "qwen3:1.7b").plan("Question", [])


def test_ollama_parses_an_artist_similarity_plan(monkeypatch) -> None:
    fake_client = FakeOllamaClient(
        ['{"kind":"artist_similarity","artist_name":"Radiohead","explanation":"Compare touring profiles."}']
    )
    monkeypatch.setattr("concert_assistant.agent.Client", lambda **_kwargs: fake_client)

    plan = OllamaConcertPlanner("http://127.0.0.1:11434", "qwen3:1.7b").plan(
        "Artists similar to Radiohead", []
    )

    assert plan.kind == "artist_similarity"
    assert plan.artist_name == "Radiohead"
