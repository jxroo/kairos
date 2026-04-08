import { useQuery } from "@tanstack/react-query";
import { listModels } from "@/api/inference";

export function useModels() {
  return useQuery({
    queryKey: ["models"],
    queryFn: listModels,
    staleTime: 30_000,
  });
}
