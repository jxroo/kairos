import { cn } from "@/lib/cn";
import type { ReactNode } from "react";

type Variant = "default" | "accent" | "green" | "red" | "blue";

interface BadgeProps {
  variant?: Variant;
  children: ReactNode;
  className?: string;
}

const variants: Record<Variant, string> = {
  default: "bg-white/5 text-white/45 border-white/10",
  accent: "bg-accent/15 text-accent-light border-accent/20",
  green: "bg-green-500/15 text-green-400/80 border-green-500/20",
  red: "bg-red-500/15 text-red-400/80 border-red-500/20",
  blue: "bg-accent/15 text-accent-light border-accent/20",
};

export function Badge({ variant = "default", children, className }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-medium border leading-none",
        variants[variant],
        className,
      )}
    >
      {children}
    </span>
  );
}
