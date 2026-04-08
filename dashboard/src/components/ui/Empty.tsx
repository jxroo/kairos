import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

interface EmptyProps {
  icon: LucideIcon;
  title: string;
  description?: string;
  action?: ReactNode;
}

export function Empty({ icon: Icon, title, description, action }: EmptyProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <Icon className="h-10 w-10 text-white/25 mb-3" strokeWidth={1.5} />
      <h3 className="text-sm font-medium text-white/45 mb-1">
        {title}
      </h3>
      {description && (
        <p className="text-xs text-white/25 max-w-xs">{description}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
