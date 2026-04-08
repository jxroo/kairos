import { apiFetch } from "./client";
import type { Conversation, ConversationDetail } from "./types";

export async function listConversations(limit = 50, offset = 0): Promise<Conversation[]> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return apiFetch<Conversation[]>(`/conversations?${params}`);
}

export async function searchConversations(q: string, limit = 50): Promise<Conversation[]> {
  const params = new URLSearchParams({ q, limit: String(limit) });
  return apiFetch<Conversation[]>(`/conversations/search?${params}`);
}

export async function getConversation(id: string): Promise<ConversationDetail> {
  return apiFetch<ConversationDetail>(`/conversations/${id}`);
}

export async function deleteConversation(id: string): Promise<void> {
  return apiFetch<void>(`/conversations/${id}`, { method: "DELETE" });
}
