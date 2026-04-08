import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Progress } from "@/components/ui/Progress";
import { Skeleton } from "@/components/ui/Skeleton";
import { useIndexStatus } from "@/hooks/useIndexStatus";
import { rebuildIndex } from "@/api/rag";
import { useState } from "react";
import { RefreshCw, Database } from "lucide-react";

export function IndexStatusCard() {
  const { data: status, isLoading } = useIndexStatus();
  const [rebuilding, setRebuilding] = useState(false);

  const handleRebuild = async () => {
    setRebuilding(true);
    try {
      await rebuildIndex();
    } finally {
      setTimeout(() => setRebuilding(false), 2000);
    }
  };

  if (isLoading) {
    return (
      <Card>
        <CardHeader><CardTitle>Index Status</CardTitle></CardHeader>
        <div className="space-y-2"><Skeleton className="h-5 w-32" /><Skeleton className="h-2 w-full" /><Skeleton className="h-4 w-48" /></div>
      </Card>
    );
  }

  const stateVariant = status?.state === "idle" ? "green" : status?.state === "indexing" ? "accent" : "default";

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Database className="h-4 w-4 text-white/30" />
          <CardTitle>Index Status</CardTitle>
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={handleRebuild}
          disabled={rebuilding || status?.state === "indexing"}
        >
          <RefreshCw className={`h-3 w-3 ${status?.state === "indexing" ? "animate-spin" : ""}`} />
          <span className="max-sm:hidden">Rebuild</span>
        </Button>
      </CardHeader>
      {status && (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Badge variant={stateVariant}>{status.state}</Badge>
            {status.state === "indexing" && (
              <span className="text-[10px] text-white/30">{status.percent}%</span>
            )}
          </div>
          <Progress value={status.percent} />
          <div className="grid grid-cols-3 gap-3">
            <div className="text-center">
              <div className="text-[24px] font-light text-white/90 tracking-[-1.5px]">{status.total_files}</div>
              <div className="text-[10px] uppercase tracking-[0.8px] text-white/30">Total</div>
            </div>
            <div className="text-center">
              <div className="text-[24px] font-light text-green-400/80 tracking-[-1.5px]">{status.indexed_files}</div>
              <div className="text-[10px] uppercase tracking-[0.8px] text-white/30">Indexed</div>
            </div>
            <div className="text-center">
              <div className="text-[24px] font-light text-error tracking-[-1.5px]">{status.failed_files}</div>
              <div className="text-[10px] uppercase tracking-[0.8px] text-white/30">Failed</div>
            </div>
          </div>
        </div>
      )}
    </Card>
  );
}
