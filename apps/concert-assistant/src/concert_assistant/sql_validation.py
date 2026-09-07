"""Read-only SQL validation for LLM-generated PostgreSQL queries."""

from __future__ import annotations

from dataclasses import dataclass

from sqlglot import exp, parse
from sqlglot.errors import ParseError

from concert_assistant.models import ValidatedQuery
from concert_assistant.schema import ALLOWED_COLUMNS


FORBIDDEN_EXPRESSIONS = (
    exp.Insert,
    exp.Update,
    exp.Delete,
    exp.Create,
    exp.Drop,
    exp.Alter,
    exp.Command,
    exp.Copy,
    exp.Merge,
    exp.Grant,
    exp.Revoke,
)


class SQLValidationError(ValueError):
    """The generated query is outside the intentionally small query boundary."""


@dataclass(frozen=True)
class SQLValidator:
    max_result_rows: int

    def validate(self, sql: str) -> ValidatedQuery:
        try:
            statements = parse(sql, read="postgres")
        except ParseError as error:
            raise SQLValidationError("The generated query could not be parsed.") from error

        if len(statements) != 1:
            raise SQLValidationError("Exactly one SQL statement is allowed.")
        statement = statements[0]
        if not isinstance(statement, exp.Select):
            raise SQLValidationError("Only SELECT queries are allowed.")
        if any(statement.find(expression_type) for expression_type in FORBIDDEN_EXPRESSIONS):
            raise SQLValidationError("Queries may not contain write or administration operations.")
        if any(isinstance(expression, exp.Star) for expression in statement.expressions):
            raise SQLValidationError("SELECT * is not allowed.")

        self._validate_relation(statement)
        self._validate_columns(statement)

        display_sql = statement.sql(dialect="postgres")
        execution_sql = (
            "SELECT * FROM ("
            f"{display_sql}"
            f") AS concert_assistant_query LIMIT {self.max_result_rows}"
        )
        return ValidatedQuery(display_sql=display_sql, execution_sql=execution_sql)

    @staticmethod
    def _validate_relation(statement: exp.Select) -> None:
        tables = list(statement.find_all(exp.Table))
        if len(tables) != 1:
            raise SQLValidationError("Queries must read exactly one concerts relation.")

        table = tables[0]
        if table.name.lower() != "concerts" or (table.db and table.db.lower() != "public"):
            raise SQLValidationError("Queries may read only public.concerts.")

    @staticmethod
    def _validate_columns(statement: exp.Select) -> None:
        aliases = {
            expression.alias.lower()
            for expression in statement.expressions
            if expression.alias
        }
        unknown_columns = {
            column.name.lower()
            for column in statement.find_all(exp.Column)
            if column.name
            and column.name.lower() not in ALLOWED_COLUMNS
            and column.name.lower() not in aliases
        }
        if unknown_columns:
            names = ", ".join(sorted(unknown_columns))
            raise SQLValidationError(f"Query references unavailable columns: {names}.")
