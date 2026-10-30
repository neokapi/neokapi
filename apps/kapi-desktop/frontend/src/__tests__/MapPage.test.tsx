import { act, render, screen, within, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { DEEPER_LOOK_RESULT_MS, MapPage, MapView, decideRowKey } from "../components/MapPage";
import { api } from "../hooks/useApi";
import {
  MAP_ALL_KEPT,
  MAP_EMPTY,
  MAP_PARTLY_READ,
  MAP_PENDING,
  MAP_SOURCE_ONLY_UNCHECKED,
  MAP_SUGGESTED_ONLY,
} from "../stories/fixtures/mapSummary";
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

function renderView(summary: MapSummary, over: Partial<Parameters<typeof MapView>[0]> = {}) {
  const props = {
    projectName: "Fernwell",
    summary,
    onRun: vi.fn(),
    onDecide: vi.fn(),
    onNavigate: vi.fn(),
    onOpenFile: vi.fn(),
    onAddFiles: vi.fn(),
    ...over,
  };
  render(<MapView {...props} />);
  return props;
}

/** The counts as assistive technology reads them, in order. */
const counts = () =>
  Array.from(document.querySelectorAll('[data-slot="map-counts"] [data-slot="stat"]')).map(
    (li) => li.querySelector(".sr-only")?.textContent,
  );

describe("the Map", () => {
  it("shows the four counts, with findings the only one in the warning ink", () => {
    renderView(MAP_PENDING);
    expect(counts()).toEqual(["4 files", "251 blocks", "23 rules", "4 findings"]);
    const tones = Array.from(document.querySelectorAll('[data-slot="stat"]')).map((li) =>
      li.getAttribute("data-tone"),
    );
    expect(tones).toEqual(["default", "default", "default", "warning"]);
  });

  it("draws no findings in the warning ink when there are none, and says when none ran", () => {
    renderView(MAP_ALL_KEPT);
    expect(document.querySelector('[data-slot="stat"][data-tone="warning"]')).toBeNull();
    renderView(MAP_SOURCE_ONLY_UNCHECKED);
    const unchecked = document.querySelectorAll('[data-slot="map-counts"]')[1];
    expect(unchecked.querySelector('[data-tone="muted"]')).not.toBeNull();
  });

  it("names when kapi read the files, and no count the files count already gives", () => {
    renderView(MAP_PENDING);
    const read = document.querySelector('[data-slot="map-read"]')?.textContent ?? "";
    expect(read).toMatch(/^Read /);
    expect(read).not.toMatch(/\d+ files?/);
  });

  it("names how many files kapi read when that differs from the files count", () => {
    renderView(MAP_PARTLY_READ);
    expect(document.querySelector('[data-slot="map-read"]')?.textContent).toMatch(/^Read 3 files /);
  });

  it("leaves the line out before the first read when the counts agree", () => {
    renderView({ ...MAP_ALL_KEPT, read_at: undefined });
    expect(document.querySelector('[data-slot="map-read"]')).toBeNull();
  });

  it("lists each suggestion with its kind mark and evidence, and Decide opens that row", async () => {
    const { onDecide } = renderView(MAP_PENDING);
    const rows = document.querySelectorAll('[data-slot="map-suggestion"]');
    expect(rows).toHaveLength(MAP_PENDING.suggestions.length);
    const kinds = Array.from(rows).map((r) =>
      r.querySelector('[data-slot="kind-icon"]')?.getAttribute("data-kind"),
    );
    expect(kinds).toEqual(["conflict", "name", "name", "word", "note"]);
    expect(rows[1].textContent).toContain("1 block · 1 file");

    await userEvent.click(within(rows[1] as HTMLElement).getByRole("button", { name: "Decide" }));
    expect(onDecide).toHaveBeenCalledWith(decideRowKey(MAP_PENDING.suggestions[1]));
    expect(decideRowKey(MAP_PENDING.suggestions[0])).toBe(
      `conflict:${MAP_PENDING.suggestions[0].item.id}`,
    );
  });

  it("shows kapi suggests only when disagreements make it differ from Decide", () => {
    renderView(MAP_SUGGESTED_ONLY);
    expect(screen.getByRole("button", { name: "Decide 4" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /kapi suggests/ })).not.toBeInTheDocument();
  });

  it("offers Decide with the count awaiting, and kapi suggests beside the counts", async () => {
    const { onDecide } = renderView(MAP_PENDING);
    await userEvent.click(screen.getByRole("button", { name: "Decide 5" }));
    expect(onDecide).toHaveBeenLastCalledWith();
    await userEvent.click(screen.getByRole("button", { name: "kapi suggests 4" }));
    expect(onDecide).toHaveBeenCalledTimes(2);
  });

  it("sends each count to the place that holds what it counts", async () => {
    const { onNavigate } = renderView(MAP_PENDING);
    await userEvent.click(screen.getByRole("button", { name: "4 findings" }));
    expect(onNavigate).toHaveBeenCalledWith("checks");
    await userEvent.click(screen.getByRole("button", { name: "23 rules" }));
    expect(onNavigate).toHaveBeenCalledWith("context:atlas");
    await userEvent.click(screen.getByRole("button", { name: "251 blocks" }));
    expect(onNavigate).toHaveBeenCalledWith("context:explorer");
  });

  it("lists the files with their format, blocks and findings, and Open opens one", async () => {
    const { onOpenFile } = renderView(MAP_PENDING);
    const rows = document.querySelectorAll('[data-slot="map-file"]');
    expect(Array.from(rows).map((r) => r.getAttribute("data-path"))).toEqual(
      MAP_PENDING.file_rows.map((f) => f.relative),
    );
    expect(rows[0].textContent).toContain("markdown");
    expect(rows[0].textContent).toContain("billing.md");
    expect(rows[0].textContent).toContain("42 blocks");
    expect(rows[0].textContent).toContain("3 findings");
    expect(rows[2].querySelector('[data-slot="map-file-findings"]')).toBeNull();

    await userEvent.click(within(rows[1] as HTMLElement).getByRole("button", { name: "Open" }));
    expect(onOpenFile).toHaveBeenCalledWith(MAP_PENDING.file_rows[1]);
  });

  it("says in one line when nobody has suggested anything, and leads to an assistant", async () => {
    const { onNavigate } = renderView(MAP_ALL_KEPT);
    const empty = document.querySelector('[data-slot="map-suggestions-empty"]') as HTMLElement;
    expect(empty).toHaveTextContent("No suggestions yet.");
    expect(empty.textContent?.match(/Connect an assistant/g)).toHaveLength(1);
    expect(screen.queryByRole("button", { name: /^Decide/ })).not.toBeInTheDocument();
    await userEvent.click(within(empty).getByRole("button", { name: "Connect an assistant" }));
    expect(onNavigate).toHaveBeenCalledWith("context:agent");
  });

  it("offers to add files to a project with none, and nothing to run", async () => {
    const { onAddFiles } = renderView(MAP_EMPTY);
    const empty = document.querySelector('[data-slot="map-files-empty"]') as HTMLElement;
    expect(empty).toHaveTextContent("No content files yet");
    await userEvent.click(within(empty).getByRole("button", { name: "Add files" }));
    expect(onAddFiles).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Run on everything" })).toBeDisabled();
  });

  it("leads a project waiting to be set up to the page that sets it up", async () => {
    const { onNavigate } = renderView(MAP_EMPTY, { setupPending: true });
    const empty = document.querySelector('[data-slot="map-files-empty"]') as HTMLElement;
    expect(within(empty).queryByRole("button", { name: "Add files" })).not.toBeInTheDocument();
    await userEvent.click(within(empty).getByRole("button", { name: "Set up the project" }));
    expect(onNavigate).toHaveBeenCalledWith("project-home");
  });

  it("runs the checks on everything and shows a quiet progress state", async () => {
    const { onRun } = renderView(MAP_PENDING);
    await userEvent.click(screen.getByRole("button", { name: "Run on everything" }));
    expect(onRun).toHaveBeenCalled();
    renderView(MAP_PENDING, { running: true });
    expect(screen.getByRole("button", { name: "Checking" })).toBeDisabled();
    expect(document.body.textContent).not.toMatch(/extract/i);
  });
});

describe("MapPage", () => {
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
    renderView(MAP_PENDING, { deeperLook: { model: "demo-stub" }, onDeeperLook: vi.fn() });
    const row = document.querySelector('[data-slot="map-deeper-look"]') as HTMLElement;
    expect(row.closest('[data-slot="map-suggestions"]')).not.toBeNull();
    expect(within(row).getByRole("button", { name: "Deeper look with AI" })).toBeEnabled();
    expect(row).toHaveTextContent("Reads your files with the connected model and suggests rules.");
  });

  it("has nothing to read in a project with no files", () => {
    renderView(MAP_EMPTY, { deeperLook: { model: "demo-stub" }, onDeeperLook: vi.fn() });
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
