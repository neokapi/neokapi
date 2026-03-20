// ---------------------------------------------------------------------------
// assistant-ui powered components (new)
// ---------------------------------------------------------------------------

export { BravoSidebar } from "./bravo-sidebar";
export type { BravoSidebarProps } from "./bravo-sidebar";

export { BravoAssistantThread } from "./bravo-thread";

export { BravoToolCallRenderer, BravoFallbackToolUI } from "./bravo-tool-ui";

export { useBravoRuntime, useBravoThreadListAdapter } from "./bravo-runtime";
export type { BravoRuntimeOptions, BravoThreadListOptions } from "./bravo-runtime";

// ---------------------------------------------------------------------------
// Legacy components (kept — not replaced by assistant-ui)
// ---------------------------------------------------------------------------

/** @deprecated Use BravoSidebar instead. Kept for backwards compatibility. */
export { BravoPanel } from "./BravoPanel";
export type { BravoPanelProps } from "./BravoPanel";

export { BravoPanelTrigger } from "./BravoPanelTrigger";
export type { BravoPanelTriggerProps } from "./BravoPanelTrigger";

/** @deprecated Replaced by assistant-ui Thread. */
export { BravoThread } from "./BravoThread";
export type { BravoThreadProps } from "./BravoThread";

/** @deprecated Replaced by assistant-ui Composer. */
export { BravoComposer } from "./BravoComposer";
export type { BravoComposerProps } from "./BravoComposer";

export { BravoToolCall } from "./BravoToolCall";
export type { BravoToolCallProps } from "./BravoToolCall";

/** @deprecated Replaced by assistant-ui built-in markdown rendering. */
export { BravoCodeBlock } from "./BravoCodeBlock";
export type { BravoCodeBlockProps } from "./BravoCodeBlock";

export { BravoApprovalCard } from "./BravoApprovalCard";
export type { BravoApprovalCardProps } from "./BravoApprovalCard";

export { BravoConversationList } from "./BravoConversationList";
export type { BravoConversationListProps } from "./BravoConversationList";

export { BravoConfigPanel } from "./BravoConfigPanel";
export type { BravoConfigPanelProps } from "./BravoConfigPanel";

export { BravoUsageDashboard } from "./BravoUsageDashboard";
export type { BravoUsageDashboardProps } from "./BravoUsageDashboard";

export { BravoModeSelector } from "./BravoModeSelector";
export type { BravoMode, BravoModeSelectorProps } from "./BravoModeSelector";

export { BravoColdStart } from "./BravoColdStart";
