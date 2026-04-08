import { Plus, MessageSquare, Search } from "lucide-react";
import { useState } from "react";
import { cn } from "@/lib/cn";
import { useConversations } from "@/hooks/useConversations";
import { formatRelativeTime } from "@/lib/format";

interface ChatSidebarProps {
  activeId: string | null;
  onSelect: (id: string) => void;
  onNew: () => void;
}

export function ChatSidebar({ activeId, onSelect, onNew }: ChatSidebarProps) {
  const { data: conversations } = useConversations(30);
  const [search, setSearch] = useState("");

  const filtered = conversations?.filter(
    (c) =>
      !search ||
      (c.title || "").toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="flex h-full w-[230px] shrink-0 flex-col glass">
      {/* Search */}
      <div className="p-3">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-white/25" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search..."
            className="w-full rounded-[10px] bg-white/4 border border-white/8 pl-8 pr-3 py-1.5 text-xs text-white/65 placeholder:text-white/25 glass-input transition-colors"
          />
        </div>
      </div>

      {/* Conversation list */}
      <div className="flex-1 overflow-y-auto px-2">
        {filtered?.map((c) => (
          <button
            key={c.id}
            onClick={() => onSelect(c.id)}
            className={cn(
              "flex w-full items-start gap-2 rounded-[10px] px-3 py-2 text-left transition-colors cursor-pointer mb-0.5",
              activeId === c.id
                ? "bg-white/8 text-white/90"
                : "text-white/45 hover:bg-white/4 hover:text-white/65",
            )}
          >
            <MessageSquare className="h-3.5 w-3.5 shrink-0 mt-0.5" />
            <div className="min-w-0 flex-1">
              <div className="truncate text-xs">
                {c.title || "(untitled)"}
              </div>
              <div className="text-[10px] text-white/18 mt-0.5">
                {formatRelativeTime(c.updated_at)}
              </div>
            </div>
          </button>
        ))}
        {!filtered?.length && (
          <div className="px-3 py-4 text-center text-[10px] text-white/25">
            No conversations yet
          </div>
        )}
      </div>

      {/* New chat button */}
      <div className="p-3 border-t border-white/8">
        <button
          onClick={onNew}
          className="flex w-full items-center justify-center gap-1.5 rounded-[12px] bg-white/5 border border-white/8 px-3 py-2 text-xs text-white/45 hover:text-white/65 hover:bg-white/8 transition-colors cursor-pointer"
        >
          <Plus className="h-3.5 w-3.5" />
          New Chat
        </button>
      </div>
    </div>
  );
}
