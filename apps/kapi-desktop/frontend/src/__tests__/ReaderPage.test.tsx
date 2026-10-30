import { useState } from "react";
import { render, screen, within, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { ErrorProvider } from "../components/ErrorBanner";
import { ChecksPanel } from "../components/ChecksPanel";
import { MapPage } from "../components/MapPage";
import { ReaderPage, readerTargetOf, readerView } from "../components/ReaderPage";
import { api } from "../hooks/useApi";
import { MAP_PENDING } from "../stories/fixtures/mapSummary";
import {
  READER_CLEAN,
  READER_REFUSED,
  READER_WITH_FIXES,
  READER_WITHOUT_FIXES,
} from "../stories/fixtures/readerFile";
import type { CheckRunResult, MapSummary } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

function block(id: string): HTMLElement {
  const el = document.querySelector<HTMLElement>(
    `[data-slot="reader-block"][data-block-id="${id}"]`,
  );
  if (!el) throw new Error(`no block ${id}`);
  return el;
}

describe("the Reader", () => {
  it("draws each finding on its words, its row under the block, and the rules in the margin", async () => {
    vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITH_FIXES);
    render(<ReaderPage tabID="t1" path="docs/billing.md" onBack={vi.fn()} />);

    expect(await screen.findByRole("heading", { name: "docs/billing.md" })).toBeInTheDocument();
    expect(screen.getByText("3 blocks · 3 findings")).toBeInTheDocument();

    const p2 = block("p2");
    const marks = p2.querySelectorAll('mark[data-overlay-type="finding"]');
    expect([...marks].map((m) => m.textContent)).toEqual(["Login", "utilize"]);
    expect(within(p2).getByText("Sign in, not Login")).toBeInTheDocument();
    expect(within(p2).getByText("use, not utilize")).toBeInTheDocument();
    // The placeholder is drawn as a chip, not dropped.
    expect(block("p3").textContent).toContain("Your credits reset on");
    expect(block("p3").querySelector('mark[data-overlay-type="finding"]')?.textContent).toBe(
      "absolutely delighted",
    );

    const margin = document.querySelector<HTMLElement>('[data-slot="reader-rules"]')!;
    const rules = [...margin.querySelectorAll('[data-slot="reader-rule"]')];
    expect(rules.map((r) => r.getAttribute("data-kind"))).toEqual([
      "tone",
      "word",
      "name",
      "word",
      "note",
      "word",
    ]);
    expect(within(margin).getByText("voice fernwell")).toBeInTheDocument();
    expect(within(margin).getByText("Sign in, not Login")).toBeInTheDocument();
    expect(within(margin).getByText("suggested")).toBeInTheDocument();
  });

  it("offers Fix only on a finding that has a fix", async () => {
    vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITH_FIXES);
    render(<ReaderPage tabID="t1" path="docs/billing.md" onBack={vi.fn()} />);
    await screen.findByRole("heading", { name: "docs/billing.md" });

    expect(within(block("p2")).getAllByRole("button", { name: "Fix" })).toHaveLength(2);
    expect(within(block("p3")).queryByRole("button", { name: "Fix" })).toBeNull();
    expect(within(block("p3")).getByText("Edit by hand")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Fix 2 in this file" })).toBeInTheDocument();
  });

  it("draws no Fix at all when no finding has one", async () => {
    vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITHOUT_FIXES);
    render(<ReaderPage tabID="t1" path="docs/billing.md" onBack={vi.fn()} />);
    await screen.findByRole("heading", { name: "docs/billing.md" });

    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
    expect(within(block("p3")).getAllByText("Edit by hand")).toHaveLength(2);
    expect(within(block("p3")).getByText("fr")).toBeInTheDocument();
  });

  it("applies one fix through the backend and draws the file it answers with", async () => {
    vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITH_FIXES);
    const after = {
      ...READER_WITH_FIXES,
      findings: 2,
      fixable: 1,
      blocks: READER_WITH_FIXES.blocks.map((b) =>
        b.id === "p2"
          ? {
              ...b,
              runs: [
                { text: "Sign in to see your invoices, then utilize the export to download them." },
              ],
              findings: b.findings
                .filter((f) => f.id !== "a1f0-1")
                .map((f) => ({
                  ...f,
                  position: {
                    kind: "range" as const,
                    start: { run: 0, offset: 35 },
                    end: { run: 0, offset: 42 },
                  },
                })),
            }
          : b,
      ),
    };
    const fix = vi.spyOn(api, "fixFinding").mockResolvedValue(after);
    render(<ReaderPage tabID="t1" path="docs/billing.md" onBack={vi.fn()} />);
    await screen.findByRole("heading", { name: "docs/billing.md" });

    await userEvent.click(within(block("p2")).getAllByRole("button", { name: "Fix" })[0]);
    expect(fix).toHaveBeenCalledWith("t1", "docs/billing.md", "a1f0-1");
    await waitFor(() => expect(block("p2").textContent).toContain("Sign in to see your invoices"));
    expect(within(block("p2")).queryByText("Sign in, not Login")).toBeNull();
    expect(within(block("p2")).getAllByRole("button", { name: "Fix" })).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Fix 1 in this file" })).toBeInTheDocument();
  });

  it("applies every fix in the file at once", async () => {
    vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITH_FIXES);
    const fixFile = vi.spyOn(api, "fixFile").mockResolvedValue(READER_CLEAN);
    render(<ReaderPage tabID="t1" path="docs/billing.md" onBack={vi.fn()} />);
    await screen.findByRole("heading", { name: "docs/billing.md" });

    await userEvent.click(screen.getByRole("button", { name: "Fix 2 in this file" }));
    expect(fixFile).toHaveBeenCalledWith("t1", "docs/billing.md");
    await waitFor(() => expect(screen.getByText("3 blocks · 0 findings")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
  });

  it("says why a refused fix did not apply, under its finding", async () => {
    vi.spyOn(api, "readerFile").mockResolvedValue(READER_REFUSED);
    render(<ReaderPage tabID="t1" path="docs/billing.md" onBack={vi.fn()} />);
    await screen.findByRole("heading", { name: "docs/billing.md" });

    const row = block("p2").querySelector<HTMLElement>(
      '[data-slot="reader-finding"][data-id="b2c3-1"]',
    )!;
    expect(within(row).getByRole("alert").textContent).toMatch(/^Not fixed: the edit introduces/);
  });

  it("draws a file with no findings with its blocks and the margin only", async () => {
    vi.spyOn(api, "readerFile").mockResolvedValue(READER_CLEAN);
    render(<ReaderPage tabID="t1" path="docs/billing.md" onBack={vi.fn()} />);
    await screen.findByRole("heading", { name: "docs/billing.md" });

    expect(document.querySelectorAll('[data-slot="reader-finding"]')).toHaveLength(0);
    expect(document.querySelectorAll('mark[data-overlay-type="finding"]')).toHaveLength(0);
    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
    expect(document.querySelectorAll('[data-slot="reader-rule"]')).toHaveLength(6);
  });

  it("names a file and a block in its view", () => {
    expect(readerTargetOf(readerView("docs/a:b.md", "p:2"))).toEqual({
      path: "docs/a:b.md",
      blockID: "p:2",
    });
    expect(readerTargetOf(readerView("docs/billing.md"))).toEqual({ path: "docs/billing.md" });
    expect(readerTargetOf("map")).toBeNull();
  });
});

