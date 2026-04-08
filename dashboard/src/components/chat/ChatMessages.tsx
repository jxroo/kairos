import { useEffect, useRef } from "react";
import { MessageSquare } from "lucide-react";
import { ChatBubble } from "./ChatBubble";
import { ToolCallIndicator } from "./ToolCallIndicator";
import type { UIMessage } from "@/hooks/useChat";
import type { ToolCallInfo } from "@/api/types";

interface ChatMessagesProps {
  messages: UIMessage[];
  activeToolCalls: ToolCallInfo[];
}

export function ChatMessages({ messages, activeToolCalls }: ChatMessagesProps) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, activeToolCalls]);

  if (messages.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="text-center">
          <MessageSquare className="h-10 w-10 text-white/15 mx-auto mb-3" />
          <p className="text-sm text-white/45">Start a conversation</p>
          <p className="text-xs text-white/25 mt-1">
            Messages are enriched with your memories and documents
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto px-5 py-4">
      {messages.map((msg) => (
        <ChatBubble key={msg.id} message={msg} />
      ))}
      <ToolCallIndicator toolCalls={activeToolCalls} />
      <div ref={bottomRef} />
    </div>
  );
}
