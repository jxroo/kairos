import { useState, useCallback } from "react";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { Empty } from "@/components/ui/Empty";
import { IndexStatusCard } from "@/components/rag/IndexStatusCard";
import { IndexedDocumentsCard } from "@/components/rag/IndexedDocumentsCard";
import { DocumentSearch } from "@/components/rag/DocumentSearch";
import { DocumentList } from "@/components/rag/DocumentList";
import { ChunkPreview } from "@/components/rag/ChunkPreview";
import { useDocumentSearch } from "@/hooks/useDocumentSearch";
import { FolderSearch } from "lucide-react";
import type { ChunkSearchResult } from "@/api/types";

export function RAGStatus() {
  const [query, setQuery] = useState("");
  const [selectedResult, setSelectedResult] = useState<ChunkSearchResult | null>(null);
  const { data: results, isLoading } = useDocumentSearch(query);

  const handleSearch = useCallback((q: string) => { setQuery(q); setSelectedResult(null); }, []);

  return (
    <div className="animate-fade-in-up">
      <PageHeader title="RAG Index" description="File indexing status, indexed-file browser, and document search" />
      <div className="space-y-4">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <IndexStatusCard />
          <DocumentSearch onSearch={handleSearch} />
        </div>
        <IndexedDocumentsCard />
        {query && (
          <Card className="p-0 overflow-hidden">
            {isLoading ? (
              <div className="p-5"><TableSkeleton rows={5} cols={4} /></div>
            ) : !results?.length ? (
              <Empty icon={FolderSearch} title="No results" description="No documents match your search" />
            ) : (
              <DocumentList results={results} onSelect={setSelectedResult} />
            )}
          </Card>
        )}
        {selectedResult && (
          <ChunkPreview result={selectedResult} onClose={() => setSelectedResult(null)} />
        )}
      </div>
    </div>
  );
}
