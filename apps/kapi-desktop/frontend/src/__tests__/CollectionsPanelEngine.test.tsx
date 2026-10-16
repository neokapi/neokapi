import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";

// Stub the Wails-bridge API so the panel resolves file matches deterministically
// without a runtime. Declared before importing the component under test.
const matchContentMock = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    getKnownLocales: vi.fn().mockResolvedValue([]),
    getHomeDir: vi.fn().mockResolvedValue("/Users/dev"),
    listFormats: vi.fn().mockResolvedValue([]),
    getBasePath: vi.fn().mockResolvedValue("/p"),
    updateProject: vi.fn().mockResolvedValue(null),
    matchContent: (...args: unknown[]) => matchContentMock(...args),
    listProjectFiles: vi.fn().mockResolvedValue([]),
    listOutputs: vi.fn().mockResolvedValue({}),
    getProjectStatus: vi.fn().mockResolvedValue(null),
    getConvergence: vi.fn().mockResolvedValue(null),
    runExtract: vi.fn().mockResolvedValue({}),
    listFlows: vi.fn().mockResolvedValue([{ name: "translate", valid: true }]),
  },
}));

import { CollectionsPanel } from "../components/CollectionsPanel";
import { ErrorProvider } from "../components/ErrorBanner";
import type { FormatInfo, KapiProject } from "../types/api";

// The registry as it looks with okapi-bridge installed: two engines read
// .json, one reads .md.
const formats: FormatInfo[] = [
  {
    name: "json",
    display_name: "JSON",
    extensions: [".json"],
    has_reader: true,
    has_writer: true,
    has_schema: false,
    source: "built-in",
    engine: "native",
    priority: 50,
    shared_extensions: [".json"],
  },
  {
    name: "okf_json",
    display_name: "Okapi JSON",
    extensions: [".json"],
    has_reader: true,
    has_writer: true,
    has_schema: false,
    source: "okapi-bridge",
    engine: "okapi-bridge",
    priority: 25,
    shared_extensions: [".json"],
  },
  {
    name: "markdown",
    display_name: "Markdown",
    extensions: [".md"],
    has_reader: true,
    has_writer: true,
    has_schema: false,
    source: "built-in",
    engine: "native",
    priority: 50,
  },
];

function projectWith(format?: string): KapiProject {
  return {
    version: "v1",
    name: "Demo",
    defaults: { source_language: "en-US", target_languages: ["fr-FR"] },
    plugins: { "okapi-bridge": {} },
    collections: [
      {
        name: "Strings",
        content: [{ path: "src/*.json", format: format ? { name: format } : undefined }],
      },
    ],
    flows: { translate: { steps: [{ tool: "translate" }] } },
  };
}

async function openEditor() {
  const [edit] = await screen.findAllByRole("button", { name: "Edit collection" });
  await userEvent.click(edit);
}

describe("CollectionsPanel engine choice", () => {
  beforeEach(() => {
    matchContentMock.mockReset();
    matchContentMock.mockResolvedValue([
      {
        path: "/p/src/messages.json",
        relative: "src/messages.json",
        format: "json",
        pattern: "src/*.json",
        collection: "Strings",
        collection_index: 0,
        item_index: 0,
      },
    ]);
  });

  it("offers the engines that read the item's files and pins the chosen one's format", async () => {
    const onUpdate = vi.fn();
    render(
      <ErrorProvider>
        <CollectionsPanel
          project={projectWith()}
          onUpdate={onUpdate}
          tabID="t1"
          formatList={formats}
        />
      </ErrorProvider>,
    );
    await openEditor();

    const engine = await screen.findByRole("combobox", { name: "Engine" });
    expect(engine).toHaveTextContent("Auto (project order)");
    expect(screen.getByText("Read as json under the project's engine order")).toBeInTheDocument();

    await userEvent.click(engine);
    await userEvent.click(await screen.findByRole("option", { name: "okapi-bridge" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    const updated = onUpdate.mock.calls.at(-1)![0] as KapiProject;
    expect(updated.collections[0].content?.[0].format).toEqual({ name: "okf_json" });
  });

  it("shows the pinned engine and returns the item to the project order on Auto", async () => {
    const onUpdate = vi.fn();
    render(
      <ErrorProvider>
        <CollectionsPanel
          project={projectWith("okf_json")}
          onUpdate={onUpdate}
          tabID="t1"
          formatList={formats}
        />
      </ErrorProvider>,
    );
    await openEditor();

    const engine = await screen.findByRole("combobox", { name: "Engine" });
    expect(engine).toHaveTextContent("okapi-bridge");
    expect(screen.getByText("Pinned to okf_json")).toBeInTheDocument();

    await userEvent.click(engine);
    await userEvent.click(await screen.findByRole("option", { name: "Auto (project order)" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    const updated = onUpdate.mock.calls.at(-1)![0] as KapiProject;
    expect(updated.collections[0].content?.[0].format).toBeUndefined();
  });

  it("offers no choice when one engine reads the files", async () => {
    matchContentMock.mockResolvedValue([
      {
        path: "/p/docs/a.md",
        relative: "docs/a.md",
        format: "markdown",
        pattern: "docs/*.md",
        collection: "Docs",
        collection_index: 0,
        item_index: 0,
      },
    ]);
    const project: KapiProject = {
      ...projectWith(),
      collections: [{ name: "Docs", content: [{ path: "docs/*.md" }] }],
    };
    render(
      <ErrorProvider>
        <CollectionsPanel project={project} onUpdate={vi.fn()} tabID="t1" formatList={formats} />
      </ErrorProvider>,
    );
    await openEditor();
    await screen.findByText("Path pattern");
    expect(screen.queryByRole("combobox", { name: "Engine" })).not.toBeInTheDocument();
  });
});
