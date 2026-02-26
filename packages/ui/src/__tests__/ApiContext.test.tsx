import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ApiProvider, useApi } from "../context/ApiContext";
import type { ApiAdapter } from "../api/adapter";

function ApiDisplay() {
  const api = useApi();
  return <span data-testid="has-api">{api ? "yes" : "no"}</span>;
}

function createMockAdapter(): ApiAdapter {
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
  } as ApiAdapter;
}

describe("ApiContext", () => {
  it("provides the adapter to children", () => {
    const adapter = createMockAdapter();
    render(
      <ApiProvider adapter={adapter}>
        <ApiDisplay />
      </ApiProvider>,
    );
    expect(screen.getByTestId("has-api").textContent).toBe("yes");
  });

  it("throws when useApi is called outside ApiProvider", () => {
    expect(() => render(<ApiDisplay />)).toThrow(
      "useApi must be used within an ApiProvider",
    );
  });
});
