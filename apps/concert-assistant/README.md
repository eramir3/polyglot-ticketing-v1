# Concert Assistant

Public Gradio chat for read-only analytical questions over the concert dataset.
Dependencies are managed with [uv](https://docs.astral.sh/uv/).
When run with `uv run concert-assistant`, it loads the repository root `.env`;
explicit shell variables still take precedence. Docker Compose forwards the same
provider settings explicitly to the container.

## Query types

Concert Assistant is organized around three query types:

| Type | Example | Status |
| --- | --- | --- |
| SQL | “How many concerts did Taylor Swift play?” | Implemented. Generates validated read-only SQL over `public.concerts`. |
| Vector search | “Artists similar to Radiohead” | Implemented for named artists. Finds similar recorded touring profiles, not musical genre or style. |
| SQL + RAG | “Tell me about the concert history of Coldplay” | Planned. It will combine SQL facts with retrieved descriptive context once a richer text corpus exists. |

Broad semantic venue search remains unavailable because the current dataset has
no descriptive concert text to retrieve.

The default provider is OpenAI. For native local Ollama testing:

```bash
ollama pull qwen3:1.7b
ollama pull nomic-embed-text

make restore-concerts
make index-concert-artists
# Set LLM_PROVIDER=ollama and the Concert Assistant database values in root .env.

uv run concert-assistant
```

Ollama must be running at `http://127.0.0.1:11434`, unless `OLLAMA_BASE_URL`
is set. The imported database is required; `CONCERT_ASSISTANT_DATABASE_URL` in
`.env.example` targets its host-published local port. Compose overrides that
value with its internal database URL.

`make index-concert-artists` uses `CONCERT_ASSISTANT_ADMIN_DATABASE_URL` to
create and refresh `public.artist_profiles`, a pgvector projection of one
deterministic touring-history profile per artist. The running Gradio service
continues to use the read-only `concert_assistant_reader` role. Ask questions
such as `Artists similar to Radiohead`; results describe similar recorded
touring profiles, not musical genre or style.
