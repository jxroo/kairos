# Kairos API Reference

Kairos exposes a local HTTP API for managing memories, searching documents, chatting with LLMs, and executing tools.

- **Base URL:** `http://localhost:7777`
- **Authentication:** None (local-only daemon)
- **Content-Type:** `application/json` for all request and response bodies
- **Body size limit:** 1 MB

---

## Endpoint Reference

| Method   | Path                      | Description                              |
|----------|---------------------------|------------------------------------------|
| `GET`    | `/health`                 | Health check                             |
| `POST`   | `/memories`               | Create a new memory                      |
| `GET`    | `/memories/search`        | Semantic search across memories          |
| `GET`    | `/memories/{id}`          | Get a specific memory                    |
| `PUT`    | `/memories/{id}`          | Update a memory                          |
| `DELETE` | `/memories/{id}`          | Delete a memory                          |
| `GET`    | `/index/status`           | RAG indexing status and progress         |
| `POST`   | `/index/rebuild`          | Trigger full re-index of watched paths   |
| `GET`    | `/search/documents`       | Hybrid search across indexed documents   |
| `POST`   | `/v1/chat/completions`    | OpenAI-compatible chat completions       |
| `GET`    | `/v1/models`              | List available models from all providers |
| `GET`    | `/conversations`          | List all conversations                   |
| `GET`    | `/conversations/search`   | Search conversations                     |
| `GET`    | `/conversations/{id}`     | Get a conversation with messages         |
| `DELETE` | `/conversations/{id}`     | Delete a conversation                    |
| `GET`    | `/tools`                  | List available tools                     |
| `POST`   | `/tools/execute`          | Execute a tool in the sandbox            |
| `GET`    | `/tools/audit`            | View tool execution audit log            |
| `*`      | `/mcp`                    | MCP SSE transport endpoint               |

---

## Health

### `GET /health`

Returns daemon health status.

```bash
curl http://localhost:7777/health
```

**Response `200 OK`:**

```json
{
  "status": "ok",
  "version": "0.1.0",
  "uptime": "2h15m42s"
}
```

---

## Memories

### `POST /memories`

Create a new memory.

```bash
curl -X POST http://localhost:7777/memories \
  -H "Content-Type: application/json" \
  -d '{
    "content": "The Kairos project uses Go and Rust for its runtime.",
    "importance": "high",
    "tags": ["project", "architecture"]
  }'
```

**Request body:**

| Field        | Type       | Required | Description                              |
|--------------|------------|----------|------------------------------------------|
| `content`    | `string`   | yes      | The memory text                          |
| `importance` | `string`   | no       | `"high"`, `"medium"`, or `"low"` (default `"medium"`) |
| `tags`       | `string[]` | no       | Tags to associate with the memory        |

**Response `201 Created`:**

```json
{
  "id": "a3f1b2c4-5d6e-7f8a-9b0c-1d2e3f4a5b6c",
  "content": "The Kairos project uses Go and Rust for its runtime.",
  "importance": "high",
  "tags": ["project", "architecture"],
  "entities": [
    {"type": "project", "value": "Kairos"}
  ],
  "created_at": "2026-03-16T10:30:00Z",
  "updated_at": "2026-03-16T10:30:00Z"
}
```

### `GET /memories/search`

Semantic search across stored memories. Results are ranked by a combination of vector similarity, temporal decay, and importance.

```bash
curl "http://localhost:7777/memories/search?query=what+languages+does+kairos+use"
```

**Query parameters:**

| Param   | Type     | Required | Description          |
|---------|----------|----------|----------------------|
| `query` | `string` | yes      | Natural language query |

**Response `200 OK`:**

```json
{
  "results": [
    {
      "id": "a3f1b2c4-5d6e-7f8a-9b0c-1d2e3f4a5b6c",
      "content": "The Kairos project uses Go and Rust for its runtime.",
      "importance": "high",
      "relevance": 0.9142,
      "tags": ["project", "architecture"],
      "entities": [
        {"type": "project", "value": "Kairos"}
      ],
      "created_at": "2026-03-16T10:30:00Z",
      "updated_at": "2026-03-16T10:30:00Z"
    }
  ]
}
```

### `GET /memories/{id}`

Retrieve a specific memory by ID. Accessing a memory reinforces its decay score.

```bash
curl http://localhost:7777/memories/a3f1b2c4-5d6e-7f8a-9b0c-1d2e3f4a5b6c
```

**Response `200 OK`:**

