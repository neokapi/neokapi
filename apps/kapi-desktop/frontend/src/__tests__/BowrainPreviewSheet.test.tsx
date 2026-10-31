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
import {
  PreviewRefusalError,
  localPreviewPort,
  previewOpens,
  previewSource,
  remotePreviewPort,
} from "../lib/previewPort";
import { api } from "../hooks/useApi";
import type { BowrainPreviewAnswer, BowrainPreviewPart } from "../types/api";
import { BOWRAIN_SIGNED_IN_VIEW, BOWRAIN_VIEW, LOCAL_VIEW } from "../stories/fixtures/sharing";

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
    expect(within(sheet).getByText("As this Mac shows it")).toBeInTheDocument();
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

  it("previews this Mac unless the recipe names a server a sign-in names", () => {
    expect(previewSource(LOCAL_VIEW)).toBe("local");
    expect(previewSource(BOWRAIN_VIEW)).toBe("local");
    expect(previewSource(BOWRAIN_SIGNED_IN_VIEW)).toBe("bowrain");
    expect(previewSource({ ...LOCAL_VIEW, signed_in: true })).toBe("local");
    expect(previewSource(null)).toBe("local");
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

/** The server's answers to the four reads, the Fernwell fixtures. */
function serverAnswers(part: BowrainPreviewPart): BowrainPreviewAnswer {
  switch (part) {
    case "map":
      return { map: MAP_PENDING };
    case "reader":
      return { reader: READER_WITH_FIXES };
    case "decide":
      return { decide: DECIDE_VIEW };
    case "kept":
      return { kept: KEPT_RULES };
  }
}

describe("the remote preview port", () => {
  it("reads through BowrainPreview, opening a file by the item name the Map carries", async () => {
    const preview = vi
      .spyOn(api, "bowrainPreview")
      .mockImplementation(async (_tab, part) => serverAnswers(part));
    const port = remotePreviewPort("t9", "Mira Holt");
    await expect(port.mapSummary()).resolves.toBe(MAP_PENDING);
    await expect(port.readerFile("docs/billing.md")).resolves.toBe(READER_WITH_FIXES);
    expect(preview).toHaveBeenCalledWith("t9", "reader", "/work/fernwell/docs/billing.md");
    await expect(port.readerFile("docs/other.md")).resolves.toBe(READER_WITH_FIXES);
    expect(preview).toHaveBeenLastCalledWith("t9", "reader", "docs/other.md");
    await expect(port.decideView()).resolves.toBe(DECIDE_VIEW);
    await expect(port.keptRules?.()).resolves.toBe(KEPT_RULES);
    expect(Object.keys(port).sort()).toEqual([
      "actorName",
      "decideView",
      "keptRules",
      "mapSummary",
      "readerFile",
    ]);
    vi.restoreAllMocks();
  });

  it("shows the server's member names, and the signed-in name for a person it leaves unnamed", () => {
    const port = remotePreviewPort("t9", "Mira Holt");
    expect(port.actorName?.({ kind: "person", name: "ada" })).toBe("ada");
    expect(port.actorName?.({ kind: "person" })).toBe("Mira Holt");
    expect(port.actorName?.({ kind: "agent", name: "claude-code" })).toBe("claude-code");
    // BOWRAIN_AUTH_TOKEN stores no name: the screens then draw "you".
    expect(remotePreviewPort("t9").actorName?.({ kind: "person" })).toBeUndefined();
  });

  it("rejects a refused read with the refusal as it came", async () => {
    vi.spyOn(api, "bowrainPreview").mockResolvedValue({
      refusal: { status: 403, message: "insufficient project permissions", action: "open" },
    });
    const read = remotePreviewPort("t9").mapSummary();
    await expect(read).rejects.toBeInstanceOf(PreviewRefusalError);
    await expect(read).rejects.toMatchObject({
      refusal: { status: 403, message: "insufficient project permissions", action: "open" },
    });
    vi.restoreAllMocks();
  });
});

describe("the connected preview", () => {
  /** Open Preview on the connected Share page, with no port given. */
  async function openConnected(view = BOWRAIN_SIGNED_IN_VIEW) {
    render(<SharePanel tabID="t1" projectName="Fernwell" view={view} />);
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    return screen.findByRole("dialog");
  }

  it("reads the server, says so, and names who kept each rule", async () => {
    const preview = vi
      .spyOn(api, "bowrainPreview")
      .mockImplementation(async (_tab, part) => serverAnswers(part));
    const local = vi.spyOn(api, "mapSummary");
    const sheet = await openConnected();
    expect(within(sheet).getByText("As Bowrain shows it")).toBeInTheDocument();
    await within(sheet).findByRole("heading", { name: "Fernwell" });
    expect(preview).toHaveBeenCalledWith("t1", "map", "");
    expect(local).not.toHaveBeenCalled();
    await userEvent.click(within(sheet).getByRole("tab", { name: "Rules" }));
    const kept = await within(sheet).findByRole("region", { name: "Kept by" });
    const rows = within(kept).getAllByRole("listitem");
    expect(rows[0].querySelector("[data-slot='kept-by']")).toHaveTextContent(/Mira Holt$/);
    expect(rows[1].querySelector("[data-slot='kept-by']")).toHaveTextContent(/ada$/);
    expect(within(kept).queryByText(/you$/)).toBeNull();
    expectNoAction(sheet);
    vi.restoreAllMocks();
  });

  it("draws an unnamed keeper as you when the sign-in stores no name", async () => {
    vi.spyOn(api, "bowrainPreview").mockImplementation(async (_tab, part) => serverAnswers(part));
    const sheet = await openConnected({ ...BOWRAIN_SIGNED_IN_VIEW, signed_in_as: undefined });
    await userEvent.click(within(sheet).getByRole("tab", { name: "Rules" }));
    const kept = await within(sheet).findByRole("region", { name: "Kept by" });
    const rows = within(kept).getAllByRole("listitem");
    expect(rows[0].querySelector("[data-slot='kept-by']")).toHaveTextContent(/you$/);
    expect(rows[1].querySelector("[data-slot='kept-by']")).toHaveTextContent(/ada$/);
    vi.restoreAllMocks();
  });

  it("draws a permission refusal in the body, in the server's words, with Open Bowrain", async () => {
    vi.spyOn(api, "bowrainPreview").mockResolvedValue({
      refusal: { status: 403, message: "insufficient project permissions", action: "open" },
    });
    const sheet = await openConnected();
    const body = await within(sheet).findByRole("alert");
    expect(body).toHaveTextContent("insufficient project permissions");
    expect(body).toHaveTextContent("HTTP 403");
    expect(within(body).getByRole("link", { name: /Open Bowrain/ })).toHaveAttribute(
      "href",
      "https://app.bowrain.cloud/acme/help",
    );
    expect(within(sheet).queryByRole("tab", { name: "Map" })).toBeNull();
    vi.restoreAllMocks();
  });

  it("draws a sign-in the server does not take with the command that signs in", async () => {
    vi.spyOn(api, "bowrainPreview").mockResolvedValue({
      refusal: { status: 401, message: "invalid token", action: "sign_in" },
    });
    const sheet = await openConnected();
    const body = await within(sheet).findByRole("alert");
    expect(body).toHaveTextContent("invalid token");
    expect(body).toHaveTextContent("Sign in");
    expect(body.querySelector("[data-slot='bowrain-preview-sign-in']")).toHaveTextContent(
      "kapi auth login --server https://app.bowrain.cloud",
    );
    expect(within(body).queryByRole("link", { name: /Open Bowrain/ })).toBeNull();
    vi.restoreAllMocks();
  });

  it("names the server when this Mac holds no sign-in for it", async () => {
    vi.spyOn(api, "bowrainPreview").mockResolvedValue({
      refusal: { status: 0, action: "sign_in" },
    });
    const sheet = await openConnected();
    const body = await within(sheet).findByRole("alert");
    expect(body).toHaveTextContent("Not signed in to app.bowrain.cloud");
    expect(body).not.toHaveTextContent("HTTP");
    vi.restoreAllMocks();
  });
});
