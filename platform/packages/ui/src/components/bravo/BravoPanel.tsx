import { useState } from "react";
import { cn } from "../../lib/utils";
import type { BravoConversation, BravoMessage, BravoToolCall } from "../../types/api";
import { BravoConversationList } from "./BravoConversationList";
import { BravoThread } from "./BravoThread";
import { BravoComposer } from "./BravoComposer";
import { BravoModeSelector, type BravoMode } from "./BravoModeSelector";
import { BravoColdStart } from "./BravoColdStart";

export interface BravoPanelProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  conversations: BravoConversation[];
  activeConversation?: BravoConversation;
  messages: BravoMessage[];
  streaming?: boolean;
  streamingContent?: string;
  streamingToolCalls?: BravoToolCall[];
  onNewConversation: () => void;
  onSelectConversation: (conv: BravoConversation) => void;
  onDeleteConversation: (conv: BravoConversation) => void;
  onSendMessage: (content: string) => void;
  onApproveToolCall: (toolCallId: string) => void;
  onDenyToolCall: (toolCallId: string) => void;
  onCancelStreaming?: () => void;
  onShowConfig?: () => void;
  loading?: boolean;
  sendDisabled?: boolean;
  /** True while the agent container is provisioning (cold start). */
  coldStarting?: boolean;
  /** Current mode for the agent interaction. */
  mode?: BravoMode;
  onModeChange?: (mode: BravoMode) => void;
}

type View = "list" | "chat";

export function BravoPanel({
  open,
  onOpenChange,
  conversations,
  activeConversation,
  messages,
  streaming,
  streamingContent,
  streamingToolCalls,
  onNewConversation,
  onSelectConversation,
  onDeleteConversation,
  onSendMessage,
  onApproveToolCall,
  onDenyToolCall,
  onCancelStreaming,
  onShowConfig,
  loading,
  sendDisabled,
  coldStarting,
  mode = "ask",
  onModeChange,
}: BravoPanelProps) {
  const [view, setView] = useState<View>(activeConversation ? "chat" : "list");

  const handleSelect = (conv: BravoConversation) => {
    onSelectConversation(conv);
    setView("chat");
  };

  const handleNew = () => {
    onNewConversation();
    setView("chat");
  };

  const handleBack = () => {
    setView("list");
  };

  const placeholders: Record<BravoMode, string> = {
    ask: "Ask about projects, formats, TM...",
    coworker: "Tell @bravo what to do...",
    bravo: "Paste content for voice review...",
  };

  return (
    <>
      {/* Backdrop for mobile — tap to close */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/20 md:hidden"
          onClick={() => onOpenChange(false)}
        />
      )}

      {/* Panel */}
      <aside
        className={cn(
          // Base: right-side panel with border
          "z-50 flex flex-col border-l bg-background",
          // Transition
          "transition-[width,opacity] duration-300 ease-in-out",
          // Desktop: inline in the layout flow
          "hidden md:flex",
          open ? "w-[400px] opacity-100" : "w-0 opacity-0 overflow-hidden border-l-0",
          // We'll handle mobile separately below
        )}
      >
        <PanelInner
          view={view}
          onBack={handleBack}
          onClose={() => onOpenChange(false)}
          activeConversation={activeConversation}
          onShowConfig={onShowConfig}
          conversations={conversations}
          messages={messages}
          streaming={streaming}
          streamingContent={streamingContent}
          streamingToolCalls={streamingToolCalls}
          onNewConversation={handleNew}
          onSelectConversation={handleSelect}
          onDeleteConversation={onDeleteConversation}
          onSendMessage={onSendMessage}
          onApproveToolCall={onApproveToolCall}
          onDenyToolCall={onDenyToolCall}
          onCancelStreaming={onCancelStreaming}
          loading={loading}
          sendDisabled={sendDisabled}
          coldStarting={coldStarting}
          mode={mode}
          onModeChange={onModeChange}
          placeholder={placeholders[mode]}
        />
      </aside>

      {/* Mobile: full-screen overlay panel */}
      <aside
        className={cn(
          "fixed inset-y-0 right-0 z-50 flex flex-col bg-background",
          "transition-transform duration-300 ease-in-out",
          "w-full max-w-[420px] md:hidden",
          open ? "translate-x-0" : "translate-x-full",
        )}
      >
        <PanelInner
          view={view}
          onBack={handleBack}
          onClose={() => onOpenChange(false)}
          activeConversation={activeConversation}
          onShowConfig={onShowConfig}
          conversations={conversations}
          messages={messages}
          streaming={streaming}
          streamingContent={streamingContent}
          streamingToolCalls={streamingToolCalls}
          onNewConversation={handleNew}
          onSelectConversation={handleSelect}
          onDeleteConversation={onDeleteConversation}
          onSendMessage={onSendMessage}
          onApproveToolCall={onApproveToolCall}
          onDenyToolCall={onDenyToolCall}
          onCancelStreaming={onCancelStreaming}
          loading={loading}
          sendDisabled={sendDisabled}
          coldStarting={coldStarting}
          mode={mode}
          onModeChange={onModeChange}
          placeholder={placeholders[mode]}
        />
      </aside>
    </>
  );
}

