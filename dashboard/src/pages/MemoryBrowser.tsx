import { useState, useCallback } from "react";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { Empty } from "@/components/ui/Empty";
import { MemorySearch } from "@/components/memory/MemorySearch";
import { MemoryTable } from "@/components/memory/MemoryTable";
import { MemoryDetail } from "@/components/memory/MemoryDetail";
import { useMemorySearch, useDeleteMemory } from "@/hooks/useMemories";
import { Brain } from "lucide-react";

export function MemoryBrowser() {
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const { data: results, isLoading } = useMemorySearch(query, 50);
  const deleteMutation = useDeleteMemory();

  const handleSearch = useCallback((q: string) => setQuery(q), []);

  const selectedMemory = results?.find((r) => r.Memory.ID === selectedId)?.Memory;

  return (
    <div className="animate-fade-in-up">
      <PageHeader
        title="Memory Browser"
        description="Search and manage stored memories"
      />
      <div className="space-y-4">
        <div className="flex items-center gap-3">
          <div className="flex-1">
            <MemorySearch onSearch={handleSearch} />
          </div>
          <span className="text-xs text-white/25">{results?.length ?? 0} memories</span>
        </div>
        <Card className="p-0 overflow-hidden">
          {isLoading ? (
            <div className="p-5"><TableSkeleton rows={6} cols={5} /></div>
          ) : !results?.length ? (
            <Empty icon={Brain} title="No memories" description={query ? "No memories match your search" : "No memories stored yet"} />
          ) : (
            <MemoryTable
              results={results}
              onSelect={setSelectedId}
              onDelete={(id) => { if (selectedId === id) setSelectedId(null); deleteMutation.mutate(id); }}
            />
          )}
        </Card>
        {selectedMemory && (
          <MemoryDetail memory={selectedMemory} onClose={() => setSelectedId(null)} />
        )}
      </div>
    </div>
  );
}
