import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Table, Tbody, Td, Th, Thead, Tr } from "@/components/ui/Table";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { Empty } from "@/components/ui/Empty";
import { Badge } from "@/components/ui/Badge";
import { useDocuments } from "@/hooks/useDocuments";
import { Files } from "lucide-react";
import { formatRelativeTime, truncate } from "@/lib/format";

export function IndexedDocumentsCard() {
  const { data: documents, isLoading } = useDocuments(100);

  return (
    <Card className="p-0 overflow-hidden">
      <CardHeader className="px-5 pt-5">
        <CardTitle>Indexed Files</CardTitle>
        <span className="text-[10px] text-white/25">{documents?.length ?? 0} tracked</span>
      </CardHeader>
      {isLoading ? (
        <div className="p-5">
          <TableSkeleton rows={5} cols={4} />
        </div>
      ) : !documents?.length ? (
        <div className="p-6">
          <Empty icon={Files} title="No indexed files" description="Add watch paths or trigger a rebuild to populate the index" />
        </div>
      ) : (
        <Table>
          <Thead>
            <Tr>
              <Th>Filename</Th>
              <Th>Status</Th>
              <Th>Path</Th>
              <Th>Updated</Th>
            </Tr>
          </Thead>
          <Tbody>
            {documents.map((doc) => (
              <Tr key={doc.ID}>
                <Td className="text-white/90 whitespace-nowrap">{doc.Filename}</Td>
                <Td>
                  <Badge variant={doc.Status === "indexed" ? "green" : doc.Status === "error" ? "red" : "accent"}>
                    {doc.Status}
                  </Badge>
                </Td>
                <Td>{truncate(doc.Path, 60)}</Td>
                <Td className="whitespace-nowrap">{formatRelativeTime(doc.UpdatedAt)}</Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </Card>
  );
}
