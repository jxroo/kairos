import { cn } from "@/lib/cn";
import type { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
}

const variants: Record<Variant, string> = {
  primary:
    "accent-gradient text-white hover:brightness-110 active:brightness-90 shadow-lg shadow-accent/15",
  secondary:
    "glass text-white/65 hover:text-white/90 hover:border-white/18",
  ghost: "text-white/45 hover:text-white/65 hover:bg-white/5",
  danger: "bg-red-500/15 text-red-400/80 hover:bg-red-500/25 border border-red-500/20",
};

const sizes: Record<Size, string> = {
  sm: "px-2.5 py-1 text-xs",
  md: "px-3.5 py-1.5 text-sm",
};

export function Button({
  variant = "secondary",
  size = "md",
  className,
  ...props
}: ButtonProps) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-1.5 rounded-[14px] font-medium transition-all duration-200 disabled:opacity-40 disabled:pointer-events-none cursor-pointer",
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  );
}
