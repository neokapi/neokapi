import { describe, it, expect, vi } from "vitest";
import { deriveCapabilities } from "@neokapi/context-explorer";
import {
  DESKTOP_FREE_DIMENSIONS,
  createLocalContextSource,
  type ContextBackend,
} from "../lib/localContextSource";

const TAB = "tab-1";
const SCOPE = { project: "Northsea", stream: "main", collection: "Docs" };

/** A backend whose every method is a recording stub returning a canned answer. */
function fakeBackend(over: Partial<ContextBackend> = {}): ContextBackend {
  return {
    governs: vi.fn().mockResolvedValue({
      point: { profile: "support", channel: "docs", collection: "Docs", default: false },
      voice: { name: "Support", field: "profiles.support.voice" },
      concepts: [
        {
          id: "c-signin",
          definition: "Authenticating into the product.",
          terms: [
            { text: "sign in", locale: "en-US", status: "preferred" },
            { text: "log in", locale: "en-US", status: "forbidden" },
          ],
        },
      ],
      concepts_total: 1,
      profiles: [{ name: "campaign", state: "expired", valid_to: "2026-06-30" }],
      notes: ["terms moved since this was last read here"],
    }),
    lives: vi.fn().mockResolvedValue({
      point: { collection: "Docs", default: false },
      collections: [{ name: "Docs", channel: "docs", profile: "support", blocks: 2 }],
      coverage: [
        { locale: "nb-NO", units: 2, covered: 1 },
        { locale: "de-DE", units: 2, covered: 0 },
      ],
      items: [
        { hash: "h1", document: "docs/help/billing.json", locale: "", text: "Sign in" },
        { hash: "h1", document: "docs/help/billing.json", locale: "nb-NO", text: "Logg inn" },
      ],
      items_total: 2,
      notes: [],
    }),
    relates: vi.fn().mockResolvedValue({
      subject: "sign in",
      reach: "project",
      occurrences: [
        {
          concept_id: "c-signin",
          term: "sign in",
          locale: "en-US",
          document: "docs/help/billing.json",
          matched: "Sign in",
          snippet: "Sign in to see your invoices.",
        },
      ],
      occurrences_total: 1,
      blessings: [{ unit: "docs/help/billing.json#intro", variant: "nb-NO", status: "approved" }],
      notes: [],
    }),
    search: vi.fn().mockResolvedValue({
      answer: {
        query: "sign in",
        scope: "project",
        terms: [{ concept_id: "c-signin", term: "sign in", discouraged: false }],
        precedent: [{ entry_id: "m1", text: "Sign in to manage your subscription." }],
        profiles: [],
        notes: [],
      },
      content: [{ hash: "h1", document: "docs/help/billing.json", locale: "", text: "Sign in" }],
    }),
    options: vi.fn().mockResolvedValue([{ value: "Docs" }, { value: "App" }]),
    ...over,
  };
}

describe("createLocalContextSource", () => {
  it("declares the dimensions a desktop tab lets the reader move", () => {
    const caps = deriveCapabilities(createLocalContextSource(TAB, fakeBackend()));
    expect(caps.free).toEqual(DESKTOP_FREE_DIMENSIONS);
    expect(caps.free).not.toContain("project");
    expect(caps.relations).toBe("project");
  });

  it("passes the tuple's collection and path to the backend", async () => {
    const backend = fakeBackend();
    const source = createLocalContextSource(TAB, backend);
    await source.governs({ scope: { ...SCOPE, path: "docs/help/billing.json" }, limit: 5 });
    expect(backend.governs).toHaveBeenCalledWith(TAB, "Docs", "docs/help/billing.json", 5);
  });

  it("flattens concepts into term rows, marking the discouraged one", async () => {
    const source = createLocalContextSource(TAB, fakeBackend());
    const answer = await source.governs({ scope: SCOPE });
    const byTerm = Object.fromEntries(answer.terms.map((t) => [t.term, t]));
    expect(byTerm["log in"].discouraged).toBe(true);
    expect(byTerm["log in"].replacement).toBe("sign in");
    expect(byTerm["sign in"].discouraged).toBe(false);
    expect(byTerm["sign in"].replacement).toBeUndefined();
  });

  it("carries the staleness note through untouched", async () => {
    const source = createLocalContextSource(TAB, fakeBackend());
    const answer = await source.governs({ scope: SCOPE });
    expect(answer.notes).toEqual(["terms moved since this was last read here"]);
  });

  it("narrows content and coverage to a chosen language", async () => {
    const source = createLocalContextSource(TAB, fakeBackend());
    const all = await source.lives!({ scope: SCOPE });
    expect(all.coverage).toHaveLength(2);
    expect(all.items).toHaveLength(2);

    const nb = await source.lives!({ scope: { ...SCOPE, locale: "nb-NO" } });
    expect(nb.coverage.map((row) => row.locale)).toEqual(["nb-NO"]);
    expect(nb.items.map((item) => item.locale)).toEqual(["nb-NO"]);
  });

  it("reports project reach and lists no projects", async () => {
    const source = createLocalContextSource(TAB, fakeBackend());
    const answer = await source.relates!(
      { kind: "concept", id: "c-signin", label: "sign in" },
      { scope: SCOPE },
    );
    expect(answer.reach).toBe("project");
    expect(answer.projects).toEqual([]);
    expect(answer.occurrences).toHaveLength(1);
    expect(answer.blessings).toHaveLength(1);
  });

  it("groups search results by kind and never merges them", async () => {
    const source = createLocalContextSource(TAB, fakeBackend());
    const answer = await source.search!("sign in", { scope: SCOPE });
    expect(answer.groups.map((g) => g.kind)).toEqual(["terms", "precedent", "content"]);
  });

  it("omits a group the answer holds nothing for", async () => {
    const backend = fakeBackend({
      search: vi.fn().mockResolvedValue({
        answer: { query: "x", scope: "project", terms: [], precedent: [], notes: [] },
        content: [],
      }),
    });
    const answer = await createLocalContextSource(TAB, backend).search!("x", { scope: SCOPE });
    expect(answer.groups).toEqual([]);
  });

  it("reports an absent Wails runtime rather than answering empty", async () => {
    const backend = fakeBackend({ governs: vi.fn().mockResolvedValue(null) });
    await expect(createLocalContextSource(TAB, backend).governs({ scope: SCOPE })).rejects.toThrow(
      /not available/,
    );
  });
});
