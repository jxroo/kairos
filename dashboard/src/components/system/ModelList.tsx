import { Table, Thead, Tbody, Tr, Th, Td } from "@/components/ui/Table";
import { Badge } from "@/components/ui/Badge";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { Empty } from "@/components/ui/Empty";
import { useModels } from "@/hooks/useModels";
import { formatBytes } from "@/lib/format";
import { Cpu } from "lucide-react";

export function ModelList() {
  const { data, isLoading } = useModels();
  const models = data?.data ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Models</CardTitle>
        <span className="text-[10px] text-white/25">{models.length} available</span>
      </CardHeader>
      {isLoading ? (
        <TableSkeleton rows={3} cols={4} />
      ) : models.length === 0 ? (
        <Empty icon={Cpu} title="No models" description="No LLM providers connected" />
      ) : (
        <Table>
          <Thead>
            <Tr>
              <Th>Model ID</Th>
              <Th>Provider</Th>
              <Th>Context</Th>
              <Th>Size</Th>
            </Tr>
          </Thead>
          <Tbody>
            {models.map((m) => (
              <Tr key={m.id}>
                <Td className="text-white/90">{m.id}</Td>
                <Td><Badge variant="blue">{m.owned_by}</Badge></Td>
                <Td>{m.context_length ? `${(m.context_length / 1024).toFixed(0)}K` : "—"}</Td>
                <Td>{m.size_bytes ? formatBytes(m.size_bytes) : "—"}</Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </Card>
  );
}
