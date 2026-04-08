import { Badge } from "@/components/ui/Badge";
import { cn } from "@/lib/cn";

export function ImportanceBadge({ importance }: { importance: string }) {
  const variant = importance === "high" ? "accent" : "default";
  return (
    <Badge
      variant={variant}
      className={cn(importance === "high" && "glow-accent", importance === "low" && "opacity-50")}
    >
      {importance}
    </Badge>
  );
}
