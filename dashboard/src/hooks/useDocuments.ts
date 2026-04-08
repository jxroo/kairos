import { useQuery } from "@tanstack/react-query";
import { listDocuments } from "@/api/rag";

export function useDocuments(limit = 100, status?: string) {
  return useQuery({
    queryKey: ["documents", "list", limit, status],
    queryFn: () => listDocuments(limit, 0, status),
    refetchInterval: 5000,
  });
}
