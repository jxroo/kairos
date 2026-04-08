import { apiFetch } from "./client";
import type { HealthResponse } from "./types";

export async function getHealth(): Promise<HealthResponse> {
  return apiFetch<HealthResponse>("/health");
}
