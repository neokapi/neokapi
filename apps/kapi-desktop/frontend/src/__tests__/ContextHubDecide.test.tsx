import { render, screen } from "./testUtils";
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { ContextHub, contextSectionOf, decideRowOf } from "../components/ContextHub";
import { decideRowKey } from "../components/MapPage";
import { api } from "../hooks/useApi";
import { CONTEXT_DIGEST } from "../stories/fixtures/contextDigest";
import { DECIDE_IDS, DECIDE_VIEW, DISAGREEMENT } from "../stories/fixtures/rulesDecide";
import type { MapSuggestionRow } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

const QUICKCAST = DECIDE_IDS.quickcast;

/** The question Decide has selected, by its suggestion's id. */
async function selectedId(): Promise<string | null | undefined> {
  const list = await screen.findByRole("listbox", { name: "Suggestions to decide" });
  return list.querySelector('[role="option"][aria-selected="true"]')?.getAttribute("data-id");
}

/** A Map row for one suggestion, as MapSummary sends it. */
function mapRow(group: MapSuggestionRow["group"], id: string): MapSuggestionRow {
  const item = { ...CONTEXT_DIGEST.suggested[0].groups[0].items[0], id };
  return { group, item, blocks: 1, files: 1 };
}

describe("the Map's Decide on one suggestion", () => {
  it("names the row in the view, and only a view that names one opens on it", () => {
    expect(contextSectionOf(`context:rules:suggested:${QUICKCAST}`)).toBe("rules");
    expect(decideRowOf(`context:rules:suggested:${QUICKCAST}`)).toBe(`suggested:${QUICKCAST}`);
    expect(decideRowOf("context:rules")).toBeUndefined();
    expect(decideRowOf("context")).toBeUndefined();
  });

  it("opens Decide with that suggestion's question selected", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(DECIDE_VIEW);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(
      <ContextHub
        tabID="t1"
        projectName="KapiMart"
        section="rules"
        decideRow={decideRowKey(mapRow("suggested", QUICKCAST))}
      />,
    );
    expect(await selectedId()).toBe(QUICKCAST);
  });

  it("opens a disagreement the Map lists by its first side on its question", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(DECIDE_VIEW);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    const side = DISAGREEMENT.options[1].id!;
    render(
      <ContextHub
        tabID="t1"
        projectName="KapiMart"
        section="rules"
        decideRow={decideRowKey(mapRow("conflict", side))}
      />,
    );
    expect(await selectedId()).toBe(DISAGREEMENT.options[0].id);
  });

  it("opens Decide on the first question when no row is named", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(DECIDE_VIEW);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<ContextHub tabID="t1" projectName="KapiMart" section="rules" />);
    expect(await selectedId()).toBe(DECIDE_VIEW.decisions[0].options[0].id);
  });

  it("reads the digest on a tab with no checkout, which has no files to count uses in", async () => {
    const view = vi.spyOn(api, "rulesDecideView");
    vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<ContextHub tabID="t1" projectName="KapiMart" section="rules" contextOnly />);
    expect(await screen.findByRole("listbox", { name: "Decide" })).toBeTruthy();
    expect(view).not.toHaveBeenCalled();
  });
});
