import { render, waitFor } from "./testUtils";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Events } from "@wailsio/runtime";

const copyFileToProject = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    getKnownLocales: vi.fn().mockResolvedValue([]),
    getHomeDir: vi.fn().mockResolvedValue("/home/dev"),
    listFormats: vi.fn().mockResolvedValue([]),
    getBasePath: vi.fn().mockResolvedValue("/p"),
    updateProject: vi.fn().mockResolvedValue(null),
    matchContent: vi.fn().mockResolvedValue([]),
    listProjectFiles: vi.fn().mockResolvedValue([]),
    listOutputs: vi.fn().mockResolvedValue({}),
    getProjectStatus: vi.fn().mockResolvedValue(null),
    getConvergence: vi.fn().mockResolvedValue(null),
    runExtract: vi.fn().mockResolvedValue({}),
    listFlows: vi.fn().mockResolvedValue([]),
    copyFileToProject: (...args: unknown[]) => copyFileToProject(...args),
  },
}));

import { CollectionsPanel } from "../components/CollectionsPanel";
import { ErrorProvider } from "../components/ErrorBanner";
import type { KapiProject } from "../types/api";

const project: KapiProject = {
  version: "v1",
  name: "Demo",
  defaults: { source_language: "en-US" },
  collections: [{ name: "Docs", content: [{ path: "docs/**/*.md" }] }],
};

/** Resolve the handler the panel registered for a Wails event. */
async function handlerFor(name: string): Promise<(e: { data: unknown }) => void> {
  const on = vi.mocked(Events.On);
  return await waitFor(() => {
    const call = on.mock.calls.find((c) => c[0] === name);
    if (!call) throw new Error(`no subscription for ${name}`);
    return call[1] as (e: { data: unknown }) => void;
  });
}

describe("CollectionsPanel file drop", () => {
  beforeEach(() => {
    copyFileToProject.mockReset();
    copyFileToProject.mockResolvedValue("docs/dropped.md");
    vi.mocked(Events.On).mockClear();
  });

  it("copies the paths the window reports for a native file drop", async () => {
    render(
      <ErrorProvider>
        <CollectionsPanel project={project} onUpdate={vi.fn()} tabID="t1" />
      </ErrorProvider>,
    );

    const onDropped = await handlerFor("files-dropped");
    onDropped({ data: ["/tmp/one.md", "/tmp/two.md"] });

    await waitFor(() => expect(copyFileToProject).toHaveBeenCalledTimes(2));
    expect(copyFileToProject).toHaveBeenNthCalledWith(1, "t1", "/tmp/one.md", "");
    expect(copyFileToProject).toHaveBeenNthCalledWith(2, "t1", "/tmp/two.md", "");
  });

  it("ignores a drop payload that carries no paths", async () => {
    render(
      <ErrorProvider>
        <CollectionsPanel project={project} onUpdate={vi.fn()} tabID="t1" />
      </ErrorProvider>,
    );

    const onDropped = await handlerFor("files-dropped");
    onDropped({ data: null });
    onDropped({ data: [] });

    await new Promise((r) => setTimeout(r, 0));
    expect(copyFileToProject).not.toHaveBeenCalled();
  });
});
