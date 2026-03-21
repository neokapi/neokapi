/**
 * BravoSidebar — the main @bravo side panel built on assistant-ui.
 *
 * Replaces the old BravoPanel. Wraps the assistant-ui Thread in the same
 * responsive layout (inline 400px on desktop, full overlay on mobile) with
 * the familiar header (mode selector, settings, close button).
 */

import { type FC } from "react";
import { AssistantRuntimeProvider, type AssistantRuntime } from "@assistant-ui/react";
import { cn } from "../../lib/utils";
import { BravoAssistantThread } from "./bravo-thread";
import { BravoConversationList, type BravoConversationListProps } from "./BravoConversationList";
import { BravoModeSelector, type BravoMode } from "./BravoModeSelector";
import { BravoStepUpCard } from "./BravoStepUpCard";
import type { BravoSSEStepUp } from "../../types/api";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface BravoSidebarProps {
  /** Whether the sidebar is open. */
  open: boolean;
  /** Toggle open/close. */
  onOpenChange: (open: boolean) => void;
  /** The assistant-ui runtime instance (from useBravoRuntime). */
  runtime: AssistantRuntime;
  /** Which view to show: the conversation list or the active chat thread. */
  view: "list" | "chat";
  /** Switch back to conversation list. */
  onBack: () => void;
  /** Title of the active conversation (shown in header). */
  activeTitle?: string;
  /** Props for the conversation list view. */
  conversationListProps?: BravoConversationListProps;
  /** Open the @bravo settings panel. */
  onShowConfig?: () => void;
  /** Whether the agent is cold-starting. */
  coldStarting?: boolean;
  /** Current interaction mode. */
  mode?: BravoMode;
  /** Mode change handler. */
  onModeChange?: (mode: BravoMode) => void;
  /** Composer placeholder override based on mode. */
  placeholder?: string;
  /** Active step-up prompt (null = no prompt). */
  stepUp?: BravoSSEStepUp | null;
  /** Handle mode switch from step-up card. */
  onStepUpSwitch?: (mode: string) => void;
  /** Dismiss step-up card. */
  onStepUpDismiss?: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export const BravoSidebar: FC<BravoSidebarProps> = ({
  open,
  onOpenChange,
  runtime,
  view,
  onBack,
  activeTitle,
  conversationListProps,
  onShowConfig,
  coldStarting,
  mode = "ask",
  onModeChange,
  stepUp,
  onStepUpSwitch,
  onStepUpDismiss,
}) => {
  return (
    <>
      {/* Backdrop for mobile — tap to close */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/20 md:hidden"
          onClick={() => onOpenChange(false)}
        />
      )}

      {/* Panel — responsive: inline on desktop, fixed overlay on mobile */}
      <aside
        role="complementary"
        aria-label="@bravo assistant"
        className={cn(
          "shrink-0 flex flex-col bg-background overflow-hidden",
          // Desktop
          "md:relative md:z-auto md:border-l",
          "md:transition-[width,opacity] md:duration-300 md:ease-in-out",
          open
            ? "md:w-[400px] md:opacity-100"
            : "md:w-0 md:opacity-0 md:overflow-hidden md:border-l-0",
          // Mobile — fixed overlay with safe area support
          "fixed inset-0 z-50 max-md:transition-transform max-md:duration-300 max-md:ease-in-out",
          "pb-[env(safe-area-inset-bottom)] pt-[env(safe-area-inset-top)]",
          "pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)]",
          open ? "max-md:translate-x-0" : "max-md:translate-x-full",
        )}
      >
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
              {view === "chat" && activeTitle ? activeTitle : "@bravo"}
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
              onClick={() => onOpenChange(false)}
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
              <BravoModeSelector mode={mode} onChange={onModeChange} />
            </div>
          )}
        </div>

        {/* Content */}
        {view === "list" ? (
          <div className="flex-1 overflow-y-auto p-4">
            {conversationListProps && <BravoConversationList {...conversationListProps} />}
          </div>
        ) : (
          <AssistantRuntimeProvider runtime={runtime}>
            <div className="flex-1 flex flex-col min-h-0">
              <BravoAssistantThread coldStart={coldStarting} />
            </div>
            {stepUp && onStepUpSwitch && onStepUpDismiss && (
              <div className="px-4 pb-2">
                <BravoStepUpCard
                  currentMode={stepUp.current_mode}
                  requiredMode={stepUp.required_mode}
                  action={stepUp.action}
                  onSwitchMode={onStepUpSwitch}
                  onDismiss={onStepUpDismiss}
                />
              </div>
            )}
          </AssistantRuntimeProvider>
        )}
      </aside>
    </>
  );
};
