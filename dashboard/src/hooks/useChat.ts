import { useState, useCallback, useRef } from "react";
import { streamChat, type ChatStreamEvent } from "@/api/chat";
import { getConversation } from "@/api/conversations";
import type { ChatMessage, ToolCallInfo } from "@/api/types";

export interface UIMessage {
  id: string;
  role: "user" | "assistant" | "tool";
  content: string;
  toolCalls?: ToolCallInfo[];
  toolName?: string;
  isStreaming?: boolean;
}

let msgCounter = 0;
function nextId() {
  return `msg-${Date.now()}-${++msgCounter}`;
}

export function useChat() {
  const [messages, setMessages] = useState<UIMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [activeToolCalls, setActiveToolCalls] = useState<ToolCallInfo[]>([]);
  const [conversationId, setConversationId] = useState<string | null>(null);
  const [model, setModel] = useState("");
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const sendMessage = useCallback(
    async (content: string) => {
      if (!content.trim() || isStreaming) return;

      setError(null);

      const userMsg: UIMessage = {
        id: nextId(),
        role: "user",
        content: content.trim(),
      };
      setMessages((prev) => [...prev, userMsg]);

      // Build messages for API.
      const apiMessages: ChatMessage[] = [
        { role: "user", content: content.trim() },
      ];

      const assistantId = nextId();
      setMessages((prev) => [
        ...prev,
        { id: assistantId, role: "assistant", content: "", isStreaming: true },
      ]);

      setIsStreaming(true);
      setActiveToolCalls([]);

      try {
        for await (const event of streamChat(model, apiMessages, conversationId || undefined)) {
          switch (event.type) {
            case "content": {
              // Check for conversation ID in first event.
              const evt = event as ChatStreamEvent & { conversationId?: string };
              if (evt.conversationId && !conversationId) {
                setConversationId(evt.conversationId);
              }
              if (event.content) {
                setMessages((prev) =>
                  prev.map((m) =>
                    m.id === assistantId
                      ? { ...m, content: m.content + event.content }
                      : m,
                  ),
                );
              }
              break;
            }

            case "tool_call": {
              const tc = event.toolCall;
              if (tc) {
                setActiveToolCalls((prev) => [...prev, tc]);
                const toolMsg: UIMessage = {
                  id: nextId(),
                  role: "tool",
                  content: `Calling ${tc.function.name}...`,
                  toolName: tc.function.name,
                  toolCalls: [tc],
                };
                setMessages((prev) => [...prev, toolMsg]);
              }
              break;
            }

            case "tool_result": {
              const result = event.toolResult;
              if (result) {
                setActiveToolCalls((prev) => prev.slice(1));
                setMessages((prev) =>
                  prev.map((m, i) => {
                    // Find the last tool message that's still pending.
                    if (
                      m.role === "tool" &&
                      m.content.startsWith("Calling ") &&
                      !prev.slice(i + 1).some((n) => n.role === "tool" && n.content.startsWith("Calling "))
                    ) {
                      return { ...m, content: result };
                    }
                    return m;
                  }),
                );
              }
              break;
            }

            case "error":
              setError(event.error || "Unknown error");
              break;

            case "done":
              break;
          }
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "Stream failed");
      } finally {
        setIsStreaming(false);
        setActiveToolCalls([]);
        setMessages((prev) =>
          prev.map((m) =>
            m.id === assistantId ? { ...m, isStreaming: false } : m,
          ),
        );
      }
    },
    [model, conversationId, isStreaming],
  );

  const loadConversation = useCallback(
    async (id: string) => {
      try {
        const detail = await getConversation(id);
        setConversationId(id);
        if (detail.model) setModel(detail.model);
        const uiMsgs: UIMessage[] = detail.messages
          .filter((m) => m.role !== "system")
          .map((m) => ({
            id: m.id,
            role: m.role as UIMessage["role"],
            content: m.content,
          }));
        setMessages(uiMsgs);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load conversation");
      }
    },
    [],
  );

  const newConversation = useCallback(() => {
    if (abortRef.current) abortRef.current.abort();
    setMessages([]);
    setConversationId(null);
    setError(null);
    setIsStreaming(false);
    setActiveToolCalls([]);
  }, []);

  return {
    messages,
    isStreaming,
    activeToolCalls,
    conversationId,
    model,
    error,
    sendMessage,
    setModel,
    loadConversation,
    newConversation,
  };
}
