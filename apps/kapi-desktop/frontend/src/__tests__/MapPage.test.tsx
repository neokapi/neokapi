import { act, fireEvent, render, screen, within, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { MapView } from "@neokapi/project-screens";
import {
  MAP_EMPTY,
  MAP_PENDING,
  MAP_SOURCE_ONLY_UNCHECKED,
} from "@neokapi/project-screens/fixtures";
import {
  DEEPER_LOOK_RESULT_MS,
  DeeperLookRow,
  MapPage,
  type DeeperLookState,
} from "../components/MapPage";
import { api } from "../hooks/useApi";
import type { DeeperLookResult, MapSummary } from "../types/api";

// The preview inspects the file through the backend, which no test runtime
// provides; the Map's job is to open it on the right file.
vi.mock("../components/FilePreview", () => ({
  FilePreview: ({ filePath, filename }: { filePath: string | null; filename: string }) =>
    filePath ? <div data-testid="file-preview">{filename}</div> : null,
}));

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

const LOOKED: DeeperLookResult = {
  provider: "demo",
  model: "demo-stub",
  filed: 3,
  asked: 251,
  blocks: 251,
  tokens: 1200,
};

/** The Map as drawn, with the deeper look's row in a given state. */
function renderView(summary: MapSummary, over: { deeperLook: DeeperLookState }) {
  render(
    <MapView
      projectName="Fernwell"
      summary={summary}
      navigate={vi.fn()}
      deeperLook={
        <DeeperLookRow state={over.deeperLook} disabled={summary.files === 0} onRun={vi.fn()} />
      }
    />,
  );
}

describe("MapPage", () => {
  it("adds files through the backend from the empty files list", async () => {
    const add = vi.spyOn(api, "addFilesDialog").mockResolvedValue([]);
    render(
      <MapPage
        tabID="t-empty"
        projectName="Untitled"
        summary={MAP_EMPTY}
        onDecide={vi.fn()}
        onNavigate={vi.fn()}
      />,
    );
    const empty = document.querySelector('[data-slot="map-files-empty"]') as HTMLElement;
    await userEvent.click(within(empty).getByRole("button", { name: "Add files" }));
    expect(add).toHaveBeenCalledWith("t-empty", "");
  });

  it("maps each place the Map leads to onto the tab's views", async () => {
    const onDecide = vi.fn();
    const onNavigate = vi.fn();
    const onOpenReader = vi.fn();
    render(
      <MapPage
        tabID="t-nav"
        projectName="Fernwell"
        summary={MAP_PENDING}
        onDecide={onDecide}
        onNavigate={onNavigate}
        onOpenReader={onOpenReader}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "4 findings" }));
    expect(onNavigate).toHaveBeenCalledWith("checks");
    await userEvent.click(screen.getByRole("button", { name: "23 rules" }));
    expect(onNavigate).toHaveBeenCalledWith("context:atlas");
    await userEvent.click(screen.getByRole("button", { name: "251 blocks" }));
    expect(onNavigate).toHaveBeenCalledWith("context:explorer");
    await userEvent.click(screen.getByRole("button", { name: "Decide 5" }));
    expect(onDecide).toHaveBeenLastCalledWith(undefined);
    const row = document.querySelector('[data-path="docs/billing.md"]') as HTMLElement;
    await userEvent.click(within(row).getByRole("button", { name: "Open" }));
    expect(onOpenReader).toHaveBeenCalledWith("docs/billing.md");
  });

  it("opens a file's preview from its row", async () => {
    render(
      <MapPage
        tabID="t1"
        projectName="Fernwell"
        summary={MAP_PENDING}
        onDecide={vi.fn()}
        onNavigate={vi.fn()}
      />,
    );
    const row = document.querySelector('[data-path="docs/account.md"]') as HTMLElement;
    await userEvent.click(within(row).getByRole("button", { name: "Open" }));
    expect(screen.getByTestId("file-preview")).toHaveTextContent("docs/account.md");
  });

  it("checks every file once for a tab with no findings yet, in the recipe's languages", async () => {
    vi.spyOn(api, "mapSummary").mockResolvedValue(MAP_SOURCE_ONLY_UNCHECKED);
    const run = vi.spyOn(api, "runChecks").mockResolvedValue(null);
    render(
      <MapPage tabID="t-auto" projectName="Fernwell" onDecide={vi.fn()} onNavigate={vi.fn()} />,
    );
    await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
    expect(run).toHaveBeenCalledWith("t-auto", { id: "", name: "", languages: [] });
  });

  it("runs the checks in the recipe's target languages", async () => {
    vi.spyOn(api, "mapSummary").mockResolvedValue(MAP_PENDING);
    const run = vi.spyOn(api, "runChecks").mockResolvedValue(null);
    render(
      <MapPage tabID="t-run" projectName="Fernwell" onDecide={vi.fn()} onNavigate={vi.fn()} />,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Run on everything" }));
    expect(run).toHaveBeenCalledWith("t-run", { id: "", name: "", languages: ["de", "nb"] });
  });
});

