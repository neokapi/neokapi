import { render, screen } from "./testUtils";
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { ContextHub, contextSectionOf, decideRowOf } from "../components/ContextHub";
import { api } from "../hooks/useApi";
import { CONTEXT_DIGEST } from "../stories/fixtures/contextDigest";

afterEach(() => {
  vi.restoreAllMocks();
});

const QUICKCAST = "0njy7pfhjp00000000000000";

/** The row Decide has selected, by its item id. */
async function selectedId(): Promise<string | null | undefined> {
  const list = await screen.findByRole("listbox", { name: "Decide" });
  return list.querySelector('[role="option"][aria-selected="true"]')?.getAttribute("data-id");
}

describe("the Map's Decide on one suggestion", () => {
  it("names the row in the view, and only a view that names one opens on it", () => {
    expect(contextSectionOf(`context:rules:suggested:${QUICKCAST}`)).toBe("rules");
    expect(decideRowOf(`context:rules:suggested:${QUICKCAST}`)).toBe(`suggested:${QUICKCAST}`);
    expect(decideRowOf("context:rules")).toBeUndefined();
    expect(decideRowOf("context")).toBeUndefined();
  });

  it("opens Decide with that suggestion selected", async () => {
    vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(
      <ContextHub
        tabID="t1"
        projectName="Fernwell"
        section="rules"
        decideRow={`suggested:${QUICKCAST}`}
      />,
    );
    expect(await selectedId()).toBe(QUICKCAST);
  });

  it("opens Decide on what needs a person first when no row is named", async () => {
    vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<ContextHub tabID="t1" projectName="Fernwell" section="rules" />);
    expect(await selectedId()).toBe(CONTEXT_DIGEST.conflicts[0].sides[0].id);
  });
});