```json
{
  "id": "a3f1b2c4-5d6e-7f8a-9b0c-1d2e3f4a5b6c",
  "content": "The Kairos project uses Go and Rust for its runtime.",
  "importance": "high",
  "tags": ["project", "architecture"],
  "entities": [
    {"type": "project", "value": "Kairos"}
  ],
  "created_at": "2026-03-16T10:30:00Z",
  "updated_at": "2026-03-16T10:30:00Z"
}
```

### `PUT /memories/{id}`

Update an existing memory.

```bash
curl -X PUT http://localhost:7777/memories/a3f1b2c4-5d6e-7f8a-9b0c-1d2e3f4a5b6c \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Kairos uses Go 1.26 and Rust 1.94 for its runtime.",
    "importance": "high",
    "tags": ["project", "architecture", "versions"]
  }'
```

**Request body:** Same fields as `POST /memories`.

**Response `200 OK`:**

```json
{
  "id": "a3f1b2c4-5d6e-7f8a-9b0c-1d2e3f4a5b6c",
  "content": "Kairos uses Go 1.26 and Rust 1.94 for its runtime.",
  "importance": "high",
  "tags": ["project", "architecture", "versions"],
  "entities": [
    {"type": "project", "value": "Kairos"}
  ],
  "created_at": "2026-03-16T10:30:00Z",
  "updated_at": "2026-03-16T11:05:00Z"
}
```

### `DELETE /memories/{id}`

Delete a memory permanently.

```bash
curl -X DELETE http://localhost:7777/memories/a3f1b2c4-5d6e-7f8a-9b0c-1d2e3f4a5b6c
```

**Response `204 No Content`** (empty body)

---

## RAG / Indexing

### `GET /index/status`

Get the current status of the RAG indexing pipeline.

```bash
curl http://localhost:7777/index/status
```

**Response `200 OK`:**

```json
{
  "total_documents": 142,
  "total_chunks": 1837,
  "indexing": false,
  "progress": 100
}
```

When indexing is in progress:

```json
{
  "total_documents": 98,
  "total_chunks": 1204,
  "indexing": true,
  "progress": 67
}
```

### `POST /index/rebuild`

Trigger a full re-index of all configured watch paths. The operation runs asynchronously; use `GET /index/status` to monitor progress.

```bash
curl -X POST http://localhost:7777/index/rebuild
```

**Response `202 Accepted`:**

```json
{
  "status": "rebuild_started"
}
```

### `GET /search/documents`

Hybrid search across indexed documents using both vector similarity and full-text (Bleve) keyword matching, combined via Reciprocal Rank Fusion (alpha=0.6 toward semantic).

```bash
curl "http://localhost:7777/search/documents?query=error+handling+patterns"
```

**Query parameters:**

| Param   | Type     | Required | Description          |
|---------|----------|----------|----------------------|
| `query` | `string` | yes      | Search query         |

**Response `200 OK`:**

```json
{
  "results": [
    {
      "document_id": "b7e2d1a0-4c3f-8e9d-2a1b-0c5d6e7f8a9b",
      "path": "/home/user/projects/kairos/internal/memory/service.go",
      "chunk_text": "func (s *Service) Search(ctx context.Context, query string) ([]Memory, error) {\n\tif query == \"\" {\n\t\treturn nil, fmt.Errorf(\"search: %w\", ErrEmptyQuery)\n\t}\n...",
      "score": 0.8731
    },
    {
      "document_id": "c8f3e2b1-5d4a-9f0e-3b2c-1d6e7f8a9b0c",
      "path": "/home/user/projects/kairos/internal/rag/pipeline.go",
      "chunk_text": "// ProcessFile handles errors at each pipeline stage:\n// parse -> chunk -> embed -> store\nfunc (p *Pipeline) ProcessFile(ctx context.Context, path string) error {\n...",
      "score": 0.8254
    }
  ]
}
```

---

## Chat / Inference

### `POST /v1/chat/completions`

OpenAI-compatible chat completions endpoint. Kairos routes requests to configured LLM providers (Ollama, llama.cpp) and automatically injects relevant context from memories and indexed documents into the system prompt.

```bash
curl -X POST http://localhost:7777/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3:8b",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "How does memory decay work in Kairos?"}
    ]
  }'
```

**Request body:**

| Field      | Type       | Required | Description                              |
|------------|------------|----------|------------------------------------------|
| `model`    | `string`   | yes      | Model name (as reported by `/v1/models`) |
| `messages` | `object[]` | yes      | Array of `{role, content}` message objects |
| `stream`   | `bool`     | no       | Enable SSE streaming (default `false`)   |

**Response `200 OK` (non-streaming):**

