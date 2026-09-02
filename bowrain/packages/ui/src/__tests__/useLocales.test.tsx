import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { useLocales } from "../hooks/useLocales";
import type { ApiAdapter } from "../api/adapter";
import type { LocaleInfo } from "../types/api";

const mockLocales: LocaleInfo[] = [
  { code: "en", display_name: "English" },
  { code: "fr", display_name: "French" },
  { code: "de", display_name: "German" },
];

function createMockAdapter(locales: LocaleInfo[] = mockLocales): ApiAdapter {
  return {
    getKnownLocales: vi.fn().mockResolvedValue(locales),
    // stub the rest as vi.fn()
    getConfig: vi.fn(),
    getPublicPlatformConfig: vi.fn(),
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
    memoryTranslateFile: vi.fn(),
    getWordCount: vi.fn(),
    exportTranslatedFile: vi.fn(),
    lookupMemoryForBlock: vi.fn(),
    lookupTermsForBlock: vi.fn(),
    runCheck: vi.fn(),
    runFileCheck: vi.fn(),
    renderDocumentPreview: vi.fn(),
    renderBlockHTML: vi.fn(),
    getMemoryEntries: vi.fn(),
    getMemoryCount: vi.fn(),
    addMemoryEntry: vi.fn(),
    updateMemoryEntry: vi.fn(),
    deleteMemoryEntry: vi.fn(),
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
  } as ApiAdapter;
}

/** Fresh QueryClient per render: useLocales caches via react-query. */
function renderWithProviders(adapter: ApiAdapter) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={adapter}>
        <LocaleDisplay />
      </ApiProvider>
    </QueryClientProvider>,
  );
}

function LocaleDisplay() {
  const { locales, loading, error, getDisplayName } = useLocales();
  return (
    <div>
      <span data-testid="loading">{loading ? "yes" : "no"}</span>
      <span data-testid="error">{error ?? "none"}</span>
      <span data-testid="count">{locales.length}</span>
      <span data-testid="display-en">{getDisplayName("en")}</span>
      <span data-testid="display-xx">{getDisplayName("xx")}</span>
    </div>
  );
}

describe("useLocales", () => {
  it("fetches and exposes locales", async () => {
    const adapter = createMockAdapter();
    renderWithProviders(adapter);

    await waitFor(() => {
      expect(screen.getByTestId("loading").textContent).toBe("no");
    });

    expect(screen.getByTestId("count").textContent).toBe("3");
    // eslint-disable-next-line typescript-eslint/unbound-method -- vitest spy
    expect(adapter.getKnownLocales).toHaveBeenCalledOnce();
  });

  it("getDisplayName returns name for known locale", async () => {
    renderWithProviders(createMockAdapter());

    await waitFor(() => {
      expect(screen.getByTestId("display-en").textContent).toBe("English");
    });
  });

  it("getDisplayName falls back to code for unknown locale", async () => {
    renderWithProviders(createMockAdapter());

    await waitFor(() => {
      expect(screen.getByTestId("display-xx").textContent).toBe("xx");
    });
  });

  it("handles API errors", async () => {
    const adapter = createMockAdapter();
    (adapter.getKnownLocales as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Network error"),
    );

    renderWithProviders(adapter);

    await waitFor(() => {
      expect(screen.getByTestId("error").textContent).toBe("Network error");
    });
    expect(screen.getByTestId("count").textContent).toBe("0");
  });
});
