import { apiFetch } from "./client";
import type { OpenAIModelsResponse } from "./types";

export async function listModels(): Promise<OpenAIModelsResponse> {
  return apiFetch<OpenAIModelsResponse>("/v1/models");
}
