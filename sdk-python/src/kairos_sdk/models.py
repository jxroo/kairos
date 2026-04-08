from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class KairosModel(BaseModel):
    model_config = ConfigDict(populate_by_name=True)


class HealthResponse(KairosModel):
    status: str
    version: str
    uptime: str


class Memory(KairosModel):
    ID: str
    Content: str
    Summary: str = ""
    Importance: str
    ConversationID: str = ""
    Source: str = ""
    ImportanceWeight: float = 0.0
    DecayScore: float = 0.0
    Tags: list[str] | None = None
    CreatedAt: str
    UpdatedAt: str
    AccessedAt: str
    AccessCount: int = 0


class SearchResult(KairosModel):
    Memory: Memory
    SimilarityScore: float
    FinalScore: float


class Conversation(KairosModel):
    id: str
    title: str
    model: str
    created_at: str
    updated_at: str


class ConversationMessage(KairosModel):
    id: str
    conversation_id: str
    role: Literal["system", "user", "assistant"]
    content: str
    tokens: int | None = None
    created_at: str


class ConversationDetail(Conversation):
    messages: list[ConversationMessage]


class Document(KairosModel):
    ID: str
    Path: str
    Filename: str
    Extension: str
    SizeBytes: int
    FileHash: str
    Status: str
    ErrorMsg: str = ""
    CreatedAt: str
    UpdatedAt: str
    IndexedAt: str | None = None


class Chunk(KairosModel):
    ID: str
    DocumentID: str
    ChunkIndex: int
    Content: str
    StartLine: int
    EndLine: int
    Metadata: str = ""
    CreatedAt: str


class ChunkSearchResult(KairosModel):
    Chunk: Chunk
    Document: Document
    FinalScore: float


class IndexStatus(KairosModel):
    state: str
    total_files: int
    indexed_files: int
    failed_files: int
    percent: int


class ModelEntry(KairosModel):
    id: str
    object: str
    owned_by: str
    context_length: int | None = None
    size_bytes: int | None = None
    capabilities: list[str] | None = None


class ModelsResponse(KairosModel):
    object: str
    data: list[ModelEntry]


class ChatMessage(KairosModel):
    role: str
    content: str


class ChatChoice(KairosModel):
    index: int
    message: ChatMessage
    finish_reason: str | None = None


class ChatUsage(KairosModel):
    prompt_tokens: int | None = None
    completion_tokens: int | None = None
    total_tokens: int | None = None


class ChatCompletionResponse(KairosModel):
    id: str
    object: str
    created: int | None = None
    model: str
    choices: list[ChatChoice]
    usage: ChatUsage | None = None


class ChatRequest(KairosModel):
    model: str
    messages: list[ChatMessage]
    temperature: float | None = None
    max_tokens: int | None = None
    stream: bool = False


class PluginContext(KairosModel):
    conversation_id: str | None = None
    memory_hits: list[SearchResult] = Field(default_factory=list)
    document_hits: list[ChunkSearchResult] = Field(default_factory=list)
    metadata: dict[str, Any] = Field(default_factory=dict)


class EvaluationResult(KairosModel):
    score: float
    label: str
    reason: str | None = None
    metadata: dict[str, Any] = Field(default_factory=dict)
