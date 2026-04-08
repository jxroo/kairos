from __future__ import annotations

from typing import Any, TypeVar

import httpx

from .models import (
    ChatCompletionResponse,
    ChatRequest,
    ChunkSearchResult,
    Conversation,
    ConversationDetail,
    Document,
    HealthResponse,
    IndexStatus,
    Memory,
    ModelsResponse,
    SearchResult,
)

T = TypeVar("T")


class Client:
    def __init__(self, base_url: str = "http://127.0.0.1:7777", timeout: float = 30.0) -> None:
        self._client = httpx.Client(base_url=base_url.rstrip("/"), timeout=timeout)

    def close(self) -> None:
        self._client.close()

    def __enter__(self) -> "Client":
        return self

    def __exit__(self, *_args: object) -> None:
        self.close()

    def health(self) -> HealthResponse:
        return self._get_model("/health", HealthResponse)

    def list_models(self) -> ModelsResponse:
        return self._get_model("/v1/models", ModelsResponse)

    def create_memory(self, content: str, **kwargs: Any) -> Memory:
        payload = {"content": content, **kwargs}
        return self._request_model("POST", "/memories", Memory, json=payload)

    def search_memories(self, query: str, limit: int = 20) -> list[SearchResult]:
        return self._get_model_list("/memories/search", SearchResult, params={"query": query, "limit": limit})

    def get_memory(self, memory_id: str) -> Memory:
        return self._get_model(f"/memories/{memory_id}", Memory)

    def delete_memory(self, memory_id: str) -> None:
        self._request("DELETE", f"/memories/{memory_id}")

    def list_conversations(self, limit: int = 50) -> list[Conversation]:
        return self._get_model_list("/conversations", Conversation, params={"limit": limit})

    def get_conversation(self, conversation_id: str) -> ConversationDetail:
        return self._get_model(f"/conversations/{conversation_id}", ConversationDetail)

    def search_conversations(self, query: str, limit: int = 50) -> list[Conversation]:
        return self._get_model_list("/conversations/search", Conversation, params={"q": query, "limit": limit})

    def delete_conversation(self, conversation_id: str) -> None:
        self._request("DELETE", f"/conversations/{conversation_id}")

    def index_status(self) -> IndexStatus:
        return self._get_model("/index/status", IndexStatus)

    def rebuild_index(self) -> dict[str, str]:
        return self._request("POST", "/index/rebuild").json()

    def list_documents(self, limit: int = 100, offset: int = 0, status: str | None = None) -> list[Document]:
        params: dict[str, Any] = {"limit": limit, "offset": offset}
        if status:
            params["status"] = status
        return self._get_model_list("/documents", Document, params=params)

    def search_documents(self, query: str, limit: int = 20) -> list[ChunkSearchResult]:
        return self._get_model_list("/search/documents", ChunkSearchResult, params={"query": query, "limit": limit})

    def chat_completions(self, request: ChatRequest) -> ChatCompletionResponse:
        return self._request_model("POST", "/v1/chat/completions", ChatCompletionResponse, json=request.model_dump())

    def _request(self, method: str, path: str, **kwargs: Any) -> httpx.Response:
        response = self._client.request(method, path, **kwargs)
        response.raise_for_status()
        return response

    def _get_model(self, path: str, model: type[T], **kwargs: Any) -> T:
        return self._request_model("GET", path, model, **kwargs)

    def _request_model(self, method: str, path: str, model: type[T], **kwargs: Any) -> T:
        response = self._request(method, path, **kwargs)
        return model.model_validate(response.json())

    def _get_model_list(self, path: str, model: type[T], **kwargs: Any) -> list[T]:
        response = self._request("GET", path, **kwargs)
        return [model.model_validate(item) for item in response.json()]


class AsyncClient:
    def __init__(self, base_url: str = "http://127.0.0.1:7777", timeout: float = 30.0) -> None:
        self._client = httpx.AsyncClient(base_url=base_url.rstrip("/"), timeout=timeout)

    async def aclose(self) -> None:
        await self._client.aclose()

    async def __aenter__(self) -> "AsyncClient":
        return self

    async def __aexit__(self, *_args: object) -> None:
        await self.aclose()

    async def health(self) -> HealthResponse:
        return await self._get_model("/health", HealthResponse)

    async def list_models(self) -> ModelsResponse:
        return await self._get_model("/v1/models", ModelsResponse)

    async def create_memory(self, content: str, **kwargs: Any) -> Memory:
        payload = {"content": content, **kwargs}
        return await self._request_model("POST", "/memories", Memory, json=payload)

    async def search_memories(self, query: str, limit: int = 20) -> list[SearchResult]:
        return await self._get_model_list("/memories/search", SearchResult, params={"query": query, "limit": limit})

    async def get_memory(self, memory_id: str) -> Memory:
        return await self._get_model(f"/memories/{memory_id}", Memory)

    async def delete_memory(self, memory_id: str) -> None:
        await self._request("DELETE", f"/memories/{memory_id}")

    async def list_conversations(self, limit: int = 50) -> list[Conversation]:
        return await self._get_model_list("/conversations", Conversation, params={"limit": limit})

    async def get_conversation(self, conversation_id: str) -> ConversationDetail:
        return await self._get_model(f"/conversations/{conversation_id}", ConversationDetail)

    async def search_conversations(self, query: str, limit: int = 50) -> list[Conversation]:
        return await self._get_model_list("/conversations/search", Conversation, params={"q": query, "limit": limit})

    async def delete_conversation(self, conversation_id: str) -> None:
        await self._request("DELETE", f"/conversations/{conversation_id}")

    async def index_status(self) -> IndexStatus:
        return await self._get_model("/index/status", IndexStatus)

    async def rebuild_index(self) -> dict[str, str]:
        response = await self._request("POST", "/index/rebuild")
        return response.json()

    async def list_documents(self, limit: int = 100, offset: int = 0, status: str | None = None) -> list[Document]:
        params: dict[str, Any] = {"limit": limit, "offset": offset}
        if status:
            params["status"] = status
        return await self._get_model_list("/documents", Document, params=params)

    async def search_documents(self, query: str, limit: int = 20) -> list[ChunkSearchResult]:
        return await self._get_model_list("/search/documents", ChunkSearchResult, params={"query": query, "limit": limit})

    async def chat_completions(self, request: ChatRequest) -> ChatCompletionResponse:
        return await self._request_model("POST", "/v1/chat/completions", ChatCompletionResponse, json=request.model_dump())

    async def _request(self, method: str, path: str, **kwargs: Any) -> httpx.Response:
        response = await self._client.request(method, path, **kwargs)
        response.raise_for_status()
        return response

    async def _get_model(self, path: str, model: type[T], **kwargs: Any) -> T:
        return await self._request_model("GET", path, model, **kwargs)

    async def _request_model(self, method: str, path: str, model: type[T], **kwargs: Any) -> T:
        response = await self._request(method, path, **kwargs)
        return model.model_validate(response.json())

    async def _get_model_list(self, path: str, model: type[T], **kwargs: Any) -> list[T]:
        response = await self._request("GET", path, **kwargs)
        return [model.model_validate(item) for item in response.json()]
