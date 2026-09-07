"""Application orchestration and user-safe failures."""

from __future__ import annotations

import logging

from concert_assistant.agent import ConcertPlanner
from concert_assistant.models import AssistantResponse
from concert_assistant.repository import ConcertRepository
from concert_assistant.sql_validation import SQLValidationError, SQLValidator


logger = logging.getLogger(__name__)

UNSUPPORTED_ANSWER = (
    "I can currently answer analytical questions about the structured concert data. "
    "This dataset does not include descriptive venue or concert text for semantic search yet."
)


class ConcertAssistantService:
    def __init__(
        self,
        planner: ConcertPlanner | None,
        repository: ConcertRepository | None,
        validator: SQLValidator,
        history_messages: int,
    ) -> None:
        self._planner = planner
        self._repository = repository
        self._validator = validator
        self._history_messages = history_messages

    def answer(self, question: str, history: list[dict[str, str]]) -> AssistantResponse:
        if self._planner is None:
            return AssistantResponse(answer="Concert Assistant is not configured with an OpenAI API key.")
        if self._repository is None:
            return AssistantResponse(answer="Concert Assistant is not configured with a database connection.")

        try:
            plan = self._planner.plan(question, history[-self._history_messages :])
        except Exception:
            logger.exception("concert query planning failed")
            return AssistantResponse(answer="I couldn't interpret that question right now. Please try again.")

        if plan.kind == "unsupported":
            return AssistantResponse(answer=UNSUPPORTED_ANSWER)

        try:
            validated = self._validator.validate(plan.sql or "")
        except SQLValidationError:
            logger.warning("concert query rejected by SQL validation")
            return AssistantResponse(answer="I couldn't safely form a supported analytical query for that question.")

        try:
            rows = self._repository.query(validated.execution_sql)
        except Exception:
            logger.exception("concert query execution failed")
            return AssistantResponse(answer="I couldn't retrieve concert data right now. Please try again.")

        try:
            answer = self._planner.synthesize(question, validated.display_sql, rows)
        except Exception:
            logger.exception("concert answer synthesis failed")
            return AssistantResponse(answer="I found matching data but couldn't summarize it right now.", sql=validated.display_sql)
        return AssistantResponse(answer=answer, sql=validated.display_sql)
