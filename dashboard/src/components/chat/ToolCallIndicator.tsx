import { Wrench } from "lucide-react";
import type { ToolCallInfo } from "@/api/types";

const toolLabels: Record<string, string> = {
  kairos_remember: "saving to memory",
  kairos_recall: "searching memories",
  kairos_search_files: "searching files",
};

export function ToolCallIndicator({ toolCalls }: { toolCalls: ToolCallInfo[] }) {
  if (toolCalls.length === 0) return null;

  return (
    <div className="flex flex-col gap-1 my-2">
      {toolCalls.map((tc) => (
        <div
          key={tc.id}
          className="flex items-center gap-2 rounded-xl bg-accent/5 border border-accent/15 px-3 py-1.5 text-xs text-accent-light"
        >
          <Wrench className="h-3.5 w-3.5 animate-spin" />
          <span>
            {tc.function.name}
            <span className="text-white/30 ml-1.5">
              {toolLabels[tc.function.name] || "executing"}...
            </span>
          </span>
        </div>
      ))}
    </div>
  );
}
