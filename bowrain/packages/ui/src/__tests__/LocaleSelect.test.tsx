import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { LocaleSelect, MultiLocaleSelect } from "../components/LocaleSelect";
import type { ApiAdapter } from "../api/adapter";
import type { LocaleInfo } from "../types/api";

const mockLocales: LocaleInfo[] = [
  { code: "en", display_name: "English" },
  { code: "fr", display_name: "French" },
  { code: "de", display_name: "German" },
  { code: "es", display_name: "Spanish" },
  { code: "ja", display_name: "Japanese" },
];

function createMockAdapter(locales: LocaleInfo[] = mockLocales): ApiAdapter {
  return {
    getKnownLocales: vi.fn().mockResolvedValue(locales),
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
    getBlockHistory: vi.fn(),
    addBlockNote: vi.fn(),
    listBlockNotes: vi.fn(),
    deleteBlockNote: vi.fn(),
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

function Wrapper({ children }: { children: React.ReactNode }) {
  const [qc] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false } } }));
  return (
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={createMockAdapter()}>{children}</ApiProvider>
    </QueryClientProvider>
  );
}

/* ── LocaleSelect (backed by Combobox input) ── */

describe("LocaleSelect", () => {
  it("names a locale the catalog has never heard of", async () => {
    // The known-locale catalog is a curated list of major tags, so a project
    // target like fr-FR is not in it. The picker must still say French.
    const onChange = vi.fn();
    render(
      <Wrapper>
        <LocaleSelect value="fr-FR" onChange={onChange} codes={["en", "fr-FR"]} data-testid="src" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("combobox")).toHaveValue("French (France) (fr-FR)");
    });
  });

  it("renders the combobox with display value", async () => {
    const onChange = vi.fn();
    render(
      <Wrapper>
        <LocaleSelect value="en" onChange={onChange} data-testid="src" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("combobox")).toHaveValue("English (en)");
    });
  });

  it("opens dropdown on trigger click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <LocaleSelect value="en" onChange={onChange} data-testid="src" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("combobox")).toHaveValue("English (en)");
    });

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByRole("option", { name: /French \(fr\)/ })).toBeInTheDocument();
  });

  it("selects a locale and closes dropdown", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <LocaleSelect value="en" onChange={onChange} data-testid="src" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("combobox")).toHaveValue("English (en)");
    });

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByRole("option", { name: /French \(fr\)/ }));

    expect(onChange).toHaveBeenCalledWith("fr");
  });

  it("accepts search text in the combobox input", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <LocaleSelect value="en" onChange={onChange} data-testid="src" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("combobox")).toHaveValue("English (en)");
    });

    // Click to open, then type to search
    await user.click(screen.getByRole("combobox"));
    await user.clear(screen.getByRole("combobox"));
    await user.type(screen.getByRole("combobox"), "Ger");

    expect(screen.getByRole("combobox")).toHaveValue("Ger");
    // German option should still be available
    expect(screen.getByRole("option", { name: /German \(de\)/ })).toBeInTheDocument();
  });

  it("works correctly inside a <label> element", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <label>
          Source Language
          <LocaleSelect value="en" onChange={onChange} data-testid="src" />
        </label>
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("combobox")).toHaveValue("English (en)");
    });

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByRole("option", { name: /German \(de\)/ }));

    expect(onChange).toHaveBeenCalledWith("de");
    expect(onChange).toHaveBeenCalledTimes(1);
  });
});

/* ── MultiLocaleSelect ── */

describe("MultiLocaleSelect", () => {
  it("renders chips for selected values", async () => {
    const onChange = vi.fn();
    render(
      <Wrapper>
        <MultiLocaleSelect value={["fr", "de"]} onChange={onChange} data-testid="tgt" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-fr")).toBeInTheDocument();
    });
    expect(screen.getByTestId("tgt-remove-de")).toBeInTheDocument();
  });

  it("opens dropdown on search input click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <MultiLocaleSelect value={["fr"]} onChange={onChange} data-testid="tgt" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-fr")).toBeInTheDocument();
    });
    // The add-picker mounts once the cached locale list resolves.
    await waitFor(() => {
      expect(screen.getByTestId("tgt-search")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("tgt-search"));
    // "fr" already selected, so "en", "de", "es", "ja" should appear
    expect(screen.getByTestId("tgt-option-en")).toBeInTheDocument();
    expect(screen.getByTestId("tgt-option-de")).toBeInTheDocument();
    expect(screen.queryByTestId("tgt-option-fr")).not.toBeInTheDocument();
  });

  it("adds a locale when clicking an option", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <MultiLocaleSelect value={["fr"]} onChange={onChange} data-testid="tgt" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-fr")).toBeInTheDocument();
    });
    // The add-picker mounts once the cached locale list resolves.
    await waitFor(() => {
      expect(screen.getByTestId("tgt-search")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("tgt-search"));
    await user.click(screen.getByTestId("tgt-option-de"));

    expect(onChange).toHaveBeenCalledWith(["fr", "de"]);
  });

  it("removes a locale when clicking the chip remove button", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <MultiLocaleSelect value={["fr", "de"]} onChange={onChange} data-testid="tgt" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-fr")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("tgt-remove-fr"));

    expect(onChange).toHaveBeenCalledWith(["de"]);
  });

  it("works correctly inside a <label> (regression test)", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <label>
          Target Languages
          <MultiLocaleSelect value={["fr"]} onChange={onChange} data-testid="tgt" />
        </label>
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-fr")).toBeInTheDocument();
    });
    // The add-picker mounts once the cached locale list resolves.
    await waitFor(() => {
      expect(screen.getByTestId("tgt-search")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("tgt-search"));
    await user.click(screen.getByTestId("tgt-option-de"));

    // Should only call onChange once — to add "de"
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith(["fr", "de"]);
  });

  it("filters available locales by search", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <MultiLocaleSelect value={["fr"]} onChange={onChange} data-testid="tgt" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-fr")).toBeInTheDocument();
    });
    // The add-picker mounts once the cached locale list resolves.
    await waitFor(() => {
      expect(screen.getByTestId("tgt-search")).toBeInTheDocument();
    });

    const searchInput = screen.getByTestId("tgt-search");
    await user.click(searchInput);
    await user.type(searchInput, "Jap");

    // Non-matching options are filtered out of the a11y tree (base-ui hides them).
    expect(screen.getByRole("option", { name: /Japanese/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /English/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /German/ })).not.toBeInTheDocument();
  });

  it("shows 'All locales selected' when all are chosen", async () => {
    const allCodes = mockLocales.map((l) => l.code);
    render(
      <Wrapper>
        <MultiLocaleSelect value={allCodes} onChange={vi.fn()} data-testid="tgt" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-en")).toBeInTheDocument();
    });

    // With every locale selected the add-picker collapses to an inline hint.
    expect(screen.getByText("All locales selected")).toBeInTheDocument();
  });
});
