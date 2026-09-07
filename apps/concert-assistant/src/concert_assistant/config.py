"""Environment-backed configuration for Concert Assistant."""

from __future__ import annotations

from dataclasses import dataclass
import os

from dotenv import load_dotenv


@dataclass(frozen=True)
class Settings:
    database_url: str | None
    llm_provider: str
    openai_api_key: str | None
    openai_model: str
    ollama_base_url: str
    ollama_model: str
    server_host: str
    server_port: int
    statement_timeout_ms: int
    max_result_rows: int
    history_messages: int

    @classmethod
    def from_environment(cls) -> "Settings":
        load_dotenv(override=False)
        return cls(
            database_url=os.getenv("CONCERT_ASSISTANT_DATABASE_URL"),
            llm_provider=os.getenv("LLM_PROVIDER", "openai").lower(),
            openai_api_key=os.getenv("OPENAI_API_KEY"),
            openai_model=os.getenv("OPENAI_MODEL", "gpt-4.1-mini"),
            ollama_base_url=os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434"),
            ollama_model=os.getenv("OLLAMA_MODEL", "qwen3:1.7b"),
            server_host=os.getenv("CONCERT_ASSISTANT_HOST", "0.0.0.0"),
            server_port=int(os.getenv("CONCERT_ASSISTANT_PORT", "7860")),
            statement_timeout_ms=int(os.getenv("CONCERT_ASSISTANT_STATEMENT_TIMEOUT_MS", "3000")),
            max_result_rows=int(os.getenv("CONCERT_ASSISTANT_MAX_RESULT_ROWS", "100")),
            history_messages=int(os.getenv("CONCERT_ASSISTANT_HISTORY_MESSAGES", "6")),
        )
