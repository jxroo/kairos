import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getConfig, updateConfig } from "@/api/config";
import type { UpdateConfigInput } from "@/api/types";

export function useConfig() {
  return useQuery({
    queryKey: ["config"],
    queryFn: getConfig,
    retry: 1,
  });
}

export function useSaveConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateConfigInput) => updateConfig(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["config"] }),
  });
}
