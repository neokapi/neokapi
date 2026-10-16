import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";

vi.mock("../hooks/useApi", () => ({
  api: {
    listPlugins: vi.fn().mockResolvedValue([]),
    getKnownLocales: vi.fn().mockResolvedValue([]),
  },
}));

import { ProjectSettingsPage } from "../components/ProjectSettingsPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { KapiProject, PluginInfo } from "../types/api";
import type { RecipeGovernance } from "../components/GovernanceSettings";

const installed: PluginInfo[] = [
  { name: "okapi-bridge", id: "okapi-bridge", version: "1.48.0", type: "format" },
  { name: "kapi-av", id: "kapi-av", version: "0.3.0", type: "format" },
];

const governance: RecipeGovernance = {
  axes: [],
  channels: [],
  profiles: [],
  voice_profiles: [],
  packs: [],
};

function projectWith(overrides: Partial<KapiProject> = {}): KapiProject {
  return {
    version: "v1",
    name: "Demo",
    defaults: { source_language: "en-US" },
    plugins: { "okapi-bridge": {} },
    collections: [],
    ...overrides,
  };
}

function renderPage(project: KapiProject, onUpdate = vi.fn()) {
  render(
    <ErrorProvider>
      <ProjectSettingsPage
        tabID="t1"
        project={project}
        onUpdate={onUpdate}
        installedPlugins={installed}
        governance={governance}
      />
    </ErrorProvider>,
  );
  return onUpdate;
}

describe("ProjectSettingsPage format engine", () => {
  it("offers native and the declared plugins, and writes defaults.engine", async () => {
    const onUpdate = renderPage(projectWith());
    const select = await screen.findByRole("combobox", { name: "Format engine" });
    expect(select).toHaveTextContent("native (default)");

    await userEvent.click(select);
    const options = await screen.findAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["native (default)", "okapi-bridge"]);
    await userEvent.click(screen.getByRole("option", { name: "okapi-bridge" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    const updated = onUpdate.mock.calls.at(-1)![0] as KapiProject;
    expect(updated.defaults?.engine).toBe("okapi-bridge");
  });

  it("clears the engine when native is chosen again", async () => {
    const onUpdate = renderPage(
      projectWith({ defaults: { source_language: "en-US", engine: "okapi-bridge" } }),
    );
    const select = await screen.findByRole("combobox", { name: "Format engine" });
    expect(select).toHaveTextContent("okapi-bridge");
    await userEvent.click(select);
    await userEvent.click(await screen.findByRole("option", { name: "native (default)" }));
    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    const updated = onUpdate.mock.calls.at(-1)![0] as KapiProject;
    expect(updated.defaults?.engine).toBeUndefined();
  });

  it("drops the engine with the plugin that provided it", async () => {
    const onUpdate = renderPage(
      projectWith({
        defaults: {
          source_language: "en-US",
          engine: "okapi-bridge",
          formats: { ".json": { engine: "okapi-bridge", priority: 3 } },
        },
      }),
    );
    const boxes = await screen.findAllByRole("checkbox");
    const bridge = boxes.find((b) => b.closest("label")?.textContent?.includes("okapi-bridge"));
    expect(bridge).toBeDefined();
    await userEvent.click(bridge!);
    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    const updated = onUpdate.mock.calls.at(-1)![0] as KapiProject;
    expect(updated.plugins).toBeUndefined();
    expect(updated.defaults?.engine).toBeUndefined();
    expect(updated.defaults?.formats?.[".json"]).toEqual({ priority: 3, engine: undefined });
  });

  it("offers no engine without a declared plugin", async () => {
    renderPage(projectWith({ plugins: undefined }));
    await screen.findByText("Plugins");
    expect(screen.queryByRole("combobox", { name: "Format engine" })).not.toBeInTheDocument();
  });

  it("lets a format default name its engine", async () => {
    const onUpdate = renderPage(
      projectWith({
        defaults: { source_language: "en-US", formats: { ".json": { priority: 3 } } },
      }),
    );
    const select = await screen.findByRole("combobox", { name: "Engine for .json" });
    expect(select).toHaveTextContent("Project engine");
    await userEvent.click(select);
    await userEvent.click(await screen.findByRole("option", { name: "native" }));
    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    const updated = onUpdate.mock.calls.at(-1)![0] as KapiProject;
    expect(updated.defaults?.formats?.[".json"]).toEqual({ priority: 3, engine: "native" });
  });
});
