"""Application orchestration and user-safe failures."""

from __future__ import annotations

import logging

from concert_assistant.agent import ConcertPlanner
from concert_assistant.models import AssistantResponse
from concert_assistant.repository import ConcertRepository
from concert_assistant.sql_validation import SQLValidationError, SQLValidator
from concert_assistant.vector_repository import ArtistNotFoundError, ArtistProfileRepository


logger = logging.getLogger(__name__)

UNSUPPORTED_ANSWER = (
    "I can currently answer analytical questions and compare artists by recorded touring history. "
    "This dataset does not include descriptive venue or concert text for broader semantic search yet."
)


class ConcertAssistantService:
    def __init__(
        self,
        planner: ConcertPlanner | None,
        repository: ConcertRepository | None,
        artist_profiles: ArtistProfileRepository | None,
        validator: SQLValidator,
        history_messages: int,
    ) -> None:
        self._planner = planner
        self._repository = repository
        self._artist_profiles = artist_profiles
        self._validator = validator
        self._history_messages = history_messages

    def answer(self, question: str, history: list[dict[str, str]]) -> AssistantResponse:
        if self._planner is None:
            return AssistantResponse(answer="Concert Assistant is not configured with an OpenAI API key.")
        if self._repository is None or self._artist_profiles is None:
            return AssistantResponse(answer="Concert Assistant is not configured with a database connection.")

        try:
            plan = self._planner.plan(question, history[-self._history_messages :])
        except Exception:
            logger.exception("concert query planning failed")
            return AssistantResponse(answer="I couldn't interpret that question right now. Please try again.")

        if plan.kind == "unsupported":
            return AssistantResponse(answer=UNSUPPORTED_ANSWER)

        if plan.kind == "artist_similarity":
            return self._answer_artist_similarity(question, plan.artist_name or "")

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
            return AssistantResponse(answer="I found matching data but couldn't summarize it right now.", details=validated.display_sql)
        return AssistantResponse(answer=answer, details=validated.display_sql)

    def _answer_artist_similarity(self, question: str, artist_name: str) -> AssistantResponse:
        if not artist_name:
            return AssistantResponse(answer="I couldn't identify the artist to compare.")
        try:
            matches = self._artist_profiles.find_similar(artist_name)
        except ArtistNotFoundError:
            return AssistantResponse(answer=f"I couldn't find an indexed touring profile for {artist_name}.")
        except Exception:
            logger.exception("artist similarity retrieval failed")
            return AssistantResponse(answer="I couldn't retrieve similar touring profiles right now. Please try again.")

        try:
            answer = self._planner.synthesize_similarity(question, artist_name, matches)
        except Exception:
            logger.exception("artist similarity synthesis failed")
            return AssistantResponse(
                answer="I found similar touring profiles but couldn't summarize them right now.",
                details=f"Touring-profile similarity for {artist_name}",
            )
        return AssistantResponse(
            answer=answer,
            details=f"Touring-profile similarity for {artist_name}",
        )
