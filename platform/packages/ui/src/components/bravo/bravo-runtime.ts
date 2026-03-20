import { useMemo } from "react";
import {
  useExternalStoreRuntime,
  type ThreadMessageLike,
  type ExternalStoreAdapter,
  type ExternalStoreThreadListAdapter,
  type TextMessagePart,
} from "@assistant-ui/react";
import type { BravoMessage, BravoToolCall } from "../../types/api";
import type { BravoMode } from "./BravoModeSelector";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface BravoRuntimeOptions {
  /** All messages for the active conversation (committed + in-flight). */
  messages: BravoMessage[];
  /** Whether an assistant response is currently streaming. */
  streaming: boolean;
  /** Partial text streamed so far for the in-flight assistant message. */
  streamingContent: string;
  /** Tool calls accumulated during the current streaming response. */
  streamingToolCalls: BravoToolCall[];
  /** Send a user message (triggers SSE streaming in BravoContext). */
  onSendMessage: (content: string) => Promise<void>;
  /** Cancel the current streaming response. */
  onCancel: () => void;
  /** Approve a tool call that requires human approval. */
  onApproveToolCall: (toolCallId: string) => Promise<void>;
  /** Deny a tool call that requires human approval. */
  onDenyToolCall: (toolCallId: string) => Promise<void>;
  /** Current interaction mode. */
  mode: BravoMode;
}

export interface BravoThreadListOptions {
  conversations: Array<{ id: string; title: string; status: string }>;
  activeConversationId: string | undefined;
  onNewConversation: () => Promise<void>;
  onSelectConversation: (id: string) => Promise<void>;
  onDeleteConversation: (id: string) => Promise<void>;
}

// ---------------------------------------------------------------------------
// We build content as a plain array and cast to ThreadMessageLike["content"]
// at the end to satisfy the readonly constraint.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Message converter: BravoMessage → ThreadMessageLike
// ---------------------------------------------------------------------------

function convertBravoMessage(msg: BravoMessage): ThreadMessageLike {
  const role = msg.role === "tool" ? "assistant" : (msg.role as "user" | "assistant" | "system");

  // Build content — either a string or an array of typed parts.
  let content: ThreadMessageLike["content"] = msg.content;

  // For assistant messages with tool calls, build a structured content array.
  if (role === "assistant" && msg.tool_calls?.length) {
    const parts: Array<
      | { readonly type: "text"; readonly text: string }
      | {
          readonly type: "tool-call";
          readonly toolCallId: string;
          readonly toolName: string;
          readonly args?: Record<string, string | number | boolean | null>;
          readonly result?: unknown;
          readonly isError?: boolean;
        }
    > = [];

    if (msg.content) {
      parts.push({ type: "text", text: msg.content });
    }

    for (const tc of msg.tool_calls) {
      parts.push({
        type: "tool-call",
        toolCallId: tc.id,
        toolName: tc.tool_name,
        args: tc.input as Record<string, string | number | boolean | null>,
        result: tc.output ?? (tc.error ? { error: tc.error } : undefined),
        isError: tc.status === "failed" || tc.status === "denied",
      });
    }
    content = parts as ThreadMessageLike["content"];
  }

  return {
    role,
    id: msg.id,
    content,
    createdAt: new Date(msg.created_at),
    status:
      role === "assistant"
        ? { type: "complete" as const, reason: "stop" as const }
        : undefined,
    metadata: {
      custom: {
        input_tokens: msg.input_tokens,
        output_tokens: msg.output_tokens,
        tool_calls_raw: msg.tool_calls,
      },
    },
  };
}

// ---------------------------------------------------------------------------
// Build the "in-flight" streaming message that assistant-ui will display
// while the SSE stream is active.
// ---------------------------------------------------------------------------

