// A collection that declares `base:` must still show its files.
//
// The panel used to join matched files to a collection by comparing the paths
// the recipe declares against each match's `pattern`. EffectiveItems folds a
// collection's base into that pattern, so for any collection with a base the
// two spellings differ and the join found nothing: the page showed per-
// collection block counts beside "No files matched this collection's patterns".
// The join is by the recipe index the resolver carries, and this holds it there.

import { render, screen, waitFor, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

const matchContent = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    getKnownLocales: vi.fn().mockResolvedValue([]),
    getHomeDir: vi.fn().mockResolvedValue("/home/dev"),
    listFormats: vi.fn().mockResolvedValue([]),
    getBasePath: vi.fn().mockResolvedValue("/p"),
    updateProject: vi.fn().mockResolvedValue(null),
    matchContent: (...args: unknown[]) => matchContent(...args),
    listProjectFiles: vi.fn().mockResolvedValue([]),
    listOutputs: vi.fn().mockResolvedValue({}),
    getProjectStatus: vi.fn().mockResolvedValue(null),
    getConvergence: vi.fn().mockResolvedValue(null),
    runExtract: vi.fn().mockResolvedValue({}),
    listFlows: vi.fn().mockResolvedValue([]),
  },
  call: vi.fn().mockResolvedValue(null),
}));

import { CollectionsPanel } from "../components/CollectionsPanel";
import { ErrorProvider } from "../components/ErrorBanner";
import { JobFeedProvider } from "../context/JobFeedContext";
import { ActiveFilterProvider } from "../context/ActiveFilterContext";
import type { KapiProject } from "../types/api";

/** KapiMart's shape: every collection declares a base. */
const project: KapiProject = {
  version: "v1",
  name: "KapiMart",
  defaults: { source_language: "en-US" },
  collections: [
    { name: "Website", base: "web", content: [{ path: "en/**/*.md" }] },
    { name: "Contracts", base: "legal", content: [{ path: "en/*.docx" }, { path: "en/*.pdf" }] },
  ],
};

/**
 * What the backend returns: `pattern` carries the base, and the indices address
 * the recipe rows. A test that omitted the base here would pass against the bug.
 */
const matches = [
  {
    path: "/p/web/en/index.md",
    relative: "web/en/index.md",
    format: "markdown",
    pattern: "web/en/**/*.md",
    collection: "Website",
    collection_index: 0,
    item_index: 0,
  },
  {
    path: "/p/web/en/about.md",
    relative: "web/en/about.md",
    format: "markdown",
    pattern: "web/en/**/*.md",
    collection: "Website",
    collection_index: 0,
    item_index: 0,
  },
  {
    path: "/p/legal/en/terms.docx",
    relative: "legal/en/terms.docx",
    format: "docx",
    pattern: "legal/en/*.docx",
    collection: "Contracts",
    collection_index: 1,
    item_index: 0,
  },
  {
    path: "/p/legal/en/privacy.pdf",
    relative: "legal/en/privacy.pdf",
    format: "pdf",
    pattern: "legal/en/*.pdf",
    collection: "Contracts",
    collection_index: 1,
    item_index: 1,
  },
];

function renderPanel() {
  return render(
    <ErrorProvider>
      <JobFeedProvider>
        <ActiveFilterProvider tabID="t1" enabled>
          <CollectionsPanel project={project} onUpdate={vi.fn()} tabID="t1" basePath="/p" />
        </ActiveFilterProvider>
      </JobFeedProvider>
    </ErrorProvider>,
  );
}

describe("CollectionsPanel with a based collection", () => {
  beforeEach(() => {
    matchContent.mockReset();
    matchContent.mockResolvedValue(matches);
  });

  it("counts the files of a collection that declares a base", async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText("Website")).toBeInTheDocument());
    // The founder's symptom: FILES read 0 on every collection while the block
    // counts beside them were not.
    const counts = await screen.findAllByTestId("collection-file-count");
    expect(counts.map((c) => c.textContent)).toEqual(["2", "2"]);
  });

  it("lists a based collection's matched files under it", async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText("Website")).toBeInTheDocument());
    await userEventClick("Website");
    await waitFor(() => expect(screen.getByText("web/en/index.md")).toBeInTheDocument());
    expect(screen.getByText("web/en/about.md")).toBeInTheDocument();
  });

  it("counts each pattern row separately", async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText("Contracts")).toBeInTheDocument());
    await userEventClick("Contracts");
    // Contracts is the second card; both carry an edit control.
    const editors = await screen.findAllByRole("button", { name: "Edit collection" });
    await userEvent.click(editors[1]);
    const counts = await screen.findAllByTestId("pattern-match-count");
    expect(counts).toHaveLength(2);
    expect(counts[0]).toHaveTextContent("1 file(s)");
    expect(counts[1]).toHaveTextContent("1 file(s)");
  });
});

/** Expand a collection card by its title. */
async function userEventClick(title: string) {
  const row = screen.getByText(title).closest("button");
  if (!row) throw new Error(`no card for ${title}`);
  await userEvent.click(within(row).getByText(title));
}
