import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

// The content explorer's file tree: the *pattern* is the node, and the concrete
// per-locale files — the source among them, labelled — are its children. The
// old shape nested source → pattern → locales, which repeated the same path
// twice whenever the locale was not a whole path segment.

const matchContentMock = vi.fn();
const listOutputsMock = vi.fn();

vi.mock("../hooks/useApi", () => ({
  api: {
    getKnownLocales: vi.fn().mockResolvedValue([]),
    getHomeDir: vi.fn().mockResolvedValue("/Users/dev"),
    listFormats: vi.fn().mockResolvedValue([]),
    getBasePath: vi.fn().mockResolvedValue("/p"),
    updateProject: vi.fn().mockResolvedValue(null),
    matchContent: (...args: unknown[]) => matchContentMock(...args),
    listProjectFiles: vi.fn().mockResolvedValue([]),
    listOutputs: (...args: unknown[]) => listOutputsMock(...args),
    getProjectStatus: vi.fn().mockResolvedValue(null),
    getConvergence: vi.fn().mockResolvedValue(null),
    runExtract: vi.fn().mockResolvedValue({}),
    listFlows: vi.fn().mockResolvedValue([]),
  },
}));

import { CollectionsPanel, outputPattern, templatize } from "../components/CollectionsPanel";
import { ErrorProvider } from "../components/ErrorBanner";
import type { KapiProject, OutputFileInfo } from "../types/api";

const project: KapiProject = {
  version: "v1",
  name: "Demo",
  defaults: { source_language: "en-US", target_languages: ["de", "fr"] },
  collections: [
    { name: "Website", content: [{ path: "src/en-US/*.txt", target: "src/{lang}/*.txt" }] },
  ],
};

function out(lang: string, relative: string, exists = true): OutputFileInfo {
  return { lang, relative, path: `/p/${relative}`, format: "text", exists, size: 42 };
}

function panel() {
  return (
    <ErrorProvider>
      <CollectionsPanel project={project} onUpdate={vi.fn()} tabID="t1" />
    </ErrorProvider>
  );
}

/** Render the panel and expand the "Website" collection card (collapsed by default). */
async function renderExpanded() {
  render(panel());
  await userEvent.click(await screen.findByRole("button", { name: "Expand" }));
}

describe("templatize", () => {
  it("replaces a whole path segment", () => {
    expect(templatize("src/de/file.txt", "de")).toBe("src/{lang}/file.txt");
    expect(templatize("output/de-DE/docs/api.md", "de-DE")).toBe("output/{lang}/docs/api.md");
  });

  it("replaces a file stem — the case the old segment-equality test missed", () => {
    expect(templatize("locales/fr-FR.json", "fr-FR")).toBe("locales/{lang}.json");
    expect(templatize("de.json", "de")).toBe("{lang}.json");
  });

  it("replaces a delimited suffix", () => {
    expect(templatize("messages_de-DE.properties", "de-DE")).toBe("messages_{lang}.properties");
    expect(templatize("values-fr/strings.xml", "fr")).toBe("values-{lang}/strings.xml");
  });

  it("never replaces the locale letters inside a longer word", () => {
    expect(templatize("de/render.md", "de")).toBe("{lang}/render.md");
    expect(templatize("docs/design.md", "de")).toBe("docs/design.md");
    expect(templatize("src/deprecated/notes.md", "de")).toBe("src/deprecated/notes.md");
  });

  it("is a no-op without a locale", () => {
    expect(templatize("src/file.txt", "")).toBe("src/file.txt");
  });
});

describe("outputPattern", () => {
  it("derives the shared pattern across locales", () => {
    expect(outputPattern([out("de", "src/de/file.txt"), out("fr", "src/fr/file.txt")])).toBe(
      "src/{lang}/file.txt",
    );
    expect(
      outputPattern([out("de-DE", "locales/de-DE.json"), out("fr-FR", "locales/fr-FR.json")]),
    ).toBe("locales/{lang}.json");
  });

  it("reports no pattern when the locales' targets disagree", () => {
    expect(outputPattern([out("de", "src/de/file.txt"), out("fr", "i18n/fr.json")])).toBe("");
  });

  it("reports no pattern for an empty output list", () => {
    expect(outputPattern([])).toBe("");
  });
});

describe("content explorer tree", () => {
  beforeEach(() => {
    matchContentMock.mockReset();
    listOutputsMock.mockReset();
    matchContentMock.mockResolvedValue([
      {
        path: "/p/src/en-US/file.txt",
        relative: "src/en-US/file.txt",
        format: "text",
        pattern: "src/en-US/*.txt",
        collection: "Website",
        collection_index: 0,
        item_index: 0,
      },
    ]);
    listOutputsMock.mockResolvedValue({
      "src/en-US/file.txt": [out("de", "src/de/file.txt"), out("fr", "src/fr/file.txt", false)],
    });
  });

  it("shows the pattern as the node, not the source file", async () => {
    await renderExpanded();
    await waitFor(() => expect(screen.getByText("src/{lang}/file.txt")).toBeInTheDocument());
    // Collapsed: only the pattern node is visible — no concrete path yet.
    expect(screen.queryByText("src/en-US/file.txt")).not.toBeInTheDocument();
    expect(screen.queryByText("src/de/file.txt")).not.toBeInTheDocument();
  });

  it("expands to the source (labelled) plus one row per locale — and no middle level", async () => {
    await renderExpanded();
    const node = await screen.findByText("src/{lang}/file.txt");
    await userEvent.click(node);

    // The source file, labelled as such.
    expect(await screen.findByText("src/en-US/file.txt")).toBeInTheDocument();
    expect(screen.getByText("source")).toBeInTheDocument();
    // Each concrete target locale.
    expect(screen.getByText("src/de/file.txt")).toBeInTheDocument();
    expect(screen.getByText("src/fr/file.txt")).toBeInTheDocument();
    // The pattern appears exactly once — the old tree rendered it twice, once
    // as a middle row under the source and once (concretely) as a leaf.
    expect(screen.getAllByText("src/{lang}/file.txt")).toHaveLength(1);
    // Exactly three leaves under one node: source + de + fr.
    expect(document.querySelectorAll('[data-slot="matched-pattern-row"]')).toHaveLength(1);
    expect(document.querySelectorAll('[data-slot="matched-source-row"]')).toHaveLength(1);
    expect(document.querySelectorAll('[data-slot="matched-output-row"]')).toHaveLength(2);
  });

  it("keeps a not-yet-generated locale visible but marked pending", async () => {
    await renderExpanded();
    await userEvent.click(await screen.findByText("src/{lang}/file.txt"));
    expect(await screen.findByText("pending")).toBeInTheDocument();
  });

  it("collapses the pattern node again", async () => {
    await renderExpanded();
    const node = await screen.findByText("src/{lang}/file.txt");
    await userEvent.click(node);
    expect(await screen.findByText("src/de/file.txt")).toBeInTheDocument();
    await userEvent.click(node);
    await waitFor(() => expect(screen.queryByText("src/de/file.txt")).not.toBeInTheDocument());
  });

  it("falls back to the source file as the node when there are no outputs", async () => {
    listOutputsMock.mockResolvedValue({});
    await renderExpanded();
    expect(await screen.findByText("src/en-US/file.txt")).toBeInTheDocument();
    expect(screen.queryByText("src/{lang}/file.txt")).not.toBeInTheDocument();
    expect(document.querySelectorAll('[data-slot="matched-pattern-row"]')).toHaveLength(0);
  });
});
