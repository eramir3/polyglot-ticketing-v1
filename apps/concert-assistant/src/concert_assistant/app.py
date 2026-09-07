"""Gradio entry point for the public Concert Assistant chat."""

from __future__ import annotations

import logging

import gradio as gr

from concert_assistant.agent import ConcertPlanner, OllamaConcertPlanner, OpenAIConcertPlanner
from concert_assistant.config import Settings
from concert_assistant.repository import ConcertRepository
from concert_assistant.service import ConcertAssistantService
from concert_assistant.sql_validation import SQLValidator
from concert_assistant.vector_repository import ArtistProfileRepository


def create_planner(settings: Settings) -> ConcertPlanner | None:
    if settings.llm_provider == "openai":
        return OpenAIConcertPlanner(settings.openai_api_key, settings.openai_model) if settings.openai_api_key else None
    if settings.llm_provider == "ollama":
        return OllamaConcertPlanner(settings.ollama_base_url, settings.ollama_model)
    raise ValueError("LLM_PROVIDER must be either 'openai' or 'ollama'.")


def create_service(settings: Settings) -> ConcertAssistantService:
    planner = create_planner(settings)
    repository = (
        ConcertRepository(settings.database_url, settings.statement_timeout_ms)
        if settings.database_url
        else None
    )
    artist_profiles = (
        ArtistProfileRepository(settings.database_url, settings.statement_timeout_ms)
        if settings.database_url
        else None
    )
    return ConcertAssistantService(
        planner=planner,
        repository=repository,
        artist_profiles=artist_profiles,
        validator=SQLValidator(max_result_rows=settings.max_result_rows),
        history_messages=settings.history_messages,
    )


def create_demo(service: ConcertAssistantService) -> gr.Blocks:
    def respond(message: str, history: list[dict[str, str]]) -> tuple[str, list[dict[str, str]], str]:
        if not message.strip():
            return "", history, ""
        response = service.answer(message, history)
        updated_history = [
            *history,
            {"role": "user", "content": message},
            {"role": "assistant", "content": response.answer},
        ]
        return "", updated_history, response.details or ""

    with gr.Blocks(title="Concert Assistant") as demo:
        gr.Markdown(
            "# Concert Assistant\n"
            "Ask analytical questions about the imported concert dataset. "
            "Ask analytical questions or compare artists by recorded touring history."
        )
        chatbot = gr.Chatbot(label="Concert Assistant")
        with gr.Accordion("Query details", open=False):
            details = gr.Textbox(
                label="Read-only query or retrieval context",
                lines=3,
                max_lines=10,
                interactive=False,
            )
        message = gr.Textbox(label="Question", placeholder="How many concerts did Radiohead play in France?")
        gr.Examples(
            examples=[
                "How many concerts did Radiohead play in France?",
                "Which artist played the most concerts in Germany?",
                "Show me artists who performed more than 100 concerts between 2015 and 2020.",
                "What was the longest gap between concerts for Incubus?",
                "Artists similar to Radiohead",
            ],
            inputs=message,
        )
        message.submit(respond, inputs=[message, chatbot], outputs=[message, chatbot, details])
    return demo


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    settings = Settings.from_environment()
    demo = create_demo(create_service(settings))
    demo.launch(server_name=settings.server_host, server_port=settings.server_port, show_error=False)
