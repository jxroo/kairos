import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { searchMemories, createMemory, deleteMemory } from "@/api/memories";
import type { CreateMemoryInput } from "@/api/types";

export function useMemorySearch(query: string, limit = 20) {
  return useQuery({
    queryKey: ["memories", "search", query, limit],
    queryFn: () => searchMemories(query, limit),
    placeholderData: (prev) => prev,
  });
}

export function useCreateMemory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateMemoryInput) => createMemory(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["memories"] }),
  });
}

export function useDeleteMemory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteMemory(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["memories"] }),
  });
}