describe("the deeper look on the Map", () => {
  it("offers one button with one line under What your files do", () => {
    renderView(MAP_PENDING, { deeperLook: { model: "demo-stub" } });
    const row = document.querySelector('[data-slot="map-deeper-look"]') as HTMLElement;
    expect(row.closest('[data-slot="map-suggestions"]')).not.toBeNull();
    expect(within(row).getByRole("button", { name: "Deeper look with AI" })).toBeEnabled();
    expect(row).toHaveTextContent("Reads your files with the connected model and suggests rules.");
  });

  it("has nothing to read in a project with no files", () => {
    renderView(MAP_EMPTY, { deeperLook: { model: "demo-stub" } });
    expect(screen.getByRole("button", { name: "Deeper look with AI" })).toBeDisabled();
  });

  it("shows the quiet progress state with the model's name while it runs", () => {
    renderView(MAP_PENDING, { deeperLook: { model: "demo-stub", running: true } });
    expect(screen.getByRole("button", { name: "Reading with demo-stub" })).toBeDisabled();
    expect(document.querySelector('[data-slot="map-deeper-look-line"]')).toBeNull();
  });

  it("says what the run filed, and that there was nothing more", () => {
    renderView(MAP_PENDING, { deeperLook: { model: "demo-stub", result: LOOKED } });
    expect(document.querySelector('[data-slot="map-deeper-look-line"]')).toHaveTextContent(
      "3 new suggestions from demo-stub",
    );
    renderView(MAP_PENDING, {
      deeperLook: { model: "demo-stub", result: { ...LOOKED, filed: 0 } },
    });
    const lines = document.querySelectorAll('[data-slot="map-deeper-look-line"]');
    expect(lines[1]).toHaveTextContent("Nothing more to suggest");
  });

  it("opens the Connect your AI card when no model is connected, and runs nothing", async () => {
    vi.spyOn(api, "detectAIProviders").mockResolvedValue({ detected: [], configured: false });
    const look = vi.spyOn(api, "deeperLook").mockResolvedValue(LOOKED);
    render(
      <MapPage
        tabID="t-nomodel"
        projectName="Fernwell"
        summary={MAP_PENDING}
        deeperLookModel={{ connected: false }}
        onDecide={vi.fn()}
        onNavigate={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Deeper look with AI" }));
    expect(await screen.findByText("Connect your AI")).toBeInTheDocument();
    expect(screen.getByTestId("connect-ai-card")).toHaveTextContent(
      "Pick the model kapi reads your files with.",
    );
    expect(look).not.toHaveBeenCalled();
  });

  it("runs with the connected model, reads the Map again, and lets the result line go", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const summary = vi.spyOn(api, "mapSummary").mockResolvedValue(MAP_PENDING);
    vi.spyOn(api, "runChecks").mockResolvedValue(null);
    const look = vi.spyOn(api, "deeperLook").mockResolvedValue(LOOKED);
    render(
      <MapPage
        tabID="t-look"
        projectName="Fernwell"
        deeperLookModel={{ connected: true, provider: "demo", model: "demo-stub" }}
        onDecide={vi.fn()}
        onNavigate={vi.fn()}
      />,
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(await screen.findByRole("button", { name: "Deeper look with AI" }));
    expect(look).toHaveBeenCalledWith("t-look");
    expect(await screen.findByText("3 new suggestions from demo-stub")).toBeInTheDocument();
    const reads = summary.mock.calls.length;
    expect(reads).toBeGreaterThan(1);

    act(() => {
      vi.advanceTimersByTime(DEEPER_LOOK_RESULT_MS);
    });
    await waitFor(() =>
      expect(screen.queryByText("3 new suggestions from demo-stub")).not.toBeInTheDocument(),
    );
    expect(
      screen.getByText("Reads your files with the connected model and suggests rules."),
    ).toBeInTheDocument();
  });

  it("waits, and runs nothing, until it knows whether a model is connected", async () => {
    vi.spyOn(api, "deeperLookModel").mockReturnValue(new Promise(() => {}));
    const look = vi.spyOn(api, "deeperLook").mockResolvedValue(LOOKED);
    render(
      <MapPage
        tabID="t-pending"
        projectName="Fernwell"
        summary={MAP_PENDING}
        onDecide={vi.fn()}
        onNavigate={vi.fn()}
      />,
    );
    const button = screen.getByRole("button", { name: "Deeper look with AI" });
    expect(button).toBeDisabled();
    fireEvent.click(button);
    expect(look).not.toHaveBeenCalled();
    expect(screen.queryByText("Connect your AI")).not.toBeInTheDocument();
  });

  it("shows why the run failed", async () => {
    vi.spyOn(api, "deeperLook").mockRejectedValue(new Error("anthropic: API error 401"));
    render(
      <MapPage
        tabID="t-fail"
        projectName="Fernwell"
        summary={MAP_PENDING}
        deeperLookModel={{ connected: true, provider: "anthropic", model: "claude-sonnet-4-5" }}
        onDecide={vi.fn()}
        onNavigate={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Deeper look with AI" }));
    expect(await screen.findByText("The deeper look did not run")).toBeInTheDocument();
  });
});