```json
{
  "id": "chatcmpl-d4e5f6a7-8b9c-0d1e-2f3a-4b5c6d7e8f9a",
  "object": "chat.completion",
  "created": 1742121600,
  "model": "llama3:8b",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Memory decay in Kairos uses an exponential function with a base factor of 0.95. Each day that passes without a memory being accessed, its relevance score is multiplied by 0.95^days. When a memory is accessed (retrieved or searched), its decay counter is reinforced, effectively resetting the decay clock."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 384,
    "completion_tokens": 67,
    "total_tokens": 451
  }
}
```

**Response (streaming with `"stream": true`):**

The response uses Server-Sent Events (SSE) in OpenAI format:

```
data: {"id":"chatcmpl-d4e5f6a7-8b9c-0d1e-2f3a-4b5c6d7e8f9a","object":"chat.completion.chunk","created":1742121600,"model":"llama3:8b","choices":[{"index":0,"delta":{"role":"assistant","content":"Memory"},"finish_reason":null}]}

data: {"id":"chatcmpl-d4e5f6a7-8b9c-0d1e-2f3a-4b5c6d7e8f9a","object":"chat.completion.chunk","created":1742121600,"model":"llama3:8b","choices":[{"index":0,"delta":{"content":" decay"},"finish_reason":null}]}

...

data: [DONE]
```

Streaming example with curl:

```bash
curl -N -X POST http://localhost:7777/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3:8b",
    "messages": [
      {"role": "user", "content": "Explain RAG in one sentence."}
    ],
    "stream": true
  }'
```

### `GET /v1/models`

List all available models across configured inference providers.

```bash
curl http://localhost:7777/v1/models
```

**Response `200 OK`:**

```json
{
  "object": "list",
  "data": [
    {
      "id": "llama3:8b",
      "object": "model",
      "owned_by": "ollama"
    },
    {
      "id": "mistral:7b",
      "object": "model",
      "owned_by": "ollama"
    },
    {
      "id": "codellama:13b",
      "object": "model",
      "owned_by": "llamacpp"
    }
  ]
}
```

