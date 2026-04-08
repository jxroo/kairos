import { cn } from "@/lib/cn";
import { createContext, useContext, useState, type ReactNode } from "react";

const TabsContext = createContext<{
  value: string;
  onChange: (v: string) => void;
}>({ value: "", onChange: () => {} });

export function Tabs({
  defaultValue,
  children,
  className,
}: {
  defaultValue: string;
  children: ReactNode;
  className?: string;
}) {
  const [value, setValue] = useState(defaultValue);
  return (
    <TabsContext.Provider value={{ value, onChange: setValue }}>
      <div className={className}>{children}</div>
    </TabsContext.Provider>
  );
}

export function TabsList({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        "inline-flex items-center gap-0.5 rounded-[12px] bg-white/4 border border-white/8 p-0.5",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function TabsTrigger({
  value,
  children,
  className,
}: {
  value: string;
  children: ReactNode;
  className?: string;
}) {
  const ctx = useContext(TabsContext);
  const active = ctx.value === value;
  return (
    <button
      onClick={() => ctx.onChange(value)}
      className={cn(
        "px-3 py-1 text-xs rounded-[10px] transition-colors cursor-pointer",
        active
          ? "bg-white/10 text-white/90 shadow-sm"
          : "text-white/45 hover:text-white/65",
        className,
      )}
    >
      {children}
    </button>
  );
}

export function TabsContent({
  value,
  children,
  className,
}: {
  value: string;
  children: ReactNode;
  className?: string;
}) {
  const ctx = useContext(TabsContext);
  if (ctx.value !== value) return null;
  return <div className={cn("animate-fade-in-up", className)}>{children}</div>;
}
