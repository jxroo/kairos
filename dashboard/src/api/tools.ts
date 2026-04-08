import { apiFetch } from "./client";
import type { ToolDefinition, AuditEntry } from "./types";

export async function listTools(): Promise<ToolDefinition[]> {
  return apiFetch<ToolDefinition[]>("/tools");
}

export async function listAudit(limit = 50): Promise<AuditEntry[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  return apiFetch<AuditEntry[]>(`/tools/audit?${params}`);
}
