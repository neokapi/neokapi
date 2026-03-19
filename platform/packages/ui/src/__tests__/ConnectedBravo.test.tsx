import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent, act, waitFor } from "@testing-library/react";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { BravoProvider, useBravo } from "../context/BravoContext";
import { BravoPanelTrigger } from "../components/bravo/BravoPanelTrigger";
import { BravoPanel } from "../components/bravo/BravoPanel";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace, BravoConversation, BravoMessage } from "../types/api";

// ---------------------------------------------------------------------------
// This file tests the ConnectedBravo component pattern used in
// workspace-layout.tsx — a BravoPanelTrigger + BravoPanel wired to
// BravoContext via useBravo().
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

const conv: BravoConversation = {
  id: "c1",
  workspace_id: "1",
  user_id: "u1",
  project_id: "",
  title: "Test conv",
  status: "active",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
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
  content: "Hi!",
  created_at: "2026-01-01T00:00:01Z",
};

// ---------------------------------------------------------------------------
// Mock adapter (only bravo methods need realistic behaviour)
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
    bravoCreateConversation: vi.fn().mockResolvedValue(conv),
    bravoListConversations: vi.fn().mockResolvedValue({
      conversations: [conv],
      total: 1,
    }),
    bravoGetConversation: vi.fn().mockResolvedValue({
      conversation: conv,
      messages: [userMsg, assistantMsg],
    }),
    bravoDeleteConversation: vi.fn().mockResolvedValue(undefined),
    bravoSendMessage: vi.fn().mockResolvedValue({
      user_message: userMsg,
      assistant_message: assistantMsg,
    }),
    bravoListMessages: vi.fn(),
    bravoApproveToolCall: vi.fn().mockResolvedValue(undefined),
    bravoDenyToolCall: vi.fn().mockResolvedValue(undefined),
    bravoCancelConversation: vi.fn(),
    bravoGetConfig: vi.fn(),
    bravoUpdateConfig: vi.fn(),
    bravoListTools: vi.fn(),
    bravoGetUsage: vi.fn(),
    ...overrides,
  } as ApiAdapter;
}

// ---------------------------------------------------------------------------
// ConnectedBravo — mirrors the component in workspace-layout.tsx
// ---------------------------------------------------------------------------

function ConnectedBravo() {
  const { state, actions } = useBravo();

  return (
    <>
      <BravoPanelTrigger onClick={actions.togglePanel} active={state.panelOpen} />
      <BravoPanel
        open={state.panelOpen}
        onOpenChange={(open) => (open ? actions.openPanel() : actions.closePanel())}
        conversations={state.conversations}
        activeConversation={state.activeConversation}
        messages={state.messages}
        streaming={state.streaming}
        streamingContent={state.streamingContent}
        streamingToolCalls={state.streamingToolCalls}
        onNewConversation={() => void actions.newConversation()}
        onSelectConversation={(c) => void actions.selectConversation(c)}
        onDeleteConversation={(c) => void actions.deleteConversation(c)}
        onSendMessage={(content) => void actions.sendMessage(content)}
        onApproveToolCall={(id) => void actions.approveToolCall(id)}
        onDenyToolCall={(id) => void actions.denyToolCall(id)}
        loading={state.loading}
        sendDisabled={state.streaming}
      />
    </>
  );
}

function renderConnectedBravo(adapter: ApiAdapter) {
  return render(
    <ApiProvider adapter={adapter}>
      <WorkspaceProvider initialWorkspace={ws}>
        <BravoProvider>
          <ConnectedBravo />
        </BravoProvider>
      </WorkspaceProvider>
    </ApiProvider>,
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ConnectedBravo", () => {
  it("renders the @bravo trigger button", () => {
    const adapter = createMockAdapter();
    renderConnectedBravo(adapter);
    expect(screen.getByLabelText("Toggle @bravo assistant")).toBeDefined();
  });

  it("opens the panel when trigger is clicked", async () => {
    const adapter = createMockAdapter();
    renderConnectedBravo(adapter);

    await act(async () => {
      fireEvent.click(screen.getByLabelText("Toggle @bravo assistant"));
    });

    // Panel header shows "@bravo" as the title in list view
    // and fetches conversations
    await waitFor(() => {
      expect(adapter.bravoListConversations).toHaveBeenCalledWith("acme", 50, 0);
    });
  });

  it("creates a new conversation via the panel", async () => {
    const adapter = createMockAdapter();
    renderConnectedBravo(adapter);

    // Open panel
    await act(async () => {
      fireEvent.click(screen.getByLabelText("Toggle @bravo assistant"));
    });

    // Wait for conversations to load
    await waitFor(() => {
      expect(screen.getByText("New conversation")).toBeDefined();
    });

    // Click new conversation
    await act(async () => {
      fireEvent.click(screen.getByText("New conversation"));
    });

    expect(adapter.bravoCreateConversation).toHaveBeenCalledWith("acme", undefined);
  });

  it("selects a conversation and loads messages", async () => {
    const adapter = createMockAdapter();
    renderConnectedBravo(adapter);

    // Open panel
    await act(async () => {
      fireEvent.click(screen.getByLabelText("Toggle @bravo assistant"));
    });

    // Wait for conversation list
    await waitFor(() => {
      expect(screen.getByText("Test conv")).toBeDefined();
    });

    // Select the conversation
    await act(async () => {
      fireEvent.click(screen.getByText("Test conv"));
    });

    await waitFor(() => {
      expect(adapter.bravoGetConversation).toHaveBeenCalledWith("acme", "c1");
    });
  });

  it("closes the panel when trigger is clicked again", async () => {
    const adapter = createMockAdapter();
    renderConnectedBravo(adapter);

    // Open
    await act(async () => {
      fireEvent.click(screen.getByLabelText("Toggle @bravo assistant"));
    });

    // Close — use getAllByText since the panel title also says "@bravo"
    await act(async () => {
      const triggers = screen.getAllByText("@bravo");
      fireEvent.click(triggers[0]); // first match is the trigger button
    });

    // Panel should not be fetching new conversations after close
    // (initial open triggered 1 call)
    expect(adapter.bravoListConversations).toHaveBeenCalledTimes(1);
  });
});
