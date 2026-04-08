import { apiFetch } from "./client";
import type { Memory, CreateMemoryInput, UpdateMemoryInput, SearchResult } from "./types";

export async function searchMemories(query: string, limit = 20): Promise<SearchResult[]> {
  const params = new URLSearchParams({ query, limit: String(limit) });
  return apiFetch<SearchResult[]>(`/memories/search?${params}`);
}

export async function getMemory(id: string): Promise<Memory> {
  return apiFetch<Memory>(`/memories/${id}`);
}

export async function createMemory(input: CreateMemoryInput): Promise<Memory> {
  return apiFetch<Memory>("/memories", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateMemory(id: string, input: UpdateMemoryInput): Promise<Memory> {
  return apiFetch<Memory>(`/memories/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export async function deleteMemory(id: string): Promise<void> {
  return apiFetch<void>(`/memories/${id}`, { method: "DELETE" });
}
