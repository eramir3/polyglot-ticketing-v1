"""Typed boundaries between the LLM, validator, database, and UI."""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field, model_validator


class QueryPlan(BaseModel):
    kind: Literal["analytical", "artist_similarity", "unsupported"]
    sql: str | None = None
    artist_name: str | None = None
    explanation: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_sql_presence(self) -> "QueryPlan":
        if self.kind == "analytical":
            if not self.sql or self.artist_name is not None:
                raise ValueError("analytical plans require SQL only")
        elif self.kind == "artist_similarity":
            if not self.artist_name or self.sql is not None:
                raise ValueError("artist similarity plans require an artist name only")
        elif self.sql is not None or self.artist_name is not None:
            raise ValueError("unsupported plans must not include a query")
        return self


class ValidatedQuery(BaseModel):
    display_sql: str
    execution_sql: str


class AssistantResponse(BaseModel):
    answer: str
    details: str | None = None
