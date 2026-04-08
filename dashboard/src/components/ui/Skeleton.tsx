import { cn } from "@/lib/cn";

export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "rounded bg-white/5 animate-shimmer",
        className,
      )}
    />
  );
}

export function TableSkeleton({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div className="space-y-2">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex gap-3">
          {Array.from({ length: cols }).map((_, j) => (
            <Skeleton
              key={j}
              className={cn("h-5", j === 0 ? "flex-[2]" : "flex-1")}
            />
          ))}
        </div>
      ))}
    </div>
  );
}
