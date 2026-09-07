import os

from concert_assistant import config


def test_loads_dotenv_without_overriding_shell_configuration(monkeypatch) -> None:
    calls: list[bool] = []

    def fake_load_dotenv(*, override: bool) -> None:
        calls.append(override)
        os.environ.setdefault("OLLAMA_MODEL", "from-dotenv")

    monkeypatch.setattr(config, "load_dotenv", fake_load_dotenv)
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("OLLAMA_MODEL", "from-shell")

    settings = config.Settings.from_environment()

    assert calls == [False]
    assert settings.llm_provider == "ollama"
    assert settings.ollama_model == "from-shell"


def test_uses_ollama_defaults_after_loading_dotenv(monkeypatch) -> None:
    monkeypatch.setattr(config, "load_dotenv", lambda *, override: None)
    monkeypatch.delenv("LLM_PROVIDER", raising=False)
    monkeypatch.delenv("OLLAMA_BASE_URL", raising=False)
    monkeypatch.delenv("OLLAMA_MODEL", raising=False)

    settings = config.Settings.from_environment()

    assert settings.llm_provider == "openai"
    assert settings.ollama_base_url == "http://127.0.0.1:11434"
    assert settings.ollama_model == "qwen3:1.7b"
