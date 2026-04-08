import { apiFetch } from "./client";
import type { ConfigResponse, UpdateConfigInput, UpdateConfigResponse } from "./types";

export async function getConfig(): Promise<ConfigResponse> {
  return apiFetch<ConfigResponse>("/config");
}

export async function updateConfig(input: UpdateConfigInput): Promise<UpdateConfigResponse> {
  return apiFetch<UpdateConfigResponse>("/config", {
    method: "PUT",
    body: JSON.stringify(input),
  });
}
