import { Table, Thead, Tbody, Tr, Th, Td } from "@/components/ui/Table";
import { Badge } from "@/components/ui/Badge";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { Empty } from "@/components/ui/Empty";
import { useTools } from "@/hooks/useTools";
import { truncate } from "@/lib/format";
import { Wrench } from "lucide-react";

export function ToolsList() {
  const { data: tools, isLoading } = useTools();

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tools</CardTitle>
        <span className="text-[10px] text-white/25">{tools?.length ?? 0} registered</span>
      </CardHeader>
      {isLoading ? (
        <TableSkeleton rows={4} cols={4} />
      ) : !tools?.length ? (
        <Empty icon={Wrench} title="No tools" description="No tools registered" />
      ) : (
        <Table>
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Description</Th>
              <Th>Permissions</Th>
              <Th>Type</Th>
            </Tr>
          </Thead>
          <Tbody>
            {tools.map((t) => (
              <Tr key={t.name}>
                <Td className="text-white/90 whitespace-nowrap">{t.name}</Td>
                <Td>{truncate(t.description, 60)}</Td>
                <Td>
                  <div className="flex gap-1 flex-wrap">
                    {t.permissions?.map((p) => (
                      <Badge key={p.resource} variant={p.allow ? "accent" : "red"}>
                        {p.resource}
                      </Badge>
                    ))}
                  </div>
                </Td>
                <Td>{t.builtin ? <Badge variant="green">builtin</Badge> : <Badge>custom</Badge>}</Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </Card>
  );
}
