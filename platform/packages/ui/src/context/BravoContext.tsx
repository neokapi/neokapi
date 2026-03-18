import {
  createContext,
  useContext,
  useState,
  useCallback,
  useMemo,
  useRef,
  useEffect,
  type ReactNode,
} from "react";
import type {
  BravoConversation,
  BravoMessage,
  BravoSSEEventType,
} from "../types/api";
import { useApi } from "./ApiContext";
import { useWorkspace } from "./WorkspaceContext";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BravoState {
  /** Whether the side panel is open. */
  panelOpen: boolean;
  /** All conversations for the current user/workspace. */
  conversations: BravoConversation[];
  /** The conversation currently being viewed. */
  activeConversation: BravoConversation | undefined;
  /** Messages for the active conversation. */
  messages: BravoMessage[];
  /** Whether an assistant response is currently streaming. */
  streaming: boolean;
  /** Partial content being streamed from the assistant. */
  streamingContent: string;
  /** Whether conversations are being loaded. */
  loading: boolean;
}

interface BravoActions {
  openPanel: () => void;
  closePanel: () => void;
  togglePanel: () => void;
  /** Start a new conversation. */
  newConversation: (projectId?: string) => Promise<void>;
  /** Switch to an existing conversation. */
  selectConversation: (conv: BravoConversation) => Promise<void>;
  /** Delete a conversation. */
  deleteConversation: (conv: BravoConversation) => Promise<void>;
  /** Send a message in the active conversation. */
  sendMessage: (content: string) => Promise<void>;
  /** Approve a tool call that requires human approval. */
  approveToolCall: (toolCallId: string) => Promise<void>;
  /** Deny a tool call that requires human approval. */
  denyToolCall: (toolCallId: string) => Promise<void>;
  /** Refresh the conversation list. */
  refreshConversations: () => Promise<void>;
}

interface BravoContextValue {
  state: BravoState;
  actions: BravoActions;
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const BravoContext = createContext<BravoContextValue | null>(null);

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function BravoProvider({ children }: { children: ReactNode }) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [panelOpen, setPanelOpen] = useState(false);
  const [conversations, setConversations] = useState<BravoConversation[]>([]);
  const [activeConversation, setActiveConversation] = useState<BravoConversation | undefined>();
  const [messages, setMessages] = useState<BravoMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [streamingContent, setStreamingContent] = useState("");
  const [loading, setLoading] = useState(false);

  // Track if we've done the initial fetch for this workspace.
  const fetchedRef = useRef<string | null>(null);

  // Fetch conversations when panel opens for the first time.
  const fetchConversations = useCallback(async () => {
    if (!ws) return;
    setLoading(true);
    try {
      const resp = await api.bravoListConversations(ws, 50, 0);
      setConversations(resp.conversations ?? []);
    } catch {
      // Silently fail — the panel will show empty state.
    } finally {
      setLoading(false);
    }
  }, [api, ws]);

  useEffect(() => {
    if (panelOpen && fetchedRef.current !== ws) {
      fetchedRef.current = ws;
      void fetchConversations();
    }
  }, [panelOpen, ws, fetchConversations]);

  // Reset state when workspace changes.
  useEffect(() => {
    setActiveConversation(undefined);
    setMessages([]);
    setConversations([]);
    setStreaming(false);
    setStreamingContent("");
    fetchedRef.current = null;
  }, [ws]);

  // -----------------------------------------------------------------------
  // Actions
  // -----------------------------------------------------------------------

  const openPanel = useCallback(() => setPanelOpen(true), []);
  const closePanel = useCallback(() => setPanelOpen(false), []);
  const togglePanel = useCallback(() => setPanelOpen((o) => !o), []);

  const newConversation = useCallback(
    async (projectId?: string) => {
      if (!ws) return;
      const conv = await api.bravoCreateConversation(ws, projectId);
      setConversations((prev) => [conv, ...prev]);
      setActiveConversation(conv);
      setMessages([]);
    },
    [api, ws],
  );

  const selectConversation = useCallback(
    async (conv: BravoConversation) => {
      if (!ws) return;
      setActiveConversation(conv);
      try {
        const resp = await api.bravoGetConversation(ws, conv.id);
        setMessages(resp.messages ?? []);
      } catch {
        setMessages([]);
      }
    },
    [api, ws],
  );

  const deleteConversation = useCallback(
    async (conv: BravoConversation) => {
      if (!ws) return;
      await api.bravoDeleteConversation(ws, conv.id);
      setConversations((prev) => prev.filter((c) => c.id !== conv.id));
      if (activeConversation?.id === conv.id) {
        setActiveConversation(undefined);
        setMessages([]);
      }
    },
    [api, ws, activeConversation],
  );

  const sendMessage = useCallback(
    async (content: string) => {
      if (!ws || !activeConversation) return;
      setStreaming(true);
      setStreamingContent("");
      try {
        const resp = await api.bravoSendMessage(ws, activeConversation.id, content);
        setMessages((prev) => [...prev, resp.user_message, resp.assistant_message]);
      } finally {
        setStreaming(false);
        setStreamingContent("");
      }
    },
    [api, ws, activeConversation],
  );

  const approveToolCall = useCallback(
    async (toolCallId: string) => {
      if (!ws || !activeConversation) return;
      await api.bravoApproveToolCall(ws, activeConversation.id, toolCallId);
    },
    [api, ws, activeConversation],
  );

  const denyToolCall = useCallback(
    async (toolCallId: string) => {
      if (!ws || !activeConversation) return;
      await api.bravoDenyToolCall(ws, activeConversation.id, toolCallId);
    },
    [api, ws, activeConversation],
  );

  const refreshConversations = useCallback(async () => {
    fetchedRef.current = null;
    await fetchConversations();
  }, [fetchConversations]);

  // -----------------------------------------------------------------------
  // Memoized value
  // -----------------------------------------------------------------------

  const state: BravoState = useMemo(
    () => ({
      panelOpen,
      conversations,
      activeConversation,
      messages,
      streaming,
      streamingContent,
      loading,
    }),
    [panelOpen, conversations, activeConversation, messages, streaming, streamingContent, loading],
  );

  const actions: BravoActions = useMemo(
    () => ({
      openPanel,
      closePanel,
      togglePanel,
      newConversation,
      selectConversation,
      deleteConversation,
      sendMessage,
      approveToolCall,
      denyToolCall,
      refreshConversations,
    }),
    [
      openPanel,
      closePanel,
      togglePanel,
      newConversation,
      selectConversation,
      deleteConversation,
      sendMessage,
      approveToolCall,
      denyToolCall,
      refreshConversations,
    ],
  );

  const value = useMemo(() => ({ state, actions }), [state, actions]);

  return <BravoContext.Provider value={value}>{children}</BravoContext.Provider>;
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useBravo(): BravoContextValue {
  const ctx = useContext(BravoContext);
  if (!ctx) {
    throw new Error("useBravo must be used within a BravoProvider");
  }
  return ctx;
}
