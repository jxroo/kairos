import { useState } from "react";
import { Brain, FileText, Wrench } from "lucide-react";
import { cn } from "@/lib/cn";
import { useMemorySearch } from "@/hooks/useMemories";
import { useTools } from "@/hooks/useTools";
import { useModels } from "@/hooks/useModels";

const tabs = [
  { id: "memories", label: "Memories", icon: Brain },
  { id: "rag", label: "RAG", icon: FileText },
  { id: "tools", label: "Tools", icon: Wrench },
] as const;

type TabId = (typeof tabs)[number]["id"];

export function ContextPanel() {
  const [activeTab, setActiveTab] = useState<TabId>("memories");

  return (
    <div className="flex h-full w-[280px] shrink-0 flex-col glass">
      {/* Tab bar */}
      <div className="flex border-b border-white/8 px-2 pt-2">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            onClick={() => setActiveTab(id)}
            className={cn(
              "flex items-center gap-1.5 px-3 py-2 text-xs rounded-t-lg transition-colors cursor-pointer",
              activeTab === id
                ? "text-white/90 bg-white/5"
                : "text-white/30 hover:text-white/45",
            )}
          >
            <Icon className="h-3.5 w-3.5" />
            {label}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-3">
        {activeTab === "memories" && <MemoriesTab />}
        {activeTab === "rag" && <RAGTab />}
        {activeTab === "tools" && <ToolsTab />}
      </div>

      {/* Model status */}
      <ModelStatus />
    </div>
  );
}

function MemoriesTab() {
  const { data: memories } = useMemorySearch("", 10);

  if (!memories?.length) {
    return <EmptyTab text="No memories yet" />;
  }

  return (
    <div className="space-y-2">
      {memories.map((r) => (
        <div
          key={r.Memory.ID}
          className="rounded-[10px] bg-white/4 border border-white/6 p-2.5 text-xs text-white/45 hover:border-white/12 transition-colors"
        >
          <div className="line-clamp-3">{r.Memory.Content}</div>
          {r.Memory.Tags && r.Memory.Tags.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-1.5">
              {r.Memory.Tags.map((t: string) => (
                <span
                  key={t}
                  className="rounded-md bg-accent/10 text-accent-light/70 px-1.5 py-0.5 text-[9px]"
                >
                  {t}
                </span>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

function RAGTab() {
  return <EmptyTab text="RAG context appears during conversations" />;
}

function ToolsTab() {
  const { data: tools } = useTools();

  if (!tools?.length) {
    return <EmptyTab text="No tools registered" />;
  }

  return (
    <div className="space-y-2">
      {tools.map((t) => (
        <div
          key={t.name}
          className="rounded-[10px] bg-white/4 border border-white/6 p-2.5 hover:border-white/12 transition-colors"
        >
          <div className="text-xs text-white/65 font-medium">{t.name}</div>
          {t.description && (
            <div className="text-[10px] text-white/30 mt-0.5 line-clamp-2">
              {t.description}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

function ModelStatus() {
  const { data } = useModels();
  const models = data?.data || [];

  return (
    <div className="border-t border-white/8 px-3 py-2.5">
      <div className="text-[10px] uppercase tracking-[0.8px] text-white/25 mb-1.5">
        Models
      </div>
      {models.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {models.slice(0, 3).map((m) => (
            <span
              key={m.id}
              className="rounded-md bg-white/5 border border-white/8 px-1.5 py-0.5 text-[10px] text-white/45"
            >
              {m.id}
            </span>
          ))}
          {models.length > 3 && (
            <span className="text-[10px] text-white/25">
              +{models.length - 3}
            </span>
          )}
        </div>
      ) : (
        <span className="text-[10px] text-white/25">No models loaded</span>
      )}
    </div>
  );
}

function EmptyTab({ text }: { text: string }) {
  return (
    <div className="flex items-center justify-center h-32 text-xs text-white/25">
      {text}
    </div>
  );
}
