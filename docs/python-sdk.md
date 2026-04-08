# Python SDK

Typed Python client for the Kairos API, built on `httpx` and `pydantic`.

> **Status:** Pre-1.0 — the API surface may change between minor versions.

## Installation

```bash
cd sdk-python
python -m pip install -e .
# or with dev dependencies
python -m pip install -e .[dev]
```

## Quick Start

```python
from kairos_sdk import Client

with Client() as client:
    health = client.health()
    print(health.version, health.uptime)
```

## Configuration

`Client` accepts the following parameters:

| Parameter  | Type            | Default                  | Description                |
|------------|-----------------|--------------------------|----------------------------|
| `base_url` | `str`           | `http://localhost:7777`  | Kairos daemon address      |
| `timeout`  | `float \| None` | `30.0`                   | Request timeout in seconds |

```python
client = Client(base_url="http://192.168.1.50:7777", timeout=10.0)
```

The client implements the context-manager protocol and should be closed when no longer needed:

```python
# Option A: context manager (preferred)
with Client() as client:
    ...

# Option B: explicit close
client = Client()
try:
    ...
finally:
    client.close()
```

## API Reference

### Health

```python
health = client.health()
# health.version  -> "0.1.0"
# health.uptime   -> "3h12m"
```

Returns a `HealthResponse` with the daemon's version and uptime.

### Memories

#### Create a memory

```python
memory = client.create_memory(
    content="Kairos uses HNSW for vector search",
    importance=0.8,
)
print(memory.id, memory.content)
```

Returns a `Memory` object.

#### Search memories

```python
results = client.search_memories(query="vector search", limit=5)
for mem in results:
    print(mem.id, mem.content, mem.score)
```

Returns `list[Memory]`. Results are ranked by a combined similarity, decay, and importance score.

#### Get a memory by ID

```python
memory = client.get_memory("b1a2c3d4-...")
```

Returns a single `Memory`.

#### Update a memory

```python
updated = client.update_memory(
    id="b1a2c3d4-...",
    content="Kairos uses HNSW (via usearch) for vector search",
    importance=0.9,
)
```

Returns the updated `Memory`.

#### Delete a memory

```python
client.delete_memory("b1a2c3d4-...")
```

Returns `None`. Raises `KairosError` if the memory does not exist.

### Chat (Inference)

#### Blocking request

```python
response = client.chat(
    model="llama3",
    messages=[
        {"role": "user", "content": "What is Kairos?"},
    ],
)
print(response.choices[0].message.content)
```

Returns a `ChatResponse` following the OpenAI-compatible format.

#### Streaming (SSE)

```python
stream = client.chat(
    model="llama3",
    messages=[{"role": "user", "content": "Explain RAG in three sentences."}],
    stream=True,
)
for chunk in stream:
    delta = chunk.choices[0].delta.content
    if delta:
        print(delta, end="", flush=True)
print()
```

When `stream=True`, the method returns an iterator of `ChatCompletionChunk` objects. Each chunk follows the OpenAI SSE format (`data: {json}`). The stream ends when the server sends `data: [DONE]`.

### Documents (RAG)

#### Search documents

```python
docs = client.search_documents(query="memory decay formula")
for doc in docs:
    print(doc.path, doc.chunk, doc.score)
```

Returns `list[Document]`. Uses hybrid search (vector + full-text) with Reciprocal Rank Fusion.

#### Index status

```python
status = client.index_status()
print(status.documents, status.chunks, status.last_indexed)
```

Returns an `IndexStatus` with current indexing statistics.

### Models

```python
models = client.list_models()
for m in models:
    print(m.name, m.provider)
```

Returns `list[Model]` aggregated from all configured providers (Ollama, llama.cpp).

### Conversations

#### List conversations

```python
conversations = client.list_conversations()
for conv in conversations:
    print(conv.id, conv.created_at)
```

Returns `list[Conversation]`.

#### Get a conversation

```python
conv = client.get_conversation("a1b2c3d4-...")
for msg in conv.messages:
    print(f"[{msg.role}] {msg.content}")
```

Returns a `Conversation` including its full message history.

## Error Handling

All SDK errors inherit from `KairosError`:

```python
from kairos_sdk import Client, KairosError

with Client() as client:
    try:
        memory = client.get_memory("nonexistent-id")
    except KairosError as e:
        print(f"API error: {e.status_code} — {e.message}")
```

Common error scenarios:

| Exception      | Cause                                      |
|----------------|--------------------------------------------|
| `KairosError`  | Non-2xx response from the daemon           |
| `httpx.ConnectError` | Daemon is not running or unreachable |
| `httpx.TimeoutException` | Request exceeded the configured timeout |

For connection-level errors, standard `httpx` exceptions are raised. Wrap calls in a `try/except` block if your application needs to handle daemon unavailability gracefully.

## Development

Install the SDK in editable mode with dev dependencies, then run the test suite:

```bash
python -m pip install -e .[dev]
make test-python
```

Tests expect a running Kairos daemon on `localhost:7777` by default. Set `KAIROS_BASE_URL` to point at a different instance.