> **OpenAI compatibility note:** The `/v1/chat/completions` and `/v1/models` endpoints follow the [OpenAI API specification](https://platform.openai.com/docs/api-reference). This means you can point any OpenAI-compatible client library at `http://localhost:7777` and it will work out of the box. For example, with the Python `openai` library:
>
> ```python
> from openai import OpenAI
>
> client = OpenAI(base_url="http://localhost:7777/v1", api_key="unused")
> response = client.chat.completions.create(
>     model="llama3:8b",
>     messages=[{"role": "user", "content": "Hello!"}]
> )
> print(response.choices[0].message.content)
> ```

---

## Conversations

### `GET /conversations`

List all stored conversations.

```bash
curl http://localhost:7777/conversations
```

**Response `200 OK`:**

```json
{
  "conversations": [
    {
      "id": "e5f6a7b8-9c0d-1e2f-3a4b-5c6d7e8f9a0b",
      "title": "Memory decay discussion",
      "model": "llama3:8b",
      "message_count": 4,
      "created_at": "2026-03-16T09:00:00Z",
      "updated_at": "2026-03-16T09:15:00Z"
    },
    {
      "id": "f6a7b8c9-0d1e-2f3a-4b5c-6d7e8f9a0b1c",
      "title": "RAG pipeline questions",
      "model": "mistral:7b",
      "message_count": 6,
      "created_at": "2026-03-15T14:20:00Z",
      "updated_at": "2026-03-15T14:45:00Z"
    }
  ]
}
```

### `GET /conversations/search`

Search conversations by content.

```bash
curl "http://localhost:7777/conversations/search?query=memory+decay"
```

**Query parameters:**

| Param   | Type     | Required | Description          |
|---------|----------|----------|----------------------|
| `query` | `string` | yes      | Search query         |

**Response `200 OK`:**

```json
{
  "conversations": [
    {
      "id": "e5f6a7b8-9c0d-1e2f-3a4b-5c6d7e8f9a0b",
      "title": "Memory decay discussion",
      "model": "llama3:8b",
      "message_count": 4,
      "created_at": "2026-03-16T09:00:00Z",
      "updated_at": "2026-03-16T09:15:00Z"
    }
  ]
}
```

### `GET /conversations/{id}`

Get a full conversation including all messages.

```bash
curl http://localhost:7777/conversations/e5f6a7b8-9c0d-1e2f-3a4b-5c6d7e8f9a0b
```

**Response `200 OK`:**

```json
{
  "id": "e5f6a7b8-9c0d-1e2f-3a4b-5c6d7e8f9a0b",
  "title": "Memory decay discussion",
  "model": "llama3:8b",
  "created_at": "2026-03-16T09:00:00Z",
  "updated_at": "2026-03-16T09:15:00Z",
  "messages": [
    {
      "role": "user",
      "content": "How does memory decay work in Kairos?",
      "created_at": "2026-03-16T09:00:00Z"
    },
    {
      "role": "assistant",
      "content": "Memory decay in Kairos uses an exponential function with a base factor of 0.95...",
      "created_at": "2026-03-16T09:00:05Z"
    }
  ]
}
```

### `DELETE /conversations/{id}`

Delete a conversation and all its messages.

```bash
curl -X DELETE http://localhost:7777/conversations/e5f6a7b8-9c0d-1e2f-3a4b-5c6d7e8f9a0b
```

**Response `204 No Content`** (empty body)

---

## Tools

### `GET /tools`

List all available tools registered in the tool runtime.

```bash
curl http://localhost:7777/tools
```

**Response `200 OK`:**

```json
{
  "tools": [
    {
      "name": "shell",
      "description": "Execute a shell command in the sandbox",
      "parameters": {
        "command": {"type": "string", "description": "The command to run", "required": true},
        "timeout": {"type": "integer", "description": "Timeout in seconds", "required": false}
      }
    },
    {
      "name": "http",
      "description": "Make an HTTP request (SSRF-protected)",
      "parameters": {
        "url": {"type": "string", "description": "Target URL", "required": true},
        "method": {"type": "string", "description": "HTTP method", "required": false}
      }
    }
  ]
}
```

### `POST /tools/execute`

Execute a tool in the sandboxed runtime. The sandbox enforces security constraints including SSRF protection (private IP blocking) and resource limits.

```bash
curl -X POST http://localhost:7777/tools/execute \
  -H "Content-Type: application/json" \
  -d '{
    "tool": "shell",
    "parameters": {
      "command": "echo hello world",
      "timeout": 10
    }
  }'
```

**Request body:**

| Field        | Type     | Required | Description                          |
|--------------|----------|----------|--------------------------------------|
| `tool`       | `string` | yes      | Name of the tool to execute          |
| `parameters` | `object` | yes      | Tool-specific parameters             |

**Response `200 OK`:**

```json
{
  "tool": "shell",
  "status": "success",
  "result": "hello world\n",
  "duration_ms": 42
}
```

### `GET /tools/audit`

View the tool execution audit log.

```bash
curl http://localhost:7777/tools/audit
```

**Response `200 OK`:**

```json
{
  "entries": [
    {
      "id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
      "tool": "shell",
      "parameters": {"command": "echo hello world", "timeout": 10},
      "status": "success",
      "duration_ms": 42,
      "executed_at": "2026-03-16T10:45:00Z"
    },
    {
      "id": "b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e",
      "tool": "http",
      "parameters": {"url": "https://example.com", "method": "GET"},
      "status": "success",
      "duration_ms": 230,
      "executed_at": "2026-03-16T10:42:00Z"
    }
  ]
}
```

---

## MCP (Model Context Protocol)

### `* /mcp`

The MCP endpoint provides an SSE (Server-Sent Events) transport for the [Model Context Protocol](https://modelcontextprotocol.io). This allows MCP-compatible clients to connect to Kairos as both an MCP server and client.

Connect with an SSE client:

```bash
curl -N http://localhost:7777/mcp
```

The MCP transport handles bidirectional communication over SSE, enabling external tools and context providers to integrate with the Kairos runtime.

---

## Error Responses

All endpoints return errors in a consistent JSON format:

```json
{
  "error": "description of what went wrong"
}
```

Common HTTP status codes:

| Status | Meaning               | Example                                      |
|--------|-----------------------|----------------------------------------------|
| `400`  | Bad Request           | Missing required field, malformed JSON        |
| `404`  | Not Found             | Memory or conversation ID does not exist      |
| `413`  | Payload Too Large     | Request body exceeds 1 MB limit               |
| `500`  | Internal Server Error | Database error, inference provider unreachable |

**Example error response (`404`):**

```bash
curl http://localhost:7777/memories/00000000-0000-0000-0000-000000000000
```

```json
{
  "error": "memory not found"
}
```

**Example error response (`400`):**

```bash
curl -X POST http://localhost:7777/memories \
  -H "Content-Type: application/json" \
  -d '{}'
```

```json
{
  "error": "content is required"
}
```
