import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Skeleton } from "@/components/ui/Skeleton";
import { useHealth } from "@/hooks/useHealth";
import { useModels } from "@/hooks/useModels";
import { useTools } from "@/hooks/useTools";

export function HealthCard() {
  const { data, isError, isLoading } = useHealth();
  const { data: modelsData } = useModels();
  const { data: tools } = useTools();

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <Skeleton className="h-8 w-16 mb-2" />
            <Skeleton className="h-3 w-20" />
          </Card>
        ))}
      </div>
    );
  }

  const online = !!data && !isError;
  const models = modelsData?.data ?? [];

  const stats = [
    {
      label: "Status",
      value: online ? "Online" : "Offline",
      badge: true,
      variant: online ? "green" as const : "red" as const,
    },
    {
      label: "Models",
      value: String(models.length),
    },
    {
      label: "Tools",
      value: String(tools?.length ?? 0),
    },
    {
      label: "Uptime",
      value: data?.uptime ?? "—",
    },
  ];

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
      {stats.map((s) => (
        <Card key={s.label} className="text-center">
          {s.badge ? (
            <div className="mb-1">
              <Badge variant={s.variant}>{s.value}</Badge>
            </div>
          ) : (
            <div className="text-[28px] font-light text-white/90 tracking-[-1.5px] mb-1">
              {s.value}
            </div>
          )}
          <div className="text-[10px] uppercase tracking-[0.8px] text-white/30">
            {s.label}
          </div>
        </Card>
      ))}
    </div>
  );
}
