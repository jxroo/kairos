import { Table, Thead, Tbody, Tr, Th, Td } from "@/components/ui/Table";
import { Badge } from "@/components/ui/Badge";
import { truncate } from "@/lib/format";
import type { ChunkSearchResult } from "@/api/types";

interface DocumentListProps {
  results: ChunkSearchResult[];
  onSelect: (result: ChunkSearchResult) => void;
}

export function DocumentList({ results, onSelect }: DocumentListProps) {
  return (
    <Table>
      <Thead>
        <Tr>
          <Th>Filename</Th>
          <Th>Type</Th>
          <Th>Path</Th>
          <Th>Score</Th>
        </Tr>
      </Thead>
      <Tbody>
        {results.map((r) => (
          <Tr
            key={r.Chunk.ID}
            className="cursor-pointer"
            onClick={() => onSelect(r)}
          >
            <Td className="text-white/90 whitespace-nowrap">{r.Document.Filename}</Td>
            <Td><Badge>{r.Document.Extension}</Badge></Td>
            <Td>{truncate(r.Document.Path, 50)}</Td>
            <Td>
              <span className="text-accent-light">{(r.FinalScore * 100).toFixed(1)}%</span>
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}
