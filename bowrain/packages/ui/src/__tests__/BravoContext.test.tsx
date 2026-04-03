import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, act, waitFor } from "@testing-library/react";
import { BravoProvider, useBravo } from "../context/BravoContext";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace, BravoConversation, BravoMessage, BravoSSEHandler } from "../types/api";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const ws: Workspace = {
  id: "1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

const conv1: BravoConversation = {
  id: "c1",
  workspace_id: "1",
  user_id: "u1",
  project_id: "",
  title: "First chat",
  status: "active",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const conv2: BravoConversation = {
  id: "c2",
  workspace_id: "1",
  user_id: "u1",
  project_id: "",
  title: "Second chat",
  status: "active",
  created_at: "2026-01-02T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
};

const userMsg: BravoMessage = {
  id: "m1",
  conversation_id: "c1",
  role: "user",
  content: "Hello",
  created_at: "2026-01-01T00:00:00Z",
};

const assistantMsg: BravoMessage = {
  id: "m2",
  conversation_id: "c1",
  role: "assistant",
  content: "Hi there!",
  created_at: "2026-01-01T00:00:01Z",
};

// ---------------------------------------------------------------------------
// Mock adapter
// ---------------------------------------------------------------------------

function createMockAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    getConfig: vi.fn(),
    getCurrentUser: vi.fn(),
    listWorkspaces: vi.fn(),
    createWorkspace: vi.fn(),
    getWorkspace: vi.fn(),
    updateWorkspace: vi.fn(),
    deleteWorkspace: vi.fn(),
    listMembers: vi.fn(),
    addMember: vi.fn(),
    updateMemberRole: vi.fn(),
    removeMember: vi.fn(),
    listProjects: vi.fn(),
    createProject: vi.fn(),
    getProject: vi.fn(),
    deleteProject: vi.fn(),
    uploadFiles: vi.fn(),
    removeFile: vi.fn(),
    getFileBlocks: vi.fn(),
    updateBlockTarget: vi.fn(),
    updateBlockTargetCoded: vi.fn(),
    pseudoTranslateFile: vi.fn(),
    aiTranslateFile: vi.fn(),
    tmTranslateFile: vi.fn(),
    getWordCount: vi.fn(),
    exportTranslatedFile: vi.fn(),
    lookupTMForBlock: vi.fn(),
    lookupTermsForBlock: vi.fn(),
    runQACheck: vi.fn(),
    runFileQACheck: vi.fn(),
    renderDocumentPreview: vi.fn(),
    renderBlockHTML: vi.fn(),
    getTMEntries: vi.fn(),
    getTMCount: vi.fn(),
    addTMEntry: vi.fn(),
    updateTMEntry: vi.fn(),
    deleteTMEntry: vi.fn(),
    getTerms: vi.fn(),
    getTermCount: vi.fn(),
    addConcept: vi.fn(),
    updateConcept: vi.fn(),
    deleteConcept: vi.fn(),
    importTermsCSV: vi.fn(),
    importTermsJSON: vi.fn(),
    exportTermsJSON: vi.fn(),
    listProviderConfigs: vi.fn(),
    saveProviderConfig: vi.fn(),
    deleteProviderConfig: vi.fn(),
    testProviderConfig: vi.fn(),
    getKnownLocales: vi.fn(),
    listFormats: vi.fn(),
    listTools: vi.fn(),
    listInvites: vi.fn(),
    createInvite: vi.fn(),
    deleteInvite: vi.fn(),
    acceptInvite: vi.fn(),
    claimProject: vi.fn(),
    getBlockHistory: vi.fn(),
    addBlockNote: vi.fn(),
    listBlockNotes: vi.fn(),
    deleteBlockNote: vi.fn(),
    listApiTokens: vi.fn(),
    createApiToken: vi.fn(),
    deleteApiToken: vi.fn(),
    listAutomationRules: vi.fn(),
    createAutomationRule: vi.fn(),
    updateAutomationRule: vi.fn(),
    deleteAutomationRule: vi.fn(),
    toggleAutomationRule: vi.fn(),
    listAutomationEvents: vi.fn(),
    listAutomationHistory: vi.fn(),
    listNotifications: vi.fn(),
    markNotificationRead: vi.fn(),
    markAllNotificationsRead: vi.fn(),
    deleteNotification: vi.fn(),
    createEntity: vi.fn(),
    updateEntity: vi.fn(),
    deleteEntity: vi.fn(),
    promoteEntity: vi.fn(),
    listStreams: vi.fn(),
    createStream: vi.fn(),
    getStream: vi.fn(),
    deleteStream: vi.fn(),
    diffStream: vi.fn(),
    mergeStream: vi.fn(),
    updateProject: vi.fn(),
    restoreProject: vi.fn(),
    permanentlyDeleteProject: vi.fn(),
    listArchivedProjects: vi.fn(),
    restoreStream: vi.fn(),
    updateStream: vi.fn(),
    listCollections: vi.fn(),
    createCollection: vi.fn(),
    getCollection: vi.fn(),
    updateCollection: vi.fn(),
    deleteCollection: vi.fn(),
    uploadToCollection: vi.fn(),
    listWorkspaceAuditLog: vi.fn(),
    listBrandProfiles: vi.fn(),
    getBrandProfile: vi.fn(),
    createBrandProfile: vi.fn(),
    updateBrandProfile: vi.fn(),
    deleteBrandProfile: vi.fn(),
    getBrandScores: vi.fn(),
    getBrandTrends: vi.fn(),
    listActivities: vi.fn(),
    listTasks: vi.fn(),
    createTask: vi.fn(),
    getTask: vi.fn(),
    updateTask: vi.fn(),
    deleteTask: vi.fn(),
    assignTask: vi.fn(),
    completeTask: vi.fn(),
    cancelTask: vi.fn(),
    listMyTasks: vi.fn(),
    getNotificationPreferences: vi.fn(),
    updateNotificationPreferences: vi.fn(),
    // Bravo methods — bravoCreateConversation must return a valid conversation
    // because BravoContext auto-launches a conversation when the panel opens
    // with no active conversation (cold start UX).
    bravoCreateConversation: vi.fn().mockResolvedValue({
      id: "c-auto",
      workspace_id: "1",
      user_id: "u1",
      project_id: "",
      title: "New conversation",
      status: "active",
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }),
    bravoListConversations: vi.fn(),
    bravoGetConversation: vi.fn(),
    bravoDeleteConversation: vi.fn(),
    bravoSendMessage: vi.fn(),
    bravoListMessages: vi.fn(),
    bravoApproveToolCall: vi.fn(),
    bravoDenyToolCall: vi.fn(),
    bravoCancelConversation: vi.fn(),
    bravoGetConfig: vi.fn(),
    bravoUpdateConfig: vi.fn(),
    bravoListTools: vi.fn(),
    bravoGetUsage: vi.fn(),
    bravoSendMessageSSE: vi.fn().mockReturnValue(new AbortController()),
    // Billing
    billingGetOverview: vi.fn(),
    billingGetUsage: vi.fn(),
    billingGetModelUsage: vi.fn(),
    billingCreateCheckout: vi.fn(),
    billingCreatePortal: vi.fn(),
    billingGetLedger: vi.fn(),
    ...overrides,
  } as ApiAdapter;
}

