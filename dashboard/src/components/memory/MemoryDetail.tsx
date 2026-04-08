import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { ImportanceBadge } from "./ImportanceBadge";
import { formatRelativeTime } from "@/lib/format";
import { X } from "lucide-react";
import type { Memory } from "@/api/types";

interface MemoryDetailProps {
  memory: Memory;
  onClose: () => void;
}

export function MemoryDetail({ memory, onClose }: MemoryDetailProps) {
  return (
    <div className="glass p-5 space-y-3">
      <div className="flex items-start justify-between gap-2">
        <ImportanceBadge importance={memory.Importance} />
        <Button variant="ghost" size="sm" onClick={onClose}>
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
      <p className="text-sm text-white/90 whitespace-pre-wrap leading-relaxed">
        {memory.Content}
      </p>
      {memory.Tags && memory.Tags.length > 0 && (
        <div className="flex gap-1 flex-wrap">
          {memory.Tags.map((t) => (
            <Badge key={t} variant="blue">{t}</Badge>
          ))}
        </div>
      )}
      {memory.Entities && memory.Entities.length > 0 && (
        <div>
          <div className="text-[10px] text-white/25 mb-1">Entities</div>
          <div className="flex gap-1 flex-wrap">
            {memory.Entities.map((e) => (
              <Badge key={e.ID} variant="accent">{e.Name} ({e.Type})</Badge>
            ))}
          </div>
        </div>
      )}
      <div className="grid grid-cols-2 gap-2 text-[10px] text-white/30 pt-2 border-t border-white/8">
        <div>Created: <span className="text-white/45">{formatRelativeTime(memory.CreatedAt)}</span></div>
        <div>Updated: <span className="text-white/45">{formatRelativeTime(memory.UpdatedAt)}</span></div>
        <div>Accessed: <span className="text-white/45">{formatRelativeTime(memory.AccessedAt)}</span></div>
        <div>Access count: <span className="text-white/45">{memory.AccessCount}</span></div>
        <div>Decay: <span className="text-white/45">{memory.DecayScore.toFixed(3)}</span></div>
        <div>Weight: <span className="text-white/45">{memory.ImportanceWeight.toFixed(2)}</span></div>
      </div>
    </div>
  );
}
