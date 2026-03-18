import { useState } from "react";
import type { BravoConversation, BravoMessage, BravoToolCall } from "../../types/api";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "../ui/sheet";
import { BravoConversationList } from "./BravoConversationList";
import { BravoThread } from "./BravoThread";
import { BravoComposer } from "./BravoComposer";

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

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-[480px] max-w-full p-0 flex flex-col">
        <SheetHeader className="shrink-0 border-b px-4 py-3">
          <div className="flex items-center gap-2">
            {view === "chat" && (
              <button
                onClick={handleBack}
                className="text-muted-foreground hover:text-foreground text-sm"
              >
                &larr;
              </button>
            )}
            <SheetTitle className="flex-1 text-base">
              {view === "chat" && activeConversation
                ? activeConversation.title || "Conversation"
                : "@bravo"}
            </SheetTitle>
            {onShowConfig && (
              <button
                onClick={onShowConfig}
                className="text-muted-foreground hover:text-foreground text-sm"
                aria-label="Settings"
              >
                &#9881;
              </button>
            )}
          </div>
        </SheetHeader>

        {view === "list" ? (
          <div className="flex-1 overflow-y-auto p-4">
            <BravoConversationList
              conversations={conversations}
              activeId={activeConversation?.id}
              onSelect={handleSelect}
              onDelete={onDeleteConversation}
              onNew={handleNew}
              loading={loading}
            />
          </div>
        ) : (
          <>
            <div className="flex-1 overflow-y-auto">
              <BravoThread
                messages={messages}
                streaming={streaming}
                streamingContent={streamingContent}
                streamingToolCalls={streamingToolCalls}
                onApprove={onApproveToolCall}
                onDeny={onDenyToolCall}
                onCancel={onCancelStreaming}
              />
            </div>
            <div className="shrink-0">
              <BravoComposer onSend={onSendMessage} disabled={sendDisabled || streaming} />
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