// ---------------------------------------------------------------------------
// SSE mock helper
// ---------------------------------------------------------------------------

/**
 * Creates a mock bravoSendMessageSSE that captures the handler and returns
 * a controller. The test can then call handler callbacks to simulate events.
 */
function createSSEMock() {
  let capturedHandler: BravoSSEHandler | null = null;
  const controller = new AbortController();

  const mockFn = vi.fn(
    (
      _ws: string,
      _convId: string,
      _content: string,
      handler: BravoSSEHandler,
      _mode?: string,
      _context?: unknown,
    ) => {
      capturedHandler = handler;
      return controller;
    },
  );

  return {
    mockFn,
    controller,
    getHandler: () => capturedHandler!,
  };
}

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

function BravoDisplay() {
  const { state, actions } = useBravo();
  return (
    <div>
      <span data-testid="panel-open">{String(state.panelOpen)}</span>
      <span data-testid="conv-count">{state.conversations.length}</span>
      <span data-testid="active-id">{state.activeConversation?.id ?? "none"}</span>
      <span data-testid="msg-count">{state.messages.length}</span>
      <span data-testid="streaming">{String(state.streaming)}</span>
      <span data-testid="streaming-content">{state.streamingContent}</span>
      <span data-testid="streaming-tc-count">{state.streamingToolCalls.length}</span>
      <span data-testid="loading">{String(state.loading)}</span>
      <button data-testid="open" onClick={actions.openPanel} />
      <button data-testid="close" onClick={actions.closePanel} />
      <button data-testid="toggle" onClick={actions.togglePanel} />
      <button data-testid="new-conv" onClick={() => actions.newConversation()} />
      <button data-testid="select-conv" onClick={() => actions.selectConversation(conv1)} />
      <button data-testid="delete-conv" onClick={() => actions.deleteConversation(conv1)} />
      <button data-testid="send" onClick={() => actions.sendMessage("hello")} />
      <button data-testid="cancel" onClick={actions.cancelStreaming} />
      <button data-testid="approve" onClick={() => actions.approveToolCall("tc1")} />
      <button data-testid="deny" onClick={() => actions.denyToolCall("tc1")} />
      <button data-testid="refresh" onClick={() => actions.refreshConversations()} />
    </div>
  );
}

