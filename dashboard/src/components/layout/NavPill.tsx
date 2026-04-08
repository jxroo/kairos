import { NavLink } from "react-router-dom";
import { cn } from "@/lib/cn";
import { useHealth } from "@/hooks/useHealth";

const links = [
  { to: "/", label: "Chat" },
  { to: "/system", label: "System" },
  { to: "/memories", label: "Memories" },
  { to: "/rag", label: "RAG" },
] as const;

export function NavPill() {
  const { data: health, isError } = useHealth();
  const online = !!health && !isError;

  return (
    <nav className="fixed top-3 left-1/2 -translate-x-1/2 z-50 glass flex items-center gap-0 px-1.5 py-1.5 rounded-full">
      {/* Brand */}
      <div className="flex items-center gap-2 px-3 border-r border-white/10">
        <div className="flex h-6 w-6 items-center justify-center rounded-lg accent-gradient text-white font-bold text-xs shrink-0">
          K
        </div>
        <span className="text-[13px] font-semibold text-white/90 tracking-[-0.3px] pr-2">
          Kairos
        </span>
      </div>

      {/* Nav links */}
      <div className="flex items-center gap-0.5 px-2">
        {links.map(({ to, label }) => (
          <NavLink
            key={to}
            to={to}
            end={to === "/"}
            className={({ isActive }) =>
              cn(
                "px-3 py-1 text-xs font-medium rounded-full transition-colors",
                isActive
                  ? "bg-white/10 text-white/90"
                  : "text-white/45 hover:text-white/65",
              )
            }
          >
            {label}
          </NavLink>
        ))}
      </div>

      {/* Status */}
      <div className="flex items-center gap-2 px-3 border-l border-white/10">
        <div
          className={cn(
            "h-2 w-2 rounded-full",
            online
              ? "bg-cyan animate-status-glow"
              : "bg-red-500",
          )}
        />
        <span className="text-[10px] text-white/30">
          {online ? "Online" : "Offline"}
        </span>
      </div>
    </nav>
  );
}
