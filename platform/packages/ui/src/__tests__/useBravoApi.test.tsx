import { describe, it, expect, vi } from "vite-plus/test";
import { renderHook } from "@testing-library/react";
import { useBravoApi } from "../hooks/useBravoApi";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { ReactNode } from "react";
import type { Workspace } from "../types/api";

const ws: Workspace = {
  id: "1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

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
    bravoCreateConversation: vi.fn().mockResolvedValue({ id: "c1" }),
    bravoListConversations: vi.fn().mockResolvedValue({ conversations: [], total: 0 }),
    bravoGetConversation: vi.fn().mockResolvedValue({ conversation: {}, messages: [] }),
    bravoDeleteConversation: vi.fn().mockResolvedValue(undefined),
    bravoSendMessage: vi.fn().mockResolvedValue({ user_message: {}, assistant_message: {} }),
    bravoListMessages: vi.fn().mockResolvedValue({ messages: [] }),
    bravoApproveToolCall: vi.fn().mockResolvedValue(undefined),
    bravoDenyToolCall: vi.fn().mockResolvedValue(undefined),
    bravoCancelConversation: vi.fn().mockResolvedValue(undefined),
    bravoGetConfig: vi.fn().mockResolvedValue({}),
    bravoUpdateConfig: vi.fn().mockResolvedValue({}),
    bravoListTools: vi.fn().mockResolvedValue({ tools: [] }),
    bravoGetUsage: vi.fn().mockResolvedValue({}),
    ...overrides,
  } as ApiAdapter;
}

function createWrapper(adapter: ApiAdapter) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={ws}>{children}</WorkspaceProvider>
      </ApiProvider>
    );
  };
}

describe("useBravoApi", () => {
  it("delegates createConversation to the adapter with workspace slug", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.createConversation("proj-1", "My chat");
    expect(adapter.bravoCreateConversation).toHaveBeenCalledWith("acme", "proj-1", "My chat");
  });

  it("delegates listConversations with limit and offset", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.listConversations(10, 5);
    expect(adapter.bravoListConversations).toHaveBeenCalledWith("acme", 10, 5);
  });

  it("delegates getConversation", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.getConversation("c1");
    expect(adapter.bravoGetConversation).toHaveBeenCalledWith("acme", "c1");
  });

  it("delegates deleteConversation", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.deleteConversation("c1");
    expect(adapter.bravoDeleteConversation).toHaveBeenCalledWith("acme", "c1");
  });

  it("delegates sendMessage", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.sendMessage("c1", "hello");
    expect(adapter.bravoSendMessage).toHaveBeenCalledWith("acme", "c1", "hello");
  });

  it("delegates listMessages", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.listMessages("c1", 20, 0);
    expect(adapter.bravoListMessages).toHaveBeenCalledWith("acme", "c1", 20, 0);
  });

  it("delegates approveToolCall", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.approveToolCall("c1", "tc1");
    expect(adapter.bravoApproveToolCall).toHaveBeenCalledWith("acme", "c1", "tc1");
  });

  it("delegates denyToolCall", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.denyToolCall("c1", "tc1");
    expect(adapter.bravoDenyToolCall).toHaveBeenCalledWith("acme", "c1", "tc1");
  });

  it("delegates cancelConversation", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.cancelConversation("c1");
    expect(adapter.bravoCancelConversation).toHaveBeenCalledWith("acme", "c1");
  });

  it("delegates getConfig", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.getConfig();
    expect(adapter.bravoGetConfig).toHaveBeenCalledWith("acme");
  });

  it("delegates updateConfig", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.updateConfig({ enabled: true });
    expect(adapter.bravoUpdateConfig).toHaveBeenCalledWith("acme", { enabled: true });
  });

  it("delegates listTools", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.listTools();
    expect(adapter.bravoListTools).toHaveBeenCalledWith("acme");
  });

  it("delegates getUsage", async () => {
    const adapter = createMockAdapter();
    const { result } = renderHook(() => useBravoApi(), { wrapper: createWrapper(adapter) });

    await result.current.getUsage("2026-01-01", "2026-01-31");
    expect(adapter.bravoGetUsage).toHaveBeenCalledWith("acme", "2026-01-01", "2026-01-31");
  });
});
