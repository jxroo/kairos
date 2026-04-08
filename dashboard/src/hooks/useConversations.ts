import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { listConversations, searchConversations, getConversation, deleteConversation } from "@/api/conversations";

export function useConversations(limit = 50) {
  return useQuery({
    queryKey: ["conversations", limit],
    queryFn: () => listConversations(limit),
  });
}

export function useConversationSearch(q: string, limit = 50) {
  return useQuery({
    queryKey: ["conversations", "search", q, limit],
    queryFn: () => searchConversations(q, limit),
    enabled: q.length > 0,
    placeholderData: (prev) => prev,
  });
}

export function useConversation(id: string | null) {
  return useQuery({
    queryKey: ["conversations", id],
    queryFn: () => getConversation(id!),
    enabled: !!id,
  });
}

export function useDeleteConversation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteConversation(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["conversations"] }),
  });
}
