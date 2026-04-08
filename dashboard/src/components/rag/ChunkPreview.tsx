import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { X } from "lucide-react";
import type { ChunkSearchResult } from "@/api/types";

interface ChunkPreviewProps {
  result: ChunkSearchResult;
  onClose: () => void;
}

export function ChunkPreview({ result, onClose }: ChunkPreviewProps) {
  const { Chunk: chunk, Document: doc, FinalScore } = result;
  const lines = chunk.Content.split("\n");

  return (
    <Card className="p-0 overflow-hidden">
      <div className="flex items-center justify-between px-5 py-3 border-b border-white/8">
        <div className="flex items-center gap-2">
          <span className="text-xs text-white/90">{doc.Filename}</span>
          <Badge>{doc.Extension}</Badge>
          <span className="text-[10px] text-white/25">
            lines {chunk.StartLine}–{chunk.EndLine}
          </span>
          <Badge variant="accent">{(FinalScore * 100).toFixed(1)}%</Badge>
        </div>
        <Button variant="ghost" size="sm" onClick={onClose}>
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="overflow-x-auto">
        <pre className="text-[11px] font-mono leading-5 p-0">
          {lines.map((line, i) => (
            <div key={i} className="flex hover:bg-accent/5">
              <span className="w-10 shrink-0 text-right pr-3 text-white/18 select-none border-r border-white/5">
                {chunk.StartLine + i}
              </span>
              <span className="pl-3 text-white/65 whitespace-pre">{line}</span>
            </div>
          ))}
        </pre>
      </div>
    </Card>
  );
}
