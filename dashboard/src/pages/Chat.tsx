import { useState } from "react";
import { useChat } from "@/hooks/useChat";
import { ChatSidebar } from "@/components/chat/ChatSidebar";
import { ChatMessages } from "@/components/chat/ChatMessages";
import { ChatInput } from "@/components/chat/ChatInput";
import { ModelSelector } from "@/components/chat/ModelSelector";
import { ContextPanel } from "@/components/chat/ContextPanel";
import { PanelRightClose, PanelRightOpen } from "lucide-react";

export function Chat() {
  const {
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
  } = useChat();

  const [contextOpen, setContextOpen] = useState(true);

  return (
    <div className="flex gap-3 h-[calc(100vh-84px)]">
      {/* Sidebar */}
      <ChatSidebar
        activeId={conversationId}
        onSelect={loadConversation}
        onNew={newConversation}
      />

      {/* Main chat */}
      <div className="flex flex-1 flex-col min-w-0 glass-elevated">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-white/8">
          <span className="text-[13px] font-medium text-white/65">
            {conversationId ? "Conversation" : "New Chat"}
          </span>
          <div className="flex items-center gap-2">
            <ModelSelector value={model} onChange={setModel} />
            <button
              onClick={() => setContextOpen(!contextOpen)}
              className="flex h-7 w-7 items-center justify-center rounded-lg text-white/30 hover:text-white/65 hover:bg-white/5 transition-colors cursor-pointer"
            >
              {contextOpen ? (
                <PanelRightClose className="h-4 w-4" />
              ) : (
                <PanelRightOpen className="h-4 w-4" />
              )}
            </button>
          </div>
        </div>

        {/* Messages */}
        <ChatMessages messages={messages} activeToolCalls={activeToolCalls} />

        {/* Error */}
        {error && (
          <div className="mx-4 mb-2 rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-1.5 text-xs text-red-400/80">
            {error}
          </div>
        )}

        {/* Input */}
        <ChatInput onSend={sendMessage} disabled={isStreaming} />
      </div>

      {/* Context Panel */}
      {contextOpen && <ContextPanel />}
    </div>
  );
}
