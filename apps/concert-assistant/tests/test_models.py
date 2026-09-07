import pytest
from pydantic import ValidationError

from concert_assistant.models import QueryPlan


def test_accepts_an_artist_similarity_plan() -> None:
    plan = QueryPlan(
        kind="artist_similarity",
        artist_name="Radiohead",
        explanation="Compare touring profiles.",
    )

    assert plan.artist_name == "Radiohead"
    assert plan.sql is None


@pytest.mark.parametrize(
    "kwargs",
    [
        {"kind": "artist_similarity", "explanation": "Missing artist."},
        {"kind": "artist_similarity", "artist_name": "Radiohead", "sql": "SELECT 1", "explanation": "Mixed routes."},
        {"kind": "unsupported", "artist_name": "Radiohead", "explanation": "Unexpected query."},
    ],
)
def test_rejects_invalid_non_sql_plans(kwargs: dict[str, str]) -> None:
    with pytest.raises(ValidationError):
        QueryPlan(**kwargs)
