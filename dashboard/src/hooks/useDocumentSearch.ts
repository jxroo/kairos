import { useQuery } from "@tanstack/react-query";
import { searchDocuments } from "@/api/rag";

export function useDocumentSearch(query: string, limit = 20) {
  return useQuery({
    queryKey: ["documents", "search", query, limit],
    queryFn: () => searchDocuments(query, limit),
    enabled: query.length > 0,
    placeholderData: (prev) => prev,
  });
}
