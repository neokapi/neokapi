import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
    getConfig: vi.fn(), getCurrentUser: vi.fn(), listWorkspaces: vi.fn(),
    createWorkspace: vi.fn(), getWorkspace: vi.fn(), updateWorkspace: vi.fn(),
    deleteWorkspace: vi.fn(), listMembers: vi.fn(), addMember: vi.fn(),
    updateMemberRole: vi.fn(), removeMember: vi.fn(), listProjects: vi.fn(),
    createProject: vi.fn(), getProject: vi.fn(), deleteProject: vi.fn(),
    uploadFiles: vi.fn(), removeFile: vi.fn(), getFileBlocks: vi.fn(),
    updateBlockTarget: vi.fn(), updateBlockTargetCoded: vi.fn(),
    pseudoTranslateFile: vi.fn(), aiTranslateFile: vi.fn(), tmTranslateFile: vi.fn(),
    getWordCount: vi.fn(), exportTranslatedFile: vi.fn(), lookupTMForBlock: vi.fn(),
    lookupTermsForBlock: vi.fn(), getTMEntries: vi.fn(), getTMCount: vi.fn(),
    addTMEntry: vi.fn(), updateTMEntry: vi.fn(), deleteTMEntry: vi.fn(),
    getTerms: vi.fn(), getTermCount: vi.fn(), addConcept: vi.fn(),
    updateConcept: vi.fn(), deleteConcept: vi.fn(), importTermsCSV: vi.fn(),
    importTermsJSON: vi.fn(), exportTermsJSON: vi.fn(), listProviderConfigs: vi.fn(),
    saveProviderConfig: vi.fn(), deleteProviderConfig: vi.fn(),
    testProviderConfig: vi.fn(), listFormats: vi.fn(), listTools: vi.fn(),
    listInvites: vi.fn(), createInvite: vi.fn(), deleteInvite: vi.fn(),
    acceptInvite: vi.fn(),
  } as ApiAdapter;
}

function Wrapper({ children }: { children: React.ReactNode }) {
  return <ApiProvider adapter={createMockAdapter()}>{children}</ApiProvider>;
}

/* ── LocaleSelect ── */

describe("LocaleSelect", () => {
  it("renders the trigger with display value", async () => {
    const onChange = vi.fn();
    render(
      <Wrapper>
        <LocaleSelect value="en" onChange={onChange} data-testid="src" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("src-trigger")).toHaveTextContent("English (en)");
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
      expect(screen.getByTestId("src-trigger")).toHaveTextContent("English");
    });

    await user.click(screen.getByTestId("src-trigger"));
    expect(screen.getByTestId("src-option-fr")).toBeInTheDocument();
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
      expect(screen.getByTestId("src-trigger")).toHaveTextContent("English");
    });

    await user.click(screen.getByTestId("src-trigger"));
    await user.click(screen.getByTestId("src-option-fr"));

    expect(onChange).toHaveBeenCalledWith("fr");
    // Dropdown should be closed
    expect(screen.queryByTestId("src-option-fr")).not.toBeInTheDocument();
  });

  it("filters locales by search", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Wrapper>
        <LocaleSelect value="en" onChange={onChange} data-testid="src" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("src-trigger")).toHaveTextContent("English");
    });

    await user.click(screen.getByTestId("src-trigger"));
    await user.type(screen.getByTestId("src-search"), "Ger");

    expect(screen.getByTestId("src-option-de")).toBeInTheDocument();
    expect(screen.queryByTestId("src-option-fr")).not.toBeInTheDocument();
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
      expect(screen.getByTestId("src-trigger")).toHaveTextContent("English");
    });

    await user.click(screen.getByTestId("src-trigger"));
    await user.click(screen.getByTestId("src-option-de"));

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

  it("opens dropdown on chip area click", async () => {
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

    await user.click(screen.getByTestId("tgt-chips"));
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

    await user.click(screen.getByTestId("tgt-chips"));
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

    await user.click(screen.getByTestId("tgt-chips"));
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

    await user.click(screen.getByTestId("tgt-chips"));
    await user.type(screen.getByTestId("tgt-search"), "Jap");

    expect(screen.getByTestId("tgt-option-ja")).toBeInTheDocument();
    expect(screen.queryByTestId("tgt-option-en")).not.toBeInTheDocument();
    expect(screen.queryByTestId("tgt-option-de")).not.toBeInTheDocument();
  });

  it("shows 'All locales selected' when all are chosen", async () => {
    const user = userEvent.setup();
    const allCodes = mockLocales.map((l) => l.code);
    render(
      <Wrapper>
        <MultiLocaleSelect value={allCodes} onChange={vi.fn()} data-testid="tgt" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tgt-remove-en")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("tgt-chips"));
    expect(screen.getByText("All locales selected")).toBeInTheDocument();
  });
});
