import { cn } from "@/lib/cn";

interface ProgressProps {
  value: number;
  max?: number;
  className?: string;
}

export function Progress({ value, max = 100, className }: ProgressProps) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100));
  return (
    <div className={cn("h-1.5 rounded-full bg-white/8 overflow-hidden", className)}>
      <div
        className="h-full rounded-full accent-gradient shadow-sm shadow-accent/30 transition-all duration-300"
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}
