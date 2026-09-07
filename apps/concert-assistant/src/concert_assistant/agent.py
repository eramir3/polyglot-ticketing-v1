"""LLM planning and synthesis for structured concert-data questions."""

from __future__ import annotations

import json
from typing import Protocol

from ollama import Client
from openai import OpenAI

from concert_assistant.models import QueryPlan
from concert_assistant.schema import schema_prompt


class ConcertPlanner(Protocol):
    def plan(self, question: str, history: list[dict[str, str]]) -> QueryPlan: ...

    def synthesize(self, question: str, sql: str, rows: list[dict[str, object]]) -> str: ...


PLANNING_INSTRUCTIONS = (
    "You are Concert Assistant's analytical query planner. "
    "Return kind='analytical' only when the question can be answered from the structured "
    "concerts relation. Produce PostgreSQL SELECT SQL using only the supplied schema. "
    "Return kind='unsupported' for semantic or fuzzy requests requiring venue descriptions, "
    "embeddings, external knowledge, or unavailable data. Never write, modify, or inspect "
    "database metadata.\n\n"
    f"{schema_prompt()}"
)

SYNTHESIS_INSTRUCTIONS = (
    "Answer the user's concert-data question using only the supplied PostgreSQL result rows. "
    "Be concise, state when no rows matched, and do not invent facts."
)


class OpenAIConcertPlanner:
    def __init__(self, api_key: str, model: str) -> None:
        self._client = OpenAI(api_key=api_key)
        self._model = model

    def plan(self, question: str, history: list[dict[str, str]]) -> QueryPlan:
        response = self._client.responses.parse(
            model=self._model,
            instructions=PLANNING_INSTRUCTIONS,
            input=json.dumps({"history": history, "question": question}),
            text_format=QueryPlan,
        )
        print("response!!!", response)
        for output in response.output:
            for content in output.content:
                parsed = getattr(content, "parsed", None)
                if isinstance(parsed, QueryPlan):
                    return parsed
        raise RuntimeError("OpenAI did not return a structured query plan.")

    def synthesize(self, question: str, sql: str, rows: list[dict[str, object]]) -> str:
        response = self._client.responses.create(
            model=self._model,
            instructions=SYNTHESIS_INSTRUCTIONS,
            input=json.dumps({"question": question, "sql": sql, "rows": rows}, default=str),
        )
        answer = response.output_text.strip()
        if not answer:
            raise RuntimeError("OpenAI returned an empty answer.")
        return answer


class OllamaConcertPlanner:
    """Local Ollama implementation using schema-constrained query plans."""

    def __init__(self, base_url: str, model: str) -> None:
        self._client = Client(host=base_url)
        self._model = model

    def plan(self, question: str, history: list[dict[str, str]]) -> QueryPlan:
        response = self._client.chat(
            model=self._model,
            messages=[
                {"role": "system", "content": PLANNING_INSTRUCTIONS},
                {"role": "user", "content": json.dumps({"history": history, "question": question})},
            ],
            format=QueryPlan.model_json_schema(),
            options={"temperature": 0},
            stream=False,
            think=False,
        )
        content = response.message.content
        if not content:
            raise RuntimeError("Ollama returned an empty query plan.")
        return QueryPlan.model_validate_json(content)

    def synthesize(self, question: str, sql: str, rows: list[dict[str, object]]) -> str:
        response = self._client.chat(
            model=self._model,
            messages=[
                {"role": "system", "content": SYNTHESIS_INSTRUCTIONS},
                {
                    "role": "user",
                    "content": json.dumps({"question": question, "sql": sql, "rows": rows}, default=str),
                },
            ],
            stream=False,
            think=False,
        )
        answer = response.message.content.strip()
        if not answer:
            raise RuntimeError("Ollama returned an empty answer.")
        return answer
