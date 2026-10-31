// @vitest-environment jsdom
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { MapView, decideRowKey, type MapViewProps } from "../MapView";
import type { ScreenTarget } from "../port";
import {
  MAP_ALL_KEPT,
  MAP_EMPTY,
  MAP_PARTLY_READ,
  MAP_PENDING,
  MAP_SOURCE_ONLY_UNCHECKED,
  MAP_SUGGESTED_ONLY,
} from "../stories/fixtures";
import type { MapSummary } from "../types";

function renderView(summary: MapSummary, over: Partial<MapViewProps> = {}) {
  const props = {
    projectName: "Fernwell",
    summary,
    onRun: vi.fn(),
    navigate: vi.fn<(target: ScreenTarget) => void>(),
    onPreviewFile: vi.fn(),
    addFiles: <button type="button">Add files</button>,
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
    const { navigate } = renderView(MAP_PENDING);
    const rows = document.querySelectorAll('[data-slot="map-suggestion"]');
    expect(rows).toHaveLength(MAP_PENDING.suggestions.length);
    const kinds = Array.from(rows).map((r) =>
      r.querySelector('[data-slot="kind-icon"]')?.getAttribute("data-kind"),
    );
    expect(kinds).toEqual(["conflict", "name", "name", "word", "note"]);
    expect(rows[1].textContent).toContain("1 block · 1 file");

    await userEvent.click(within(rows[1] as HTMLElement).getByRole("button", { name: "Decide" }));
    expect(navigate).toHaveBeenCalledWith({
      kind: "rules",
      row: decideRowKey(MAP_PENDING.suggestions[1]),
    });
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
    const { navigate } = renderView(MAP_PENDING);
    await userEvent.click(screen.getByRole("button", { name: "Decide 5" }));
    expect(navigate).toHaveBeenLastCalledWith({ kind: "rules" });
    await userEvent.click(screen.getByRole("button", { name: "kapi suggests 4" }));
    expect(navigate).toHaveBeenCalledTimes(2);
  });

  it("sends each count to the place that holds what it counts", async () => {
    const { navigate } = renderView(MAP_PENDING);
    await userEvent.click(screen.getByRole("button", { name: "4 findings" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "checks" });
    await userEvent.click(screen.getByRole("button", { name: "23 rules" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "kept" });
    await userEvent.click(screen.getByRole("button", { name: "251 blocks" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "blocks" });
  });

  it("lists the files with their format, blocks and findings, and Open opens one in the Reader", async () => {
    const { navigate } = renderView(MAP_PENDING);
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
    expect(navigate).toHaveBeenCalledWith({
      kind: "reader",
      path: MAP_PENDING.file_rows[1].relative,
    });
  });

  it("previews a file the block cache holds no blocks for", async () => {
    const unread: MapSummary = {
      ...MAP_PENDING,
      file_rows: MAP_PENDING.file_rows.map((f, i) => (i === 0 ? { ...f, blocks: 0 } : f)),
    };
    const { navigate, onPreviewFile } = renderView(unread);
    const row = document.querySelector('[data-slot="map-file"]') as HTMLElement;
    await userEvent.click(within(row).getByRole("button", { name: "Open" }));
    expect(onPreviewFile).toHaveBeenCalledWith(unread.file_rows[0]);
    expect(navigate).not.toHaveBeenCalled();
  });

  it("says in one line when nobody has suggested anything, and leads to an assistant", async () => {
    const { navigate } = renderView(MAP_ALL_KEPT);
    const empty = document.querySelector('[data-slot="map-suggestions-empty"]') as HTMLElement;
    expect(empty).toHaveTextContent("No suggestions yet.");
    expect(empty.textContent?.match(/Connect an assistant/g)).toHaveLength(1);
    expect(screen.queryByRole("button", { name: /^Decide/ })).not.toBeInTheDocument();
    await userEvent.click(within(empty).getByRole("button", { name: "Connect an assistant" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "connect-assistant" });
  });

  it("draws the host's add-files action in a project with none, and nothing to run", () => {
    renderView(MAP_EMPTY);
    const empty = document.querySelector('[data-slot="map-files-empty"]') as HTMLElement;
    expect(empty).toHaveTextContent("No content files yet");
    expect(within(empty).getByRole("button", { name: "Add files" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Run on everything" })).toBeDisabled();
  });

  it("leads a project waiting to be set up to the page that sets it up", async () => {
    const { navigate } = renderView(MAP_EMPTY, { setupPending: true });
    const empty = document.querySelector('[data-slot="map-files-empty"]') as HTMLElement;
    expect(within(empty).queryByRole("button", { name: "Add files" })).not.toBeInTheDocument();
    await userEvent.click(within(empty).getByRole("button", { name: "Set up the project" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "setup" });
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

describe("the Map a host acts on only in part", () => {
  it("draws no check action without onRun", () => {
    renderView(MAP_PENDING, { onRun: undefined });
    expect(screen.queryByRole("button", { name: "Run on everything" })).toBeNull();
  });

  it("hides each button whose target the host does not open", () => {
    renderView(MAP_PENDING, { canNavigate: () => false, onPreviewFile: undefined });
    expect(screen.queryByRole("button", { name: /^Decide/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /kapi suggests/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Open" })).toBeNull();
    expect(screen.queryByRole("button", { name: "23 rules" })).toBeNull();
    expect(counts()).toEqual(["4 files", "251 blocks", "23 rules", "4 findings"]);
  });

  it("previews a file when the host opens no Reader", async () => {
    const { onPreviewFile, navigate } = renderView(MAP_PENDING, {
      canNavigate: (target) => target.kind !== "reader",
    });
    const row = document.querySelector('[data-slot="map-file"]') as HTMLElement;
    await userEvent.click(within(row).getByRole("button", { name: "Open" }));
    expect(onPreviewFile).toHaveBeenCalledWith(MAP_PENDING.file_rows[0]);
    expect(navigate).not.toHaveBeenCalled();
  });

  it("leads an empty list to no assistant when the host connects none", () => {
    renderView(MAP_ALL_KEPT, { canNavigate: (target) => target.kind !== "connect-assistant" });
    const empty = document.querySelector('[data-slot="map-suggestions-empty"]') as HTMLElement;
    expect(within(empty).queryByRole("button")).toBeNull();
  });
});
