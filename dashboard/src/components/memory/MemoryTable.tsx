import { Table, Thead, Tbody, Tr, Th, Td } from "@/components/ui/Table";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Progress } from "@/components/ui/Progress";
import { ImportanceBadge } from "./ImportanceBadge";
import { formatRelativeTime, truncate } from "@/lib/format";
import { Trash2 } from "lucide-react";
import type { SearchResult } from "@/api/types";

interface MemoryTableProps {
  results: SearchResult[];
  onSelect: (id: string) => void;
  onDelete: (id: string) => void;
}

export function MemoryTable({ results, onSelect, onDelete }: MemoryTableProps) {
  return (
    <Table>
      <Thead>
        <Tr>
          <Th className="w-[40%]">Content</Th>
          <Th>Importance</Th>
          <Th>Decay</Th>
          <Th>Tags</Th>
          <Th>Created</Th>
          <Th className="w-8"></Th>
        </Tr>
      </Thead>
      <Tbody>
        {results.map(({ Memory: m }) => (
          <Tr
            key={m.ID}
            className="cursor-pointer"
            onClick={() => onSelect(m.ID)}
          >
            <Td className="text-white/90">{truncate(m.Content, 80)}</Td>
            <Td><ImportanceBadge importance={m.Importance} /></Td>
            <Td>
              <div className="flex items-center gap-1.5 min-w-[60px]">
                <Progress value={m.DecayScore * 100} className="flex-1" />
                <span className="text-[10px] text-white/25">{(m.DecayScore * 100).toFixed(0)}%</span>
              </div>
            </Td>
            <Td>
              <div className="flex gap-1 flex-wrap">
                {m.Tags?.slice(0, 3).map((t) => (
                  <Badge key={t} variant="accent">{t}</Badge>
                ))}
              </div>
            </Td>
            <Td className="text-white/30 whitespace-nowrap">{formatRelativeTime(m.CreatedAt)}</Td>
            <Td>
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => { e.stopPropagation(); onDelete(m.ID); }}
              >
                <Trash2 className="h-3 w-3 text-white/25 hover:text-red-400" />
              </Button>
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}