/** Inner content shared between desktop and mobile layouts. */
function PanelInner({
  view,
  onBack,
  onClose,
  activeConversation,
  onShowConfig,
  conversations,
  messages,
  streaming,
  streamingContent,
  streamingToolCalls,
  onNewConversation,
  onSelectConversation,
  onDeleteConversation,
  onSendMessage,
  onApproveToolCall,
  onDenyToolCall,
  onCancelStreaming,
  loading,
  sendDisabled,
  coldStarting,
  mode,
  onModeChange,
  placeholder,
}: {
  view: View;
  onBack: () => void;
  onClose: () => void;
  activeConversation?: BravoConversation;
  onShowConfig?: () => void;
  conversations: BravoConversation[];
  messages: BravoMessage[];
  streaming?: boolean;
  streamingContent?: string;
  streamingToolCalls?: BravoToolCall[];
  onNewConversation: () => void;
  onSelectConversation: (conv: BravoConversation) => void;
  onDeleteConversation: (conv: BravoConversation) => void;
  onSendMessage: (content: string) => void;
  onApproveToolCall: (toolCallId: string) => void;
  onDenyToolCall: (toolCallId: string) => void;
  onCancelStreaming?: () => void;
  loading?: boolean;
  sendDisabled?: boolean;
  coldStarting?: boolean;
  mode?: BravoMode;
  onModeChange?: (mode: BravoMode) => void;
  placeholder?: string;
}) {
  return (
    <>
      {/* Header */}
      <div className="shrink-0 border-b px-4 py-3">
        <div className="flex items-center gap-2">
          {view === "chat" && (
            <button
              onClick={onBack}
              className="text-muted-foreground hover:text-foreground transition-colors"
              aria-label="Back to conversations"
            >
              <svg
                viewBox="0 0 24 24"
                className="size-4"
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path d="M15 18l-6-6 6-6" />
              </svg>
            </button>
          )}
          <span className="flex-1 text-sm font-semibold truncate">
            {view === "chat" && activeConversation
              ? activeConversation.title || "Conversation"
              : "@bravo"}
          </span>
          {onShowConfig && (
            <button
              onClick={onShowConfig}
              className="text-muted-foreground hover:text-foreground transition-colors"
              aria-label="Settings"
            >
              <svg
                viewBox="0 0 24 24"
                className="size-4"
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                <circle cx="12" cy="12" r="3" />
              </svg>
            </button>
          )}
          <button
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground transition-colors"
            aria-label="Close panel"
          >
            <svg
              viewBox="0 0 24 24"
              className="size-4"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path d="M18 6 6 18M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Mode selector */}
        {onModeChange && view === "chat" && (
          <div className="mt-2">
            <BravoModeSelector mode={mode ?? "ask"} onChange={onModeChange} />
          </div>
        )}
      </div>

      {/* Content */}
      {view === "list" ? (
        <div className="flex-1 overflow-y-auto p-4">
          <BravoConversationList
            conversations={conversations}
            activeId={activeConversation?.id}
            onSelect={onSelectConversation}
            onDelete={onDeleteConversation}
            onNew={onNewConversation}
            loading={loading}
          />
        </div>
      ) : (
        <>
          <div className="flex-1 overflow-y-auto">
            {coldStarting ? (
              <BravoColdStart />
            ) : (
              <BravoThread
                messages={messages}
                streaming={streaming}
                streamingContent={streamingContent}
                streamingToolCalls={streamingToolCalls}
                onApprove={onApproveToolCall}
                onDeny={onDenyToolCall}
                onCancel={onCancelStreaming}
              />
            )}
          </div>
          <div className="shrink-0">
            <BravoComposer
              onSend={onSendMessage}
              disabled={sendDisabled || streaming || coldStarting}
              placeholder={placeholder}
            />
          </div>
        </>
      )}
    </>
  );
}