function renderWithProviders(adapter: ApiAdapter) {
  return render(
    <ApiProvider adapter={adapter}>
      <WorkspaceProvider initialWorkspace={ws}>
        <BravoProvider>
          <BravoDisplay />
        </BravoProvider>
      </WorkspaceProvider>
    </ApiProvider>,
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("BravoContext", () => {
  it("throws when useBravo is called outside BravoProvider", () => {
    expect(() => render(<BravoDisplay />)).toThrow("useBravo must be used within a BravoProvider");
  });

  it("starts with panel closed and empty state", () => {
    const adapter = createMockAdapter();
    renderWithProviders(adapter);

    expect(screen.getByTestId("panel-open").textContent).toBe("false");
    expect(screen.getByTestId("conv-count").textContent).toBe("0");
    expect(screen.getByTestId("active-id").textContent).toBe("none");
    expect(screen.getByTestId("msg-count").textContent).toBe("0");
    expect(screen.getByTestId("streaming").textContent).toBe("false");
  });

  it("opens and closes the panel", () => {
    const adapter = createMockAdapter();
    renderWithProviders(adapter);

    act(() => screen.getByTestId("open").click());
    expect(screen.getByTestId("panel-open").textContent).toBe("true");

    act(() => screen.getByTestId("close").click());
    expect(screen.getByTestId("panel-open").textContent).toBe("false");
  });

  it("toggles the panel", () => {
    const adapter = createMockAdapter();
    renderWithProviders(adapter);

    act(() => screen.getByTestId("toggle").click());
    expect(screen.getByTestId("panel-open").textContent).toBe("true");

    act(() => screen.getByTestId("toggle").click());
    expect(screen.getByTestId("panel-open").textContent).toBe("false");
  });

  it("fetches conversations when panel opens", async () => {
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({
        conversations: [conv1, conv2],
        total: 2,
      }),
    });
    renderWithProviders(adapter);

    await act(async () => {
      screen.getByTestId("open").click();
    });

    // 2 fetched + 1 auto-launched = 3 conversations
    await waitFor(() => {
      expect(screen.getByTestId("conv-count").textContent).toBe("3");
    });
    expect(adapter.bravoListConversations).toHaveBeenCalledWith("acme", 50, 0);
  });

  it("creates a new conversation", async () => {
    const newConv: BravoConversation = { ...conv1, id: "c-new", title: "New" };
    // bravoCreateConversation is called twice: once by auto-launch, once by the explicit click.
    // Both return the same newConv (the mock always returns the same value).
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({ conversations: [], total: 0 }),
      bravoCreateConversation: vi.fn().mockResolvedValue(newConv),
    });
    renderWithProviders(adapter);

    // Open panel — triggers auto-launch (1 conversation created)
    await act(async () => {
      screen.getByTestId("open").click();
    });

    // Explicit create — adds another conversation
    await act(async () => {
      screen.getByTestId("new-conv").click();
    });

    await waitFor(() => {
      // Most recent creation becomes active
      expect(screen.getByTestId("active-id").textContent).toBe("c-new");
      // Auto-launch + explicit create = 2
      expect(screen.getByTestId("conv-count").textContent).toBe("2");
    });
  });

  it("selects a conversation and loads messages", async () => {
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({
        conversations: [conv1],
        total: 1,
      }),
      bravoGetConversation: vi.fn().mockResolvedValue({
        conversation: conv1,
        messages: [userMsg, assistantMsg],
      }),
    });
    renderWithProviders(adapter);

    await act(async () => {
      screen.getByTestId("open").click();
    });

    await act(async () => {
      screen.getByTestId("select-conv").click();
    });

    await waitFor(() => {
      expect(screen.getByTestId("active-id").textContent).toBe("c1");
      expect(screen.getByTestId("msg-count").textContent).toBe("2");
    });
  });

  it("deletes a conversation", async () => {
    const deleteFn = vi.fn().mockResolvedValue(undefined);
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({
        conversations: [conv1],
        total: 1,
      }),
      bravoGetConversation: vi.fn().mockResolvedValue({
        conversation: conv1,
        messages: [userMsg],
      }),
      bravoDeleteConversation: deleteFn,
    });
    renderWithProviders(adapter);

    // Open panel — fetches [conv1] + auto-launch adds c-auto
    await act(async () => {
      screen.getByTestId("open").click();
    });
    // Select conv1
    await act(async () => {
      screen.getByTestId("select-conv").click();
    });
    await waitFor(() => {
      expect(screen.getByTestId("active-id").textContent).toBe("c1");
    });

    // Delete conv1
    await act(async () => {
      screen.getByTestId("delete-conv").click();
    });

    // Verify delete API was called and conv1 is removed from the list
    await waitFor(() => {
      expect(deleteFn).toHaveBeenCalledWith("acme", "c1");
    });
  });

  it("sends a message via SSE and streams content", async () => {
    const sseMock = createSSEMock();
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({
        conversations: [conv1],
        total: 1,
      }),
      bravoGetConversation: vi.fn().mockResolvedValue({
        conversation: conv1,
        messages: [],
      }),
      bravoSendMessageSSE: sseMock.mockFn,
    });
    renderWithProviders(adapter);

    // Open and select conversation
    await act(async () => {
      screen.getByTestId("open").click();
    });
    await act(async () => {
      screen.getByTestId("select-conv").click();
    });

    // Send message — triggers SSE
    await act(async () => {
      screen.getByTestId("send").click();
    });

    // Should be streaming now with optimistic user message
    expect(screen.getByTestId("streaming").textContent).toBe("true");
    expect(screen.getByTestId("msg-count").textContent).toBe("1"); // optimistic user msg
    expect(sseMock.mockFn).toHaveBeenCalledWith(
      "acme",
      "c1",
      "hello",
      expect.any(Object),
      "ask",
      undefined,
    );

    // Simulate SSE events
    const handler = sseMock.getHandler();

    await act(async () => {
      handler.onMessageStart!({ id: "msg-a1", role: "assistant" });
    });

    await act(async () => {
      handler.onContentDelta!({ delta: "Hello " });
    });
    expect(screen.getByTestId("streaming-content").textContent).toBe("Hello ");

    await act(async () => {
      handler.onContentDelta!({ delta: "there!" });
    });
    expect(screen.getByTestId("streaming-content").textContent).toBe("Hello there!");

    // Complete the message
    await act(async () => {
      handler.onMessageEnd!({
        id: "msg-a1",
        usage: { input_tokens: 100, output_tokens: 50 },
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("streaming").textContent).toBe("false");
      expect(screen.getByTestId("msg-count").textContent).toBe("2"); // user + assistant
      expect(screen.getByTestId("streaming-content").textContent).toBe("");
    });
  });

  it("streams tool calls via SSE", async () => {
    const sseMock = createSSEMock();
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({ conversations: [conv1], total: 1 }),
      bravoGetConversation: vi.fn().mockResolvedValue({ conversation: conv1, messages: [] }),
      bravoSendMessageSSE: sseMock.mockFn,
    });
    renderWithProviders(adapter);

    await act(async () => {
      screen.getByTestId("open").click();
    });
    await act(async () => {
      screen.getByTestId("select-conv").click();
    });
    await act(async () => {
      screen.getByTestId("send").click();
    });

    const handler = sseMock.getHandler();
    await act(async () => {
      handler.onMessageStart!({ id: "msg-a2", role: "assistant" });
    });

    // Tool call starts
    await act(async () => {
      handler.onToolCallStart!({
        id: "tc-1",
        tool: "run_flow",
        input: { flow: "pseudo" },
      });
    });
    expect(screen.getByTestId("streaming-tc-count").textContent).toBe("1");

    // Tool call needs approval
    await act(async () => {
      handler.onNeedsApproval!({
        id: "tc-1",
        tool: "run_flow",
        input: { flow: "pseudo" },
      });
    });

    // Tool call ends
    await act(async () => {
      handler.onToolCallEnd!({
        id: "tc-1",
        status: "completed",
        output: { blocks: 42 },
        duration_ms: 1250,
      });
    });

    // Message ends
    await act(async () => {
      handler.onMessageEnd!({ id: "msg-a2" });
    });

    await waitFor(() => {
      expect(screen.getByTestId("streaming").textContent).toBe("false");
      expect(screen.getByTestId("streaming-tc-count").textContent).toBe("0");
      expect(screen.getByTestId("msg-count").textContent).toBe("2"); // user + assistant with tool calls
    });
  });

  it("cancelStreaming aborts the stream", async () => {
    const sseMock = createSSEMock();
    const abortSpy = vi.spyOn(sseMock.controller, "abort");
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({ conversations: [conv1], total: 1 }),
      bravoGetConversation: vi.fn().mockResolvedValue({ conversation: conv1, messages: [] }),
      bravoSendMessageSSE: sseMock.mockFn,
    });
    renderWithProviders(adapter);

    await act(async () => {
      screen.getByTestId("open").click();
    });
    await act(async () => {
      screen.getByTestId("select-conv").click();
    });
    await act(async () => {
      screen.getByTestId("send").click();
    });

    expect(screen.getByTestId("streaming").textContent).toBe("true");

    // Cancel
    act(() => {
      screen.getByTestId("cancel").click();
    });

    expect(screen.getByTestId("streaming").textContent).toBe("false");
    expect(abortSpy).toHaveBeenCalled();
  });

  it("handles SSE error gracefully", async () => {
    const sseMock = createSSEMock();
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({ conversations: [conv1], total: 1 }),
      bravoGetConversation: vi.fn().mockResolvedValue({ conversation: conv1, messages: [] }),
      bravoSendMessageSSE: sseMock.mockFn,
    });
    renderWithProviders(adapter);

    await act(async () => {
      screen.getByTestId("open").click();
    });
    await act(async () => {
      screen.getByTestId("select-conv").click();
    });
    await act(async () => {
      screen.getByTestId("send").click();
    });

    const handler = sseMock.getHandler();
    await act(async () => {
      handler.onError!({ error: "Internal server error" });
    });

    expect(screen.getByTestId("streaming").textContent).toBe("false");
  });

  it("calls approve and deny tool call APIs", async () => {
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockResolvedValue({ conversations: [conv1], total: 1 }),
      bravoGetConversation: vi.fn().mockResolvedValue({ conversation: conv1, messages: [] }),
      bravoApproveToolCall: vi.fn().mockResolvedValue(undefined),
      bravoDenyToolCall: vi.fn().mockResolvedValue(undefined),
    });
    renderWithProviders(adapter);

    await act(async () => {
      screen.getByTestId("open").click();
    });
    await act(async () => {
      screen.getByTestId("select-conv").click();
    });

    await act(async () => {
      screen.getByTestId("approve").click();
    });
    expect(adapter.bravoApproveToolCall).toHaveBeenCalledWith("acme", "c1", "tc1");

    await act(async () => {
      screen.getByTestId("deny").click();
    });
    expect(adapter.bravoDenyToolCall).toHaveBeenCalledWith("acme", "c1", "tc1");
  });

  it("handles fetch error silently", async () => {
    const adapter = createMockAdapter({
      bravoListConversations: vi.fn().mockRejectedValue(new Error("network")),
    });
    renderWithProviders(adapter);

    await act(async () => {
      screen.getByTestId("open").click();
    });

    // Fetch fails silently, but auto-launch still creates 1 conversation
    await waitFor(() => {
      expect(screen.getByTestId("conv-count").textContent).toBe("1");
      expect(screen.getByTestId("loading").textContent).toBe("false");
    });
  });

  it("refreshConversations re-fetches", async () => {
    const listFn = vi
      .fn()
      .mockResolvedValueOnce({ conversations: [conv1], total: 1 })
      .mockResolvedValueOnce({ conversations: [conv1, conv2], total: 2 });

    const adapter = createMockAdapter({ bravoListConversations: listFn });
    renderWithProviders(adapter);

    // First open triggers fetch (1) + auto-launch (1) = 2
    await act(async () => {
      screen.getByTestId("open").click();
    });
    await waitFor(() => {
      expect(screen.getByTestId("conv-count").textContent).toBe("2");
    });

    // Refresh replaces conversations list from API (auto-launch doesn't re-fire)
    await act(async () => {
      screen.getByTestId("refresh").click();
    });
    await waitFor(() => {
      expect(screen.getByTestId("conv-count").textContent).toBe("2");
    });
    expect(listFn).toHaveBeenCalledTimes(2);
  });
});
