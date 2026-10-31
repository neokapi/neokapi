import { describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";
import type { ProjectScreensPort } from "@neokapi/project-screens";
import {
  DECIDE_VIEW,
  KEPT_RULES,
  MAP_PENDING,
  READER_WITH_FIXES,
} from "@neokapi/project-screens/fixtures";

import { SharePanel } from "../components/SharePanel";
import { localPreviewPort, previewOpens } from "../lib/previewPort";
import { api } from "../hooks/useApi";
import { LOCAL_VIEW } from "../stories/fixtures/sharing";

/** The Fernwell fixtures through the preview's four reads. */
function fernwellPort(): ProjectScreensPort {
  return {
    mapSummary: vi.fn(async () => MAP_PENDING),
    readerFile: vi.fn(async () => READER_WITH_FIXES),
    decideView: vi.fn(async () => DECIDE_VIEW),
    keptRules: vi.fn(async () => KEPT_RULES),
  };
}

/** Open the Share page's Preview and answer with the sheet. */
async function openPreview(port = fernwellPort()) {
  render(<SharePanel tabID="t1" projectName="Fernwell" view={LOCAL_VIEW} previewPort={port} />);
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  const sheet = await screen.findByRole("dialog");
  return { sheet, port };
}

/** The controls that change the project, none of which a preview draws. */
function expectNoAction(sheet: HTMLElement) {
  const q = within(sheet);
  expect(q.queryByRole("button", { name: /^Keep/ })).toBeNull();
  expect(q.queryByRole("button", { name: /^Not a rule/ })).toBeNull();
  expect(q.queryByRole("button", { name: /^Later/ })).toBeNull();
  expect(q.queryByRole("button", { name: /^Fix/ })).toBeNull();
  expect(q.queryByRole("button", { name: "Run on everything" })).toBeNull();
}

describe("Bowrain preview", () => {
  it("opens on the Map, titled with the project, with no action", async () => {
    const { sheet } = await openPreview();
    expect(within(sheet).getByText("Fernwell as Bowrain shows it")).toBeInTheDocument();
    expect(within(sheet).getByRole("tab", { name: "Map" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await within(sheet).findByRole("heading", { name: "Fernwell" });
    expectNoAction(sheet);
  });

  it("opens a file in the Reader inside the sheet, and Back returns to the Map", async () => {
    const { sheet, port } = await openPreview();
    await within(sheet).findByRole("heading", { name: "Fernwell" });
    const open = within(sheet).getAllByRole("button", { name: "Open" });
    await userEvent.click(open[0]);
    await within(sheet).findByRole("heading", { name: READER_WITH_FIXES.path });
    expect(port.readerFile).toHaveBeenCalled();
    expectNoAction(sheet);
    await userEvent.click(within(sheet).getByRole("button", { name: /Back/ }));
    await within(sheet).findByRole("heading", { name: "Fernwell" });
  });

  it("draws Rules with the questions and Kept by, and no Keep", async () => {
    const { sheet } = await openPreview();
    await userEvent.click(within(sheet).getByRole("tab", { name: "Rules" }));
    await within(sheet).findByRole("listbox", { name: "Suggestions to decide" });
    const kept = await within(sheet).findByRole("region", { name: "Kept by" });
    const rows = within(kept).getAllByRole("listitem");
    expect(rows.map((r) => r.querySelector("[data-slot='kept-by']")?.textContent)).toEqual(
      KEPT_RULES.map(() => expect.any(String)),
    );
    expect(rows[0]).toHaveTextContent(KEPT_RULES[0].rule);
    expect(rows[0].querySelector("[data-slot='kept-by']")).toHaveTextContent(/you$/);
    expect(rows[1].querySelector("[data-slot='kept-by']")).toHaveTextContent(/ada$/);
    expect(rows[2].querySelector("[data-slot='kept-by']")).toHaveTextContent(/kapi$/);
    expectNoAction(sheet);
  });

  it("opens Rules from the Map's Decide", async () => {
    const { sheet } = await openPreview();
    await userEvent.click(
      await within(sheet).findByRole("button", { name: `Decide ${MAP_PENDING.awaiting}` }),
    );
    await waitFor(() =>
      expect(within(sheet).getByRole("tab", { name: "Rules" })).toHaveAttribute(
        "aria-selected",
        "true",
      ),
    );
  });

  it("draws no Preview on a tab with no checkout", () => {
    render(<SharePanel tabID="t1" view={LOCAL_VIEW} contextOnly previewPort={fernwellPort()} />);
    expect(screen.queryByRole("button", { name: "Preview" })).toBeNull();
  });
});

describe("the local preview port", () => {
  it("reads through the tab's bindings and carries no action", async () => {
    const mapSummary = vi.spyOn(api, "mapSummary").mockResolvedValue(MAP_PENDING);
    const keptRules = vi.spyOn(api, "keptRules").mockResolvedValue(KEPT_RULES);
    const port = localPreviewPort("t9");
    await expect(port.mapSummary()).resolves.toBe(MAP_PENDING);
    await expect(port.keptRules?.()).resolves.toBe(KEPT_RULES);
    expect(mapSummary).toHaveBeenCalledWith("t9");
    expect(keptRules).toHaveBeenCalledWith("t9");
    expect(Object.keys(port).sort()).toEqual([
      "decideView",
      "keptRules",
      "mapSummary",
      "readerFile",
    ]);
    vi.restoreAllMocks();
  });

  it("rejects a read the backend does not answer", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(null);
    await expect(localPreviewPort("t9").decideView()).rejects.toThrow();
    vi.restoreAllMocks();
  });

  it("opens the Map, the Reader and Rules, and nothing else", () => {
    expect(previewOpens({ kind: "map" })).toBe(true);
    expect(previewOpens({ kind: "reader", path: "a.md" })).toBe(true);
    expect(previewOpens({ kind: "rules" })).toBe(true);
    expect(previewOpens({ kind: "kept" })).toBe(true);
    expect(previewOpens({ kind: "checks" })).toBe(false);
    expect(previewOpens({ kind: "blocks" })).toBe(false);
    expect(previewOpens({ kind: "setup" })).toBe(false);
    expect(previewOpens({ kind: "connect-assistant" })).toBe(false);
  });
});
