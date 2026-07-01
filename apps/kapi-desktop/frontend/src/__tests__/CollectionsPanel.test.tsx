import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

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
    runExtract: vi.fn().mockResolvedValue({}),
    listFlows: vi.fn().mockResolvedValue([{ name: "translate", valid: true }]),
  },
}));

import { CollectionsPanel } from "../components/CollectionsPanel";
import { ErrorProvider } from "../components/ErrorBanner";
import type { KapiProject, ProjectStatus } from "../types/api";

const project: KapiProject = {
  version: "v1",
  name: "Demo",
  defaults: { source_language: "en-US", target_languages: ["fr-FR"] },
  content: [{ name: "Website", items: [{ path: "docs/**/*.md", format: { name: "markdown" } }] }],
  flows: { translate: { steps: [{ tool: "translate" }] } },
};

const status: ProjectStatus = {
  projectPath: "/p/demo.kapi",
  projectName: "Demo",
  hasData: true,
  collections: [
    {
      name: "Website",
      blockCount: 12,
      coverage: { "fr-FR": 6 },
      targetLanguages: ["fr-FR"],
    },
  ],
};

describe("CollectionsPanel per-collection Run", () => {
  beforeEach(() => {
    matchContentMock.mockReset();
    matchContentMock.mockResolvedValue([
      {
        path: "/p/docs/a.md",
        relative: "docs/a.md",
        format: "markdown",
        pattern: "docs/**/*.md",
        collection: "Website",
      },
    ]);
  });

  it("runs a flow scoped to the collection's files", async () => {
    const onRunFlow = vi.fn();
    render(
      <ErrorProvider>
        <CollectionsPanel
          project={project}
          onUpdate={vi.fn()}
          tabID="t1"
          flows={project.flows}
          onRunFlow={onRunFlow}
          status={status}
        />
      </ErrorProvider>,
    );

    // The Run affordance appears once the collection's files are resolved.
    const runBtn = await screen.findByRole("button", { name: "Run translate on Website" });
    await userEvent.click(runBtn);

    expect(onRunFlow).toHaveBeenCalledWith("translate", project.flows!.translate, {
      scopePaths: ["/p/docs/a.md"],
      scopeLabel: "Website",
    });
  });

  it("runs a flow across all collections from the scope-aware header picker", async () => {
    const onRunFlow = vi.fn();
    render(
      <ErrorProvider>
        <CollectionsPanel
          project={project}
          onUpdate={vi.fn()}
          tabID="t1"
          flows={project.flows}
          onRunFlow={onRunFlow}
          status={status}
        />
      </ErrorProvider>,
    );
    // With nothing selected, the header runs the flow across the whole project
    // (no explicit scope — the runner narrows by the active filter).
    const runAll = await screen.findByRole("button", { name: "Run translate on all collections" });
    await userEvent.click(runAll);
    expect(onRunFlow).toHaveBeenCalledWith("translate", project.flows!.translate);
  });

  it("hides the per-collection Run when the collection has no matched files", async () => {
    matchContentMock.mockResolvedValue([]);
    render(
      <ErrorProvider>
        <CollectionsPanel
          project={project}
          onUpdate={vi.fn()}
          tabID="t1"
          flows={project.flows}
          onRunFlow={vi.fn()}
          status={status}
        />
      </ErrorProvider>,
    );
    // The card renders (block badge), but its per-collection Run is absent.
    await waitFor(() => expect(screen.getByText("12 blocks")).toBeInTheDocument());
    expect(
      screen.queryByRole("button", { name: "Run translate on Website" }),
    ).not.toBeInTheDocument();
    // The header runner is present but disabled (nothing matched to run on).
    expect(screen.getByRole("button", { name: "Run translate on all collections" })).toBeDisabled();
  });

  it("runs a flow across the selected collections via the batch bar", async () => {
    const onRunFlow = vi.fn();
    render(
      <ErrorProvider>
        <CollectionsPanel
          project={project}
          onUpdate={vi.fn()}
          tabID="t1"
          flows={project.flows}
          onRunFlow={onRunFlow}
          status={status}
        />
      </ErrorProvider>,
    );

    // Tick the collection → the batch-run bar appears.
    const checkbox = await screen.findByRole("checkbox", { name: "Select Website" });
    await userEvent.click(checkbox);

    const runBtn = await screen.findByRole("button", {
      name: "Run translate on selected collections",
    });
    await userEvent.click(runBtn);

    expect(onRunFlow).toHaveBeenCalledWith("translate", project.flows!.translate, {
      scopePaths: ["/p/docs/a.md"],
      scopeLabel: "1 collections",
    });
  });
});
