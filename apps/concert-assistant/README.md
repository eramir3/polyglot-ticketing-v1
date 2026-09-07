# Concert Assistant

Public Gradio chat for read-only analytical questions over the concert dataset.
Dependencies are managed with [uv](https://docs.astral.sh/uv/).
When run with `uv run concert-assistant`, it loads the repository root `.env`;
explicit shell variables still take precedence. Docker Compose forwards the same
provider settings explicitly to the container.

The default provider is OpenAI. For native local Ollama testing:

```bash
ollama pull qwen3:1.7b

make restore-concerts
# Set LLM_PROVIDER=ollama and the Concert Assistant database values in root .env.

uv run concert-assistant
```

Ollama must be running at `http://127.0.0.1:11434`, unless `OLLAMA_BASE_URL`
is set. The imported database is required; `CONCERT_ASSISTANT_DATABASE_URL` in
`.env.example` targets its host-published local port. Compose overrides that
value with its internal database URL.
