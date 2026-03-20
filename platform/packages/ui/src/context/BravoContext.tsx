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
import type { BravoConversation, BravoMessage, BravoToolCall } from "../types/api";
import type { BravoMode } from "../components/bravo/BravoModeSelector";
import { useApi } from "./ApiContext";
import { useWorkspace } from "./WorkspaceContext";
import { useBravoRuntime, useBravoThreadListAdapter } from "../components/bravo/bravo-runtime";

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
  /** Tool calls being streamed for the current assistant message. */
  streamingToolCalls: BravoToolCall[];
  /** Whether conversations are being loaded. */
  loading: boolean;
  /** Current interaction mode. */
  mode: BravoMode;
  /** True while waiting for the agent container to provision (cold start). */
  coldStarting: boolean;
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
  /** Send a message in the active conversation (uses SSE streaming). */
  sendMessage: (
    content: string,
    context?: { projectId?: string; stream?: string; itemId?: string },
  ) => Promise<void>;
  /** Cancel an ongoing streaming response. */
  cancelStreaming: () => void;
  /** Approve a tool call that requires human approval. */
  approveToolCall: (toolCallId: string) => Promise<void>;
  /** Deny a tool call that requires human approval. */
  denyToolCall: (toolCallId: string) => Promise<void>;
  /** Refresh the conversation list. */
  refreshConversations: () => Promise<void>;
  /** Change the interaction mode. */
  setMode: (mode: BravoMode) => void;
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

  const [panelOpen, setPanelOpen] = useState(() => {
    try {
      return localStorage.getItem("neokapi-bravo-panel") === "open";
    } catch {
      return false;
    }
  });
  const [conversations, setConversations] = useState<BravoConversation[]>([]);
  const [activeConversation, setActiveConversation] = useState<BravoConversation | undefined>();
  const [messages, setMessages] = useState<BravoMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [streamingContent, setStreamingContent] = useState("");
  const [streamingToolCalls, setStreamingToolCalls] = useState<BravoToolCall[]>([]);
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<BravoMode>("ask");
  const [coldStarting, setColdStarting] = useState(false);

  // AbortController for the current SSE stream.
  const abortRef = useRef<AbortController | null>(null);
  // Track if we've done the initial fetch for this workspace.
  const fetchedRef = useRef<string | null>(null);
  // Track if we've auto-launched for this workspace/panel-open cycle.
  const autoLaunchedRef = useRef<string | null>(null);
  // Mutable ref for accumulated streaming content (avoids stale closures in SSE callbacks).
  const streamContentRef = useRef("");
  // Mutable ref for current streaming message ID.
  const streamMsgIdRef = useRef("");

  // Persist panel open/close state.
  useEffect(() => {
    try {
      localStorage.setItem("neokapi-bravo-panel", panelOpen ? "open" : "closed");
    } catch {
      // Ignore storage errors.
    }
  }, [panelOpen]);

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

  // Auto-launch a new conversation when the panel opens with no active conversation.
  useEffect(() => {
    if (panelOpen && !loading && !activeConversation && ws && autoLaunchedRef.current !== ws) {
      autoLaunchedRef.current = ws;
      const create = async () => {
        const conv = await api.bravoCreateConversation(ws);
        setConversations((prev) => [conv, ...prev]);
        setActiveConversation(conv);
        setMessages([]);
      };
      void create();
    }
  }, [panelOpen, loading, activeConversation, ws, api]);

  // Reset state when workspace changes.
  useEffect(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setActiveConversation(undefined);
    setMessages([]);
    setConversations([]);
    setStreaming(false);
    setStreamingContent("");
    setStreamingToolCalls([]);
    fetchedRef.current = null;
    autoLaunchedRef.current = null;
  }, [ws]);

  // Cleanup on unmount.
  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

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
      // Cancel any ongoing stream.
      abortRef.current?.abort();
      abortRef.current = null;
      setStreaming(false);
      setStreamingContent("");
      setStreamingToolCalls([]);

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
        abortRef.current?.abort();
        abortRef.current = null;
        setActiveConversation(undefined);
        setMessages([]);
        setStreaming(false);
        setStreamingContent("");
        setStreamingToolCalls([]);
      }
    },
    [api, ws, activeConversation],
  );

  const sendMessage = useCallback(
    async (content: string, context?: { projectId?: string; stream?: string; itemId?: string }) => {
      if (!ws || !activeConversation) return;

      // Add user message optimistically.
      const userMsg: BravoMessage = {
        id: `tmp-${Date.now()}`,
        conversation_id: activeConversation.id,
        role: "user",
        content,
        created_at: new Date().toISOString(),
      };
      setMessages((prev) => [...prev, userMsg]);
      setStreaming(true);
      setStreamingContent("");
      setStreamingToolCalls([]);
      setColdStarting(false);
      streamContentRef.current = "";
      streamMsgIdRef.current = "";

      // Show cold start UI if no content arrives within 3 seconds.
      const coldStartTimer = setTimeout(() => {
        if (!streamContentRef.current && !streamMsgIdRef.current) {
          setColdStarting(true);
        }
      }, 3000);

      const controller = api.bravoSendMessageSSE(
        ws,
        activeConversation.id,
        content,
        {
          onMessageStart: (data) => {
            streamMsgIdRef.current = data.id;
            setColdStarting(false);
            clearTimeout(coldStartTimer);
          },

          onContentDelta: (data) => {
            streamContentRef.current += data.delta;
            setStreamingContent(streamContentRef.current);
            setColdStarting(false);
          },

          onToolCallStart: (data) => {
            const tc: BravoToolCall = {
              id: data.id,
              message_id: streamMsgIdRef.current,
              tool_name: data.tool,
              input: data.input,
              status: "running",
              duration: 0,
            };
            setStreamingToolCalls((prev) => [...prev, tc]);
          },

          onToolCallEnd: (data) => {
            setStreamingToolCalls((prev) =>
              prev.map((tc) =>
                tc.id === data.id
                  ? {
                      ...tc,
                      status: data.status as BravoToolCall["status"],
                      output: data.output,
                      duration: data.duration_ms * 1e6, // ms → ns for display consistency
                    }
                  : tc,
              ),
            );
          },

          onNeedsApproval: (data) => {
            setStreamingToolCalls((prev) =>
              prev.map((tc) => (tc.id === data.id ? { ...tc, status: "needs_approval" } : tc)),
            );
          },

          onMessageEnd: (data) => {
            // Assemble the final assistant message and add to the message list.
            const finalMsg: BravoMessage = {
              id: data.id,
              conversation_id: activeConversation.id,
              role: "assistant",
              content: streamContentRef.current,
              input_tokens: data.usage?.input_tokens,
              output_tokens: data.usage?.output_tokens,
              created_at: new Date().toISOString(),
            };

            // Attach collected tool calls.
            setStreamingToolCalls((prevToolCalls) => {
              if (prevToolCalls.length > 0) {
                finalMsg.tool_calls = prevToolCalls;
              }

              setMessages((prev) => [...prev, finalMsg]);
              setStreaming(false);
              setStreamingContent("");
              return [];
            });

            abortRef.current = null;
            streamContentRef.current = "";
            streamMsgIdRef.current = "";
          },

          onError: () => {
            setStreaming(false);
            setStreamingContent("");
            setStreamingToolCalls([]);
            setColdStarting(false);
            abortRef.current = null;
            streamContentRef.current = "";
            streamMsgIdRef.current = "";
          },
        },
        mode,
        context,
      );

      abortRef.current = controller;
    },
    [api, ws, activeConversation],
  );

  const cancelStreaming = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setStreaming(false);
    setStreamingContent("");
    setStreamingToolCalls([]);
    streamContentRef.current = "";
    streamMsgIdRef.current = "";
  }, []);

  const approveToolCall = useCallback(
    async (toolCallId: string) => {
      if (!ws || !activeConversation) return;
      await api.bravoApproveToolCall(ws, activeConversation.id, toolCallId);
      // Update the tool call status locally.
      setStreamingToolCalls((prev) =>
        prev.map((tc) => (tc.id === toolCallId ? { ...tc, status: "running" } : tc)),
      );
      setMessages((prev) =>
        prev.map((msg) => ({
          ...msg,
          tool_calls: msg.tool_calls?.map((tc) =>
            tc.id === toolCallId ? { ...tc, status: "running" as const } : tc,
          ),
        })),
      );
    },
    [api, ws, activeConversation],
  );

  const denyToolCall = useCallback(
    async (toolCallId: string) => {
      if (!ws || !activeConversation) return;
      await api.bravoDenyToolCall(ws, activeConversation.id, toolCallId);
      // Update the tool call status locally.
      setStreamingToolCalls((prev) =>
        prev.map((tc) => (tc.id === toolCallId ? { ...tc, status: "denied" } : tc)),
      );
      setMessages((prev) =>
        prev.map((msg) => ({
          ...msg,
          tool_calls: msg.tool_calls?.map((tc) =>
            tc.id === toolCallId ? { ...tc, status: "denied" as const } : tc,
          ),
        })),
      );
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
      streamingToolCalls,
      loading,
      mode,
      coldStarting,
    }),
    [
      panelOpen,
      conversations,
      activeConversation,
      messages,
      streaming,
      streamingContent,
      streamingToolCalls,
      loading,
      mode,
      coldStarting,
    ],
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
      cancelStreaming,
      approveToolCall,
      denyToolCall,
      refreshConversations,
      setMode,
    }),
    [
      openPanel,
      closePanel,
      togglePanel,
      newConversation,
      selectConversation,
      deleteConversation,
      sendMessage,
      cancelStreaming,
      approveToolCall,
      denyToolCall,
      refreshConversations,
      setMode,
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

/**
 * Hook that creates an assistant-ui runtime backed by the BravoContext state.
 * Use this in components that render the assistant-ui Thread.
 */
export function useBravoAssistantRuntime() {
  const { state, actions } = useBravo();

  return useBravoRuntime({
    messages: state.messages,
    streaming: state.streaming,
    streamingContent: state.streamingContent,
    streamingToolCalls: state.streamingToolCalls,
    onSendMessage: (content: string) => actions.sendMessage(content),
    onCancel: actions.cancelStreaming,
    onApproveToolCall: actions.approveToolCall,
    onDenyToolCall: actions.denyToolCall,
    mode: state.mode,
  });
}

/**
 * Hook that creates an assistant-ui thread list adapter from BravoContext.
 */
export function useBravoAssistantThreadList() {
  const { state, actions } = useBravo();

  return useBravoThreadListAdapter({
    conversations: state.conversations,
    activeConversationId: state.activeConversation?.id,
    onNewConversation: async () => {
      await actions.newConversation();
    },
    onSelectConversation: async (id: string) => {
      const conv = state.conversations.find((c) => c.id === id);
      if (conv) await actions.selectConversation(conv);
    },
    onDeleteConversation: async (id: string) => {
      const conv = state.conversations.find((c) => c.id === id);
      if (conv) await actions.deleteConversation(conv);
    },
  });
}