/** The Map, the Checks page and the Reader, routed by view as ViewSwitch routes them. */
function Routed({ summary, start }: { summary: MapSummary; start: string }) {
  const [view, setView] = useState(start);
  const target = readerTargetOf(view);
  if (target) {
    return (
      <ReaderPage
        tabID="t1"
        path={target.path}
        focusBlockID={target.blockID}
        onBack={() => setView("map")}
      />
    );
  }
  if (view === "checks") {
    return (
      <ChecksPanel
        tabID="t1"
        onOpenReader={(path, blockID) => setView(readerView(path, blockID))}
      />
    );
  }
  return (
    <MapPage
      tabID="t1"
      projectName="Fernwell"
      summary={summary}
      onDecide={vi.fn()}
      onNavigate={setView}
      onOpenReader={(path) => setView(readerView(path))}
    />
  );
}

describe("opening the Reader", () => {
  it("opens from a file on the Map", async () => {
    const read = vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITH_FIXES);
    render(<Routed summary={MAP_PENDING} start="map" />);
    const row = document.querySelector<HTMLElement>(
      '[data-slot="map-file"][data-path="docs/billing.md"]',
    )!;
    await userEvent.click(within(row).getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("heading", { name: "docs/billing.md" })).toBeInTheDocument();
    expect(read).toHaveBeenCalledWith("t1", "docs/billing.md");
  });

  it("leaves a file the block cache holds no blocks for to the preview", async () => {
    const read = vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITH_FIXES);
    const unread: MapSummary = {
      ...MAP_PENDING,
      file_rows: MAP_PENDING.file_rows.map((f) =>
        f.relative === "docs/billing.md" ? { ...f, blocks: 0 } : f,
      ),
    };
    render(<Routed summary={unread} start="map" />);
    const row = document.querySelector<HTMLElement>(
      '[data-slot="map-file"][data-path="docs/billing.md"]',
    )!;
    await userEvent.click(within(row).getByRole("button", { name: "Open" }));
    // The preview sheet opens on the file; the Reader's page title does not.
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { level: 1, name: "docs/billing.md" })).toBeNull();
    expect(read).not.toHaveBeenCalled();
  });

  it("opens from a finding on the Checks page, at its block", async () => {
    const last: CheckRunResult = {
      pass: false,
      verdict: "failed",
      score: 80,
      files: [
        {
          path: "/work/fernwell/docs/billing.md",
          findings: [
            {
              category: "vocabulary",
              fails: true,
              message: 'Forbidden term "utilize" found',
              block_id: "p3",
            },
          ],
        },
      ],
    };
    vi.spyOn(api, "lastProjectChecks").mockResolvedValue(last);
    const read = vi.spyOn(api, "readerFile").mockResolvedValue(READER_WITH_FIXES);
    render(
      <ErrorProvider>
        <Routed summary={MAP_PENDING} start="checks" />
      </ErrorProvider>,
    );
    const card = await screen.findByTestId("finding-card");
    await userEvent.click(within(card).getByTestId("finding-open-document"));
    expect(await screen.findByRole("heading", { name: "docs/billing.md" })).toBeInTheDocument();
    expect(read).toHaveBeenCalledWith("t1", "/work/fernwell/docs/billing.md");
    expect(block("p3").getAttribute("data-focused")).toBe("true");
  });
});
