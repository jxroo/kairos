import { useState, useRef, useCallback, type KeyboardEvent } from "react";
import { Send } from "lucide-react";
import { cn } from "@/lib/cn";

interface ChatInputProps {
  onSend: (content: string) => void;
  disabled?: boolean;
}

export function ChatInput({ onSend, disabled }: ChatInputProps) {
  const [value, setValue] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleSend = useCallback(() => {
    const trimmed = value.trim();
    if (!trimmed || disabled) return;
    onSend(trimmed);
    setValue("");
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
  }, [value, disabled, onSend]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend],
  );

  const handleInput = useCallback(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 200) + "px";
  }, []);

  return (
    <div className="flex items-end gap-2 px-4 py-3 border-t border-white/8">
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => {
          setValue(e.target.value);
          handleInput();
        }}
        onKeyDown={handleKeyDown}
        placeholder="Send a message..."
        disabled={disabled}
        rows={1}
        className={cn(
          "flex-1 resize-none rounded-[14px] bg-white/4 border border-white/10 px-3 py-2 text-sm text-white/90",
          "placeholder:text-white/30 glass-input",
          "transition-colors disabled:opacity-40",
        )}
      />
      <button
        onClick={handleSend}
        disabled={disabled || !value.trim()}
        className={cn(
          "flex h-9 w-9 shrink-0 items-center justify-center rounded-xl transition-all cursor-pointer",
          "accent-gradient text-white shadow-lg shadow-accent/15",
          "hover:brightness-110 active:brightness-90",
          "disabled:opacity-40 disabled:pointer-events-none",
        )}
      >
        <Send className="h-4 w-4" />
      </button>
    </div>
  );
}
