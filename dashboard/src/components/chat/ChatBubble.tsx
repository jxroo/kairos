import { cn } from "@/lib/cn";
import type { UIMessage } from "@/hooks/useChat";

export function ChatBubble({ message }: { message: UIMessage }) {
  const { role, content, isStreaming } = message;

  if (role === "tool") {
    return (
      <div className="flex justify-start mb-2">
        <div className="max-w-[75%] rounded-xl border border-accent/20 bg-accent/5 px-3 py-1.5 text-[11px] font-mono text-white/45">
          <div className="whitespace-pre-wrap break-words">{content}</div>
        </div>
      </div>
    );
  }

  const isUser = role === "user";

  return (
    <div className={cn("flex mb-3", isUser ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "max-w-[75%] px-3.5 py-2.5 text-sm leading-relaxed",
          isUser
            ? "bg-accent/20 rounded-2xl rounded-br-md text-white/90"
            : "bg-white/5 rounded-2xl rounded-bl-md text-white/65",
        )}
      >
        <div className="whitespace-pre-wrap break-words">
          {content}
          {isStreaming && (
            <span className="inline-block w-1.5 h-4 bg-accent animate-pulse ml-0.5 align-text-bottom rounded-sm" />
          )}
        </div>
      </div>
    </div>
  );
}
