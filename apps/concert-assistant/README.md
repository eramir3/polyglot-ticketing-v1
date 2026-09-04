# Concert Assistant

Python service scaffold for the future Concert Assistant AI chat. Dependencies
are managed with [uv](https://docs.astral.sh/uv/); Gradio is present for the
future chat UI.

No LLM provider, retrieval pipeline, persistence, or UI behavior is configured
yet.

```bash
uv sync
uv run python -c "import gradio"
```
