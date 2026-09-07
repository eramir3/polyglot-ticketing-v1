import pytest

from concert_assistant.sql_validation import SQLValidationError, SQLValidator


@pytest.fixture
def validator() -> SQLValidator:
    return SQLValidator(max_result_rows=100)


def test_accepts_analytical_aggregate_query(validator: SQLValidator) -> None:
    query = validator.validate(
        "SELECT COUNT(*) AS concert_count FROM public.concerts "
        "WHERE artist = 'Radiohead' AND country = 'France'"
    )

    assert query.display_sql.startswith("SELECT COUNT(*)")
    assert query.execution_sql.endswith("LIMIT 100")


def test_accepts_a_select_alias_in_order_by(validator: SQLValidator) -> None:
    query = validator.validate(
        "SELECT artist, COUNT(*) AS concert_count FROM concerts "
        "WHERE country = 'Germany' GROUP BY artist ORDER BY concert_count DESC"
    )

    assert "ORDER BY concert_count DESC" in query.display_sql


@pytest.mark.parametrize(
    "sql",
    [
        "SELECT artist FROM concerts; DELETE FROM concerts",
        "DELETE FROM concerts",
        "UPDATE concerts SET country = 'France'",
        "CREATE TABLE concerts_copy AS SELECT id FROM concerts",
        "WITH removed AS (DELETE FROM concerts RETURNING id) SELECT 1",
        "SELECT id FROM orders",
        "SELECT secret_value FROM concerts",
        "SELECT * FROM concerts",
    ],
)
def test_rejects_out_of_boundary_sql(validator: SQLValidator, sql: str) -> None:
    with pytest.raises(SQLValidationError):
        validator.validate(sql)


def test_caps_rows_even_when_generated_query_has_no_limit(validator: SQLValidator) -> None:
    query = validator.validate("SELECT artist, country FROM concerts ORDER BY artist")

    assert query.execution_sql == (
        "SELECT * FROM (SELECT artist, country FROM concerts ORDER BY artist) "
        "AS concert_assistant_query LIMIT 100"
    )
