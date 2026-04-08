import { useQuery } from "@tanstack/react-query";
import { getIndexStatus } from "@/api/rag";

export function useIndexStatus() {
  return useQuery({
    queryKey: ["indexStatus"],
    queryFn: getIndexStatus,
    refetchInterval: 3000,
  });
}
