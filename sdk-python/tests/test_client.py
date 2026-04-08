from __future__ import annotations

import httpx

from kairos_sdk import AsyncClient, Client
from kairos_sdk.models import EvaluationResult, PluginContext


def test_sync_health() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json={"status": "ok", "version": "0.1.0", "uptime": "3s"},
        )
    )

    client = Client()
    client._client = httpx.Client(base_url="http://testserver", transport=transport)
    health = client.health()

    assert health.status == "ok"
    assert health.version == "0.1.0"


async def test_async_list_documents() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json=[
                {
                    "ID": "doc-1",
                    "Path": "/tmp/readme.md",
                    "Filename": "readme.md",
                    "Extension": ".md",
                    "SizeBytes": 42,
                    "FileHash": "abc",
                    "Status": "indexed",
                    "ErrorMsg": "",
                    "CreatedAt": "2026-03-16T00:00:00Z",
                    "UpdatedAt": "2026-03-16T00:00:00Z",
                    "IndexedAt": "2026-03-16T00:00:00Z",
                }
            ],
        )
    )

    client = AsyncClient()
    client._client = httpx.AsyncClient(base_url="http://testserver", transport=transport)
    docs = await client.list_documents()

    assert len(docs) == 1
    assert docs[0].Filename == "readme.md"


def test_evaluator_models() -> None:
    context = PluginContext()
    result = EvaluationResult(score=0.9, label="pass", metadata={"source": "unit"})

    assert context.metadata == {}
    assert result.score == 0.9
