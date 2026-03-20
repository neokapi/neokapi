/**
 * Styled assistant-ui Thread for the @bravo side panel.
 *
 * Uses assistant-ui primitives (ThreadPrimitive, MessagePrimitive, ComposerPrimitive,
 * ActionBarPrimitive) with Tailwind classes matching the existing Bowrain design
 * system (shadcn + OKLCH theme tokens).
 */

import { type FC } from "react";
import {
  ThreadPrimitive,
  MessagePrimitive,
  ComposerPrimitive,
  ActionBarPrimitive,
  BranchPickerPrimitive,
  useMessagePartText,
} from "@assistant-ui/react";
import { MarkdownTextPrimitive } from "@assistant-ui/react-markdown";
import { BravoFallbackToolUI } from "./bravo-tool-ui";

// ---------------------------------------------------------------------------
// Thread (top-level)
// ---------------------------------------------------------------------------

export const BravoAssistantThread: FC = () => {
  return (
    <ThreadPrimitive.Root className="flex flex-col h-full">
      <ThreadPrimitive.Viewport className="flex-1 overflow-y-auto">
        <ThreadPrimitive.Empty>
          <BravoThreadEmpty />
        </ThreadPrimitive.Empty>

        <ThreadPrimitive.Messages
          components={{
            UserMessage: BravoUserMessage,
            AssistantMessage: BravoAssistantMessage,
          }}
        />

        {/* Scroll-to-bottom button */}
        <ThreadPrimitive.ViewportFooter className="sticky bottom-0 flex justify-center pb-2">
          <ThreadPrimitive.ScrollToBottom className="inline-flex items-center gap-1 rounded-full border bg-background px-3 py-1.5 text-xs text-muted-foreground shadow-sm hover:bg-accent transition-colors cursor-pointer">
            <svg
              viewBox="0 0 24 24"
              className="size-3"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path d="M12 5v14M5 12l7 7 7-7" />
            </svg>
            New messages
          </ThreadPrimitive.ScrollToBottom>
        </ThreadPrimitive.ViewportFooter>
      </ThreadPrimitive.Viewport>

      <BravoComposer />

      {/* Register the catch-all tool UI renderer */}
      <BravoFallbackToolUI />
    </ThreadPrimitive.Root>
  );
};

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

function BravoThreadEmpty() {
  return (
    <div className="py-12 text-center text-sm text-muted-foreground">
      Start a conversation with @bravo
    </div>
  );
}

// ---------------------------------------------------------------------------
// User message
// ---------------------------------------------------------------------------

const BravoUserMessage: FC = () => {
  return (
    <MessagePrimitive.Root className="flex flex-col gap-1 items-end px-4 py-2">
      <div className="text-xs font-medium text-muted-foreground px-1">You</div>
      <div className="max-w-[85%] rounded-lg px-3 py-2 text-sm leading-relaxed bg-primary text-primary-foreground">
        <MessagePrimitive.Content
          components={{
            Text: UserTextPart,
          }}
        />
      </div>
    </MessagePrimitive.Root>
  );
};

function UserTextPart() {
  const { text } = useMessagePartText();
  return <span className="whitespace-pre-wrap">{text}</span>;
}

// ---------------------------------------------------------------------------
// Assistant message
// ---------------------------------------------------------------------------

const BravoAssistantMessage: FC = () => {
  return (
    <MessagePrimitive.Root className="flex flex-col gap-1 items-start px-4 py-2 group">
      <div className="text-xs font-medium text-muted-foreground px-1">@bravo</div>
      <div className="max-w-[85%] rounded-lg px-3 py-2 text-sm leading-relaxed bg-muted text-foreground">
        <MessagePrimitive.Content
          components={{
            Text: AssistantTextPart,
          }}
        />
      </div>

      {/* Action bar — copy, branch navigation */}
      <div className="opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-1 px-1">
        <ActionBarPrimitive.Root className="flex items-center gap-1">
          <ActionBarPrimitive.Copy className="text-xs text-muted-foreground hover:text-foreground transition-colors px-1 py-0.5 rounded cursor-pointer">
            Copy
          </ActionBarPrimitive.Copy>
        </ActionBarPrimitive.Root>

        <BranchPickerPrimitive.Root className="flex items-center gap-0.5 text-xs text-muted-foreground">
          <BranchPickerPrimitive.Previous className="hover:text-foreground transition-colors cursor-pointer disabled:opacity-30">
            <svg
              viewBox="0 0 24 24"
              className="size-3"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path d="M15 18l-6-6 6-6" />
            </svg>
          </BranchPickerPrimitive.Previous>
          <BranchPickerPrimitive.Count />
          <BranchPickerPrimitive.Next className="hover:text-foreground transition-colors cursor-pointer disabled:opacity-30">
            <svg
              viewBox="0 0 24 24"
              className="size-3"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path d="M9 18l6-6-6-6" />
            </svg>
          </BranchPickerPrimitive.Next>
        </BranchPickerPrimitive.Root>
      </div>

      {/* Token usage from metadata */}
      <MessageTokenUsage />
    </MessagePrimitive.Root>
  );
};

function AssistantTextPart() {
  return (
    <MarkdownTextPrimitive className="prose prose-sm dark:prose-invert max-w-none [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
  );
}

/** Display token usage from message metadata if available. */
function MessageTokenUsage() {
  // Access custom metadata through the message context.
  return (
    <MessagePrimitive.If assistant>
      <MessagePrimitive.Root className="contents">
        {/* Token usage is available via metadata.custom — rendered by the
            parent component when metadata is present. For now we skip this
            since assistant-ui doesn't expose metadata directly in primitives.
            The data is preserved in the converted message and can be accessed
            via useMessage() if needed. */}
      </MessagePrimitive.Root>
    </MessagePrimitive.If>
  );
}

// ---------------------------------------------------------------------------
// Composer
// ---------------------------------------------------------------------------

const BravoComposer: FC = () => {
  return (
    <ComposerPrimitive.Root className="shrink-0 border-t p-3">
      <div className="flex items-end gap-2">
        <ComposerPrimitive.Input
          autoFocus
          placeholder="Message @bravo..."
          className="flex-1 resize-none rounded-lg border bg-background px-3 py-2 text-sm leading-relaxed placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring min-h-[40px] max-h-[160px]"
          rows={1}
        />
        <ComposerPrimitive.Send className="shrink-0 inline-flex items-center justify-center rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:pointer-events-none cursor-pointer">
          <svg
            viewBox="0 0 24 24"
            className="size-4"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path d="M22 2 11 13M22 2l-7 20-4-9-9-4z" />
          </svg>
        </ComposerPrimitive.Send>
        <ComposerPrimitive.Cancel className="shrink-0 inline-flex items-center justify-center rounded-lg border px-3 py-2 text-sm font-medium text-muted-foreground hover:text-foreground hover:bg-accent transition-colors cursor-pointer">
          Stop
        </ComposerPrimitive.Cancel>
      </div>
    </ComposerPrimitive.Root>
  );
};
