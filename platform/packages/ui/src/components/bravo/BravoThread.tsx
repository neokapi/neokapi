import { useRef, useEffect } from "react";
import { cn } from "../../lib/utils";
import type { BravoMessage, BravoToolCall as BravoToolCallType } from "../../types/api";
import { BravoToolCall } from "./BravoToolCall";
import { BravoCodeBlock } from "./BravoCodeBlock";

export interface BravoThreadProps {
  messages: BravoMessage[];
  streaming?: boolean;
  streamingContent?: string;
  /** Tool calls being streamed for the current assistant message. */
  streamingToolCalls?: BravoToolCallType[];
  onApprove?: (toolCallId: string) => void;
  onDeny?: (toolCallId: string) => void;
  onCancel?: () => void;
}

function extractCodeBlocks(content: string): { text: string; lang: string; code: string }[] {
  const parts: { text: string; lang: string; code: string }[] = [];
  const regex = /```(\w*)\n([\s\S]*?)```/g;
  let last = 0;
  let match;
  while ((match = regex.exec(content)) !== null) {
    if (match.index > last) {
      parts.push({ text: content.slice(last, match.index), lang: "", code: "" });
    }
    parts.push({ text: "", lang: match[1] || "text", code: match[2] });
    last = match.index + match[0].length;
  }
  if (last < content.length) {
    parts.push({ text: content.slice(last), lang: "", code: "" });
  }
  return parts.length > 0 ? parts : [{ text: content, lang: "", code: "" }];
}

function MessageBubble({
  message,
  onApprove,
  onDeny,
}: {
  message: BravoMessage;
  onApprove?: (id: string) => void;
  onDeny?: (id: string) => void;
}) {
  const isUser = message.role === "user";
  const parts = extractCodeBlocks(message.content);

  return (
    <div className={cn("flex flex-col gap-1", isUser ? "items-end" : "items-start")}>
      <div className="text-xs font-medium text-muted-foreground px-1">
        {isUser ? "You" : "@bravo"}
      </div>
      <div
        className={cn(
          "max-w-[85%] rounded-lg px-3 py-2 text-sm leading-relaxed",
          isUser ? "bg-primary text-primary-foreground" : "bg-muted text-foreground",
        )}
      >
        {parts.map((part, i) =>
          part.code ? (
            <BravoCodeBlock key={i} language={part.lang} code={part.code} />
          ) : (
            <span key={i} className="whitespace-pre-wrap">
              {part.text}
            </span>
          ),
        )}
      </div>

      {message.tool_calls?.map((tc) => (
        <BravoToolCall
          key={tc.id}
          toolCall={tc}
          onApprove={tc.status === "needs_approval" ? () => onApprove?.(tc.id) : undefined}
          onDeny={tc.status === "needs_approval" ? () => onDeny?.(tc.id) : undefined}
        />
      ))}

      {message.input_tokens || message.output_tokens ? (
        <div className="text-[10px] text-muted-foreground px-1">
          {message.input_tokens?.toLocaleString()} in / {message.output_tokens?.toLocaleString()}{" "}
          out tokens
        </div>
      ) : null}
    </div>
  );
}

export function BravoThread({
  messages,
  streaming,
  streamingContent,
  streamingToolCalls,
  onApprove,
  onDeny,
  onCancel,
}: BravoThreadProps) {
  const bottomRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to bottom when new content arrives.
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, streamingContent, streamingToolCalls]);

  return (
    <div className="flex flex-col gap-4 p-4">
      {messages.length === 0 && !streaming && (
        <div className="py-12 text-center text-sm text-muted-foreground">
          Start a conversation with @bravo
        </div>
      )}

      {messages.map((msg) => (
        <MessageBubble key={msg.id} message={msg} onApprove={onApprove} onDeny={onDeny} />
      ))}

      {streaming && (
        <div className="flex flex-col gap-1 items-start">
          <div className="text-xs font-medium text-muted-foreground px-1">@bravo</div>

          {streamingContent ? (
            <div className="max-w-[85%] rounded-lg bg-muted px-3 py-2 text-sm leading-relaxed whitespace-pre-wrap">
              {streamingContent}
              <span className="inline-block w-1.5 h-4 bg-foreground/60 animate-pulse ml-0.5" />
            </div>
          ) : (
            <div className="max-w-[85%] rounded-lg bg-muted px-3 py-2 text-sm leading-relaxed">
              <span className="inline-block w-1.5 h-4 bg-foreground/60 animate-pulse" />
            </div>
          )}

          {streamingToolCalls?.map((tc) => (
            <BravoToolCall
              key={tc.id}
              toolCall={tc}
              onApprove={tc.status === "needs_approval" ? () => onApprove?.(tc.id) : undefined}
              onDeny={tc.status === "needs_approval" ? () => onDeny?.(tc.id) : undefined}
            />
          ))}

          {onCancel && (
            <button
              onClick={onCancel}
              className="text-xs text-muted-foreground hover:text-foreground transition-colors px-1"
            >
              Stop generating
            </button>
          )}
        </div>
      )}

      <div ref={bottomRef} />
    </div>
  );
}
