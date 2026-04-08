import { apiFetch } from "./client";
import type { IndexStatus, ChunkSearchResult, Document } from "./types";

export async function getIndexStatus(): Promise<IndexStatus> {
  return apiFetch<IndexStatus>("/index/status");
}

export async function rebuildIndex(): Promise<{ status: string }> {
  return apiFetch<{ status: string }>("/index/rebuild", { method: "POST" });
}

export async function searchDocuments(query: string, limit = 20): Promise<ChunkSearchResult[]> {
  const params = new URLSearchParams({ query, limit: String(limit) });
  return apiFetch<ChunkSearchResult[]>(`/search/documents?${params}`);
}

export async function listDocuments(limit = 100, offset = 0, status?: string): Promise<Document[]> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  if (status) {
    params.set("status", status);
  }
  return apiFetch<Document[]>(`/documents?${params}`);
}