function buildStreamingMessage(
  streamingContent: string,
  streamingToolCalls: BravoToolCall[],
): ThreadMessageLike {
  const parts: Array<
    | { readonly type: "text"; readonly text: string }
    | {
        readonly type: "tool-call";
        readonly toolCallId: string;
        readonly toolName: string;
        readonly args?: Record<string, string | number | boolean | null>;
        readonly result?: unknown;
        readonly isError?: boolean;
      }
  > = [];

  if (streamingContent) {
    parts.push({ type: "text", text: streamingContent });
  }

  for (const tc of streamingToolCalls) {
    parts.push({
      type: "tool-call",
      toolCallId: tc.id,
      toolName: tc.tool_name,
      args: tc.input as Record<string, string | number | boolean | null>,
      result: tc.output ?? (tc.error ? { error: tc.error } : undefined),
      isError: tc.status === "failed" || tc.status === "denied",
    });
  }

  return {
    role: "assistant",
    id: "__streaming__",
    content: parts.length > 0 ? (parts as ThreadMessageLike["content"]) : "",
    status: { type: "running" as const },
    metadata: {
      custom: {
        tool_calls_raw: streamingToolCalls,
      },
    },
  };
}

// ---------------------------------------------------------------------------
// Hook: useBravoRuntime
// ---------------------------------------------------------------------------

export function useBravoRuntime(opts: BravoRuntimeOptions) {
  const {
    messages: bravoMessages,
    streaming,
    streamingContent,
    streamingToolCalls,
    onSendMessage,
    onCancel,
    onApproveToolCall,
    onDenyToolCall,
  } = opts;

  // Build the full message list for assistant-ui, appending the in-flight
  // streaming message when active.
  const allMessages = useMemo(() => {
    const converted = bravoMessages.map(convertBravoMessage);
    if (streaming) {
      converted.push(buildStreamingMessage(streamingContent, streamingToolCalls));
    }
    return converted;
  }, [bravoMessages, streaming, streamingContent, streamingToolCalls]);

  const adapter: ExternalStoreAdapter<ThreadMessageLike> = useMemo(
    () => ({
      isRunning: streaming,
      messages: allMessages,
      convertMessage: (msg: ThreadMessageLike) => msg,
      onNew: async (message) => {
        // Extract text from the AppendMessage content parts.
        const textParts = message.content.filter(
          (p): p is TextMessagePart => p.type === "text",
        );
        const text = textParts.map((p) => p.text).join("\n");
        if (text.trim()) {
          await onSendMessage(text);
        }
      },
      onCancel: async () => {
        onCancel();
      },
      onAddToolResult: async ({ toolCallId, result }) => {
        // The approval/denial is handled via the custom tool UI — this is
        // a fallback for assistant-ui's built-in tool result flow.
        if (result === "approved") {
          await onApproveToolCall(toolCallId);
        } else if (result === "denied") {
          await onDenyToolCall(toolCallId);
        }
      },
    }),
    [streaming, allMessages, onSendMessage, onCancel, onApproveToolCall, onDenyToolCall],
  );

  return useExternalStoreRuntime(adapter);
}

// ---------------------------------------------------------------------------
// Hook: useBravoThreadListAdapter
// ---------------------------------------------------------------------------

export function useBravoThreadListAdapter(
  opts: BravoThreadListOptions,
): ExternalStoreThreadListAdapter {
  const { conversations, activeConversationId, onNewConversation, onSelectConversation, onDeleteConversation } = opts;

  return useMemo(
    (): ExternalStoreThreadListAdapter => ({
      threadId: activeConversationId,
      threads: conversations
        .filter((c) => c.status === "active")
        .map((c) => ({
          id: c.id,
          status: "regular" as const,
          title: c.title || "Untitled",
        })),
      archivedThreads: conversations
        .filter((c) => c.status !== "active")
        .map((c) => ({
          id: c.id,
          status: "archived" as const,
          title: c.title || "Untitled",
        })),
      onSwitchToNewThread: async () => {
        await onNewConversation();
      },
      onSwitchToThread: async (threadId: string) => {
        await onSelectConversation(threadId);
      },
      onDelete: async (threadId: string) => {
        await onDeleteConversation(threadId);
      },
    }),
    [conversations, activeConversationId, onNewConversation, onSelectConversation, onDeleteConversation],
  );
}
