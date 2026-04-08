import { useQuery } from "@tanstack/react-query";
import { listTools, listAudit } from "@/api/tools";

export function useTools() {
  return useQuery({
    queryKey: ["tools"],
    queryFn: listTools,
    staleTime: 30_000,
  });
}

export function useAudit(limit = 50) {
  return useQuery({
    queryKey: ["audit", limit],
    queryFn: () => listAudit(limit),
    refetchInterval: 5000,
  });
}
