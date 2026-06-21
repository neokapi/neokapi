// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import ToolDropWidget, { parseWordCountStat } from "./ToolDropWidget";
import PseudoTranslateWidget from "./PseudoTranslateWidget";
import StatsWidget from "./StatsWidget";
import SearchReplaceWidget, { buildSearchReplaceRecipe } from "./SearchReplaceWidget";

// vite-plus does not auto-clean the DOM between tests, so unmount each render to
// keep getByText queries scoped to a single widget instance.
afterEach(cleanup);

// ── buildSearchReplaceRecipe ─────────────────────────────────────────────────

describe("buildSearchReplaceRecipe", () => {
  it("emits a one-step lab flow carrying the pair as JSON in the config", () => {
    const recipe = buildSearchReplaceRecipe("color", "colour", false);
    expect(recipe).toContain("version: v1");
    expect(recipe).toContain("  lab:");
    expect(recipe).toContain("    steps:");
    expect(recipe).toContain("      - tool: search-replace");
    expect(recipe).toContain("        config:");
    expect(recipe).toContain("pairs:");
    // The pair appears as JSON with the engine's JSON field names.
    expect(recipe).toContain('"search":"color"');
    expect(recipe).toContain('"replace":"colour"');
    expect(recipe).toContain('"isRegex":false');
    expect(recipe).toContain("source: true");
    expect(recipe).toContain("target: false");
    expect(recipe).toContain("regEx: false");
  });

  it("sets regEx true when regex mode is on", () => {
    const recipe = buildSearchReplaceRecipe("colou?r", "color", true);
    expect(recipe).toContain('"isRegex":true');
    expect(recipe).toContain("regEx: true");
  });

  it("JSON-escapes find/replace strings so quotes survive", () => {
    const recipe = buildSearchReplaceRecipe('say "hi"', "say bye", false);
    expect(recipe).toContain('say \\"hi\\"');
  });
});

// ── parseWordCountStat ───────────────────────────────────────────────────────

describe("parseWordCountStat", () => {
  const json = JSON.stringify({
    total_source_words: 12,
    document_count: 1,
    documents: { "/project/messages.json": { source_words: 12, block_count: 4 } },
  });

  it("parses plain word-count --json into blocks/words/chars cards", () => {
    const cards = parseWordCountStat(json);
    expect(cards.map((c) => c.label)).toEqual(["Blocks", "Words", "~Characters"]);
    expect(cards[0].value).toBe("4");
    expect(cards[1].value).toBe("12");
  });

  it("strips ANSI colour codes the wasm build emits (CLICOLOR_FORCE=1)", () => {
    // Wrap keys/values in CSI sequences like the browser build does.
    const esc = String.fromCharCode(27); // ESC
    const colorized = json
      .replace(/"total_source_words"/, `${esc}[1;34m"total_source_words"${esc}[0m`)
      .replace(/12/g, `${esc}[33m12${esc}[0m`);
    const cards = parseWordCountStat(colorized);
    expect(cards.length).toBe(3);
    expect(cards[1].value).toBe("12");
  });

  it("returns no cards for unparseable output", () => {
    expect(parseWordCountStat("not json")).toEqual([]);
  });
});

// ── Idle render (assets=null → no WASM boot) ─────────────────────────────────

describe("ToolDropWidget (idle)", () => {
  // The lab gates work behind the shared zero-shift GateOverlay: the body is
  // laid out from the start and the play button (aria-label "Run") covers it
  // until the engine is ready. assets=null keeps the engine un-booted, so we
  // exercise the rendered body without WASM. getByLabelText("Run") targets the
  // gate's play button unambiguously (body controls have their own names).

  it("gates the widget behind an explicit Run play button", () => {
    render(
      <ToolDropWidget
        assets={null}
        tool="pseudo-translate"
        buildArgv={(i, o) => ["pseudo-translate", i, "-o", o]}
        autoRun={false}
      />,
    );
    expect(screen.getByLabelText("Run")).toBeTruthy();
  });

  it("renders the drop-zone and sample chips, without booting WASM", () => {
    render(
      <ToolDropWidget
        assets={null}
        tool="pseudo-translate"
        buildArgv={(i, o) => ["pseudo-translate", i, "-o", o]}
        autoRun={false}
      />,
    );
    expect(screen.getByText(/Drop or choose a file/i)).toBeTruthy();
    expect(screen.getByText(/Try a sample/i)).toBeTruthy();
    // Both hero samples are offered as chips. "messages.json" also appears in the
    // header (it is the default input), so use getAllByText for it.
    expect(screen.getAllByText("messages.json").length).toBeGreaterThan(0);
    expect(screen.getByText("welcome.docx")).toBeTruthy();
  });

  it("honours sampleIds to restrict the offered chips", () => {
    render(
      <ToolDropWidget
        assets={null}
        tool="word-count"
        buildArgv={(i) => ["word-count", i, "--json"]}
        sampleIds={["json"]}
        render="stat"
        autoRun={false}
      />,
    );
    expect(screen.getAllByText("messages.json").length).toBeGreaterThan(0);
    expect(screen.queryByText("welcome.docx")).toBeNull();
  });
});

// ── Variants render at idle ──────────────────────────────────────────────────

describe("per-tool variants (idle)", () => {
  const pressRun = () => fireEvent.click(screen.getByRole("button", { name: /run/i }));

  it("PseudoTranslateWidget renders after Run", () => {
    render(<PseudoTranslateWidget assets={null} />);
    pressRun();
    expect(screen.getByText(/Try a sample/i)).toBeTruthy();
  });

  it("StatsWidget renders after Run", () => {
    render(<StatsWidget assets={null} />);
    pressRun();
    expect(screen.getByText(/Try a sample/i)).toBeTruthy();
  });

  it("SearchReplaceWidget renders find/replace inputs and a regex toggle", () => {
    render(<SearchReplaceWidget assets={null} />);
    expect(screen.getByText("Find")).toBeTruthy();
    expect(screen.getByText("Replace")).toBeTruthy();
    expect(screen.getByText("Regex")).toBeTruthy();
    // Seeded with the US→British default.
    expect(screen.getByDisplayValue("color")).toBeTruthy();
    expect(screen.getByDisplayValue("colour")).toBeTruthy();
  });
});
