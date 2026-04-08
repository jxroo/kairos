import { cn } from "@/lib/cn";
import type { InputHTMLAttributes } from "react";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {}

export function Input({ className, ...props }: InputProps) {
  return (
    <input
      className={cn(
        "w-full rounded-[14px] bg-white/4 border border-white/10 px-3 py-1.5 text-sm text-white/90 placeholder:text-white/30 glass-input transition-colors",
        className,
      )}
      {...props}
    />
  );
}
