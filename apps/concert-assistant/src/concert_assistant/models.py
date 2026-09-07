"""Typed boundaries between the LLM, validator, database, and UI."""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field, model_validator


class QueryPlan(BaseModel):
    kind: Literal["analytical", "unsupported"]
    sql: str | None = None
    explanation: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_sql_presence(self) -> "QueryPlan":
        if self.kind == "analytical" and not self.sql:
            raise ValueError("analytical plans require SQL")
        if self.kind == "unsupported" and self.sql is not None:
            raise ValueError("unsupported plans must not include SQL")
        return self


class ValidatedQuery(BaseModel):
    display_sql: str
    execution_sql: str


class AssistantResponse(BaseModel):
    answer: str
    sql: str | None = None
