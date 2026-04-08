import type { ChatMessage, ToolCallInfo } from "./types";

export interface ChatStreamEvent {
  type: "content" | "tool_call" | "tool_result" | "done" | "error";
  content?: string;
  toolCall?: ToolCallInfo;
  toolResult?: string;
  error?: string;
}

export async function* streamChat(
  model: string,
  messages: ChatMessage[],
  conversationId?: string,
): AsyncGenerator<ChatStreamEvent> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (conversationId) {
    headers["X-Conversation-Id"] = conversationId;
  }

  const res = await fetch("/v1/chat/completions", {
    method: "POST",
    headers,
    body: JSON.stringify({
      model,
      messages,
      stream: true,
    }),
  });

  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      // ignore
    }
    yield { type: "error", error: msg };
    return;
  }

  // Read X-Conversation-Id from response header.
  const newConvId = res.headers.get("X-Conversation-Id");
  if (newConvId) {
    yield { type: "content", content: "", conversationId: newConvId } as ChatStreamEvent & { conversationId: string };
  }

  const reader = res.body?.getReader();
  if (!reader) {
    yield { type: "error", error: "No response body" };
    return;
  }

  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (const line of lines) {
      if (!line.startsWith("data: ")) continue;
      const payload = line.slice(6);
      if (payload === "[DONE]") {
        yield { type: "done" };
        return;
      }

      try {
        const data = JSON.parse(payload);

        if (data.object === "kairos.tool_call") {
          yield { type: "tool_call", toolCall: data.tool_call };
          continue;
        }

        if (data.object === "kairos.tool_result") {
          yield { type: "tool_result", toolResult: data.content };
          continue;
        }

        if (data.object === "error") {
          yield { type: "error", error: data.error };
          continue;
        }

        // Standard chat.completion.chunk
        if (data.choices?.[0]?.delta?.content) {
          yield { type: "content", content: data.choices[0].delta.content };
        }
      } catch {
        // skip malformed JSON
      }
    }
  }

  yield { type: "done" };
}

export function getConversationIdFromHeaders(res: Response): string | null {
  return res.headers.get("X-Conversation-Id");
}
