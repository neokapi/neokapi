import { describe, it, expect, vi } from "vitest";
import { deriveCapabilities } from "@neokapi/context-explorer";
import type { ApiAdapter } from "../api/adapter";
import { WORKSPACE_FREE_DIMENSIONS, createRestContextSource } from "./restContextSource";

const WS = "acme";

/** An adapter whose every method the explorer touches is a recording stub. */
function fakeApi(over: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    listContextProfiles: vi.fn().mockResolvedValue({
      profiles: [
        {
          slug: "default",
          name: "",
          label: "Brand",
          is_default: true,
          declared: true,
          collections: [],
          pending_changes: 0,
        },
        {
          slug: "channel~docs",
          name: "support",
          label: "Support · docs",
          is_default: false,
          declared: true,
          channel: "docs",
          voice: { id: "v1", name: "Support", version: 3 },
          collections: [
            {
              id: "col-1",
              name: "Docs",
              project_id: "p-site",
              project_name: "Northsea site",
              stream: "main",
              item_label: "file",
              owner: "recipe",
            },
          ],
          pending_changes: 0,
        },
      ],
      axes: ["channel"],
      terms: { concept_count: 42 },
      scan_scope: "workspace",
    }),
    listConcepts: vi.fn().mockResolvedValue({
      concepts: [
        {
          id: "c-signin",
          domain: "product",
          definition: "Authenticating into the product.",
          terms: [
            { text: "sign in", locale: "en-US", status: "preferred" },
            { text: "log in", locale: "en-US", status: "forbidden" },
          ],
          created_at: "",
          updated_at: "",
        },
      ],
      total_count: 1,
    }),
    listProjects: vi.fn().mockResolvedValue([
      {
        id: "p-site",
        name: "Northsea site",
        default_source_language: "en-US",
        target_languages: ["nb-NO"],
      },
      {
        id: "p-app",
        name: "Northsea app",
        default_source_language: "en-US",
        target_languages: ["de-DE"],
      },
    ]),
    getTranslationDashboard: vi.fn().mockResolvedValue({
      locale_stats: [
        {
          locale: "nb-NO",
          translated_blocks: 1,
          total_blocks: 2,
          translated_words: 3,
          total_words: 6,
          percentage: 50,
          ship_state: "pending",
        },
        {
          locale: "de-DE",
          translated_blocks: 2,
          total_blocks: 2,
          translated_words: 6,
          total_words: 6,
          percentage: 100,
          ship_state: "governed",
        },
      ],
      item_stats: [
        {
          item_name: "docs/help/billing.md",
          item_id: "i1",
          format: "markdown",
          collection_id: "col-1",
          collection_name: "Docs",
          block_count: 2,
          word_count: 6,
          locales: [
            {
              locale: "nb-NO",
              translated_blocks: 1,
              total_blocks: 2,
              translated_words: 3,
              total_words: 6,
              percentage: 50,
              ship_state: "pending",
            },
          ],
        },
      ],
      item_total: 1,
      collection_stats: [
        {
          collection_id: "col-1",
          collection_name: "Docs",
          channel: "docs",
          item_count: 1,
          block_count: 2,
          word_count: 6,
          locales: [],
        },
      ],
      total_blocks: 2,
      translatable_blocks: 2,
      total_source_words: 6,
    }),
    getConceptProjects: vi.fn().mockResolvedValue({
      concept_id: "c-signin",
      source: "graph",
      at: "2026-08-16T00:00:00Z",
      blocks: 12,
      occurrences: 49,
      uses_total: 2,
      projects: [
        {
          project_id: "p-site",
          project_name: "Northsea site",
          blocks: 10,
          occurrences: 42,
          collections: ["Docs"],
          status: "deprecated",
          discouraged: true,
          replacement: "sign in",
          terms: [
            {
              term: "log in",
              term_locale: "en-US",
              status: "deprecated",
              discouraged: true,
              blocks: 10,
              occurrences: 42,
            },
          ],
        },
        {
          project_id: "p-app",
          project_name: "Northsea app",
          blocks: 2,
          occurrences: 7,
          collections: [],
          status: "preferred",
          discouraged: false,
          terms: [
            {
              term: "sign in",
              term_locale: "en-US",
              status: "preferred",
              blocks: 2,
              occurrences: 7,
            },
          ],
        },
      ],
      uses: [
        {
          project_id: "p-site",
          project_name: "Northsea site",
          stream: "main",
          collection: "Docs",
          document: "docs/help/billing.md",
          block_id: "b1",
          locale: "en-US",
          term: "log in",
          occurrences: 2,
          text: "Log in to see your invoices.",
        },
      ],
    }),
    getConceptBlastRadius: vi.fn().mockResolvedValue({
      concept_id: "c-signin",
      total_blocks: 100,
      blocks: 12,
      occurrences: 49,
      words: 300,
      projects: [
        {
          project_id: "p-site",
          project_name: "Northsea site",
          blocks: 10,
          occurrences: 42,
          words: 250,
          collections: [
            {
              collection_id: "col-1",
              collection_name: "Docs",
              blocks: 10,
              occurrences: 42,
              words: 250,
              locales: [],
            },
          ],
        },
        {
          project_id: "p-app",
          project_name: "Northsea app",
          blocks: 2,
          occurrences: 7,
          words: 50,
          collections: [],
        },
      ],
      samples: [
        {
          project_id: "p-site",
          stream: "main",
          collection_id: "col-1",
          collection_name: "Docs",
          locale: "en-US",
          item_name: "docs/help/billing.md",
          block_id: "b1",
          text: "Sign in to see your invoices.",
        },
      ],
    }),
    getMemoryEntries: vi.fn().mockResolvedValue({
      entries: [
        {
          id: "m1",
          source: "Sign in to manage your subscription.",
          target: "Logg inn …",
          source_language: "en-US",
          target_language: "nb-NO",
          updated_at: "",
        },
      ],
      total_count: 1,
    }),
    listStreams: vi.fn().mockResolvedValue([{ name: "main" }, { name: "next" }]),
    listCollections: vi.fn().mockResolvedValue([{ id: "col-1", name: "Docs" }]),
    getProject: vi.fn().mockResolvedValue({
      id: "p-site",
      name: "Northsea site",
      default_source_language: "en-US",
      target_languages: ["nb-NO"],
    }),
    ...over,
  } as unknown as ApiAdapter;
}

describe("createRestContextSource", () => {
  it("leaves every dimension below the workspace free", () => {
    const caps = deriveCapabilities(createRestContextSource(fakeApi(), WS));
    expect(caps.free).toEqual(WORKSPACE_FREE_DIMENSIONS);
    expect(caps.free).toContain("project");
    expect(caps.relations).toBe("workspace");
    expect(caps.governance).toBe("derived");
  });

  it("resolves the profile whose collections sit at the point", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.governs({ scope: { workspace: WS, collection: "Docs" } });
    expect(answer.point.profile).toBe("support");
    expect(answer.point.channel).toBe("docs");
    expect(answer.voice?.name).toBe("Support");
    expect(answer.scope).toBe("workspace");
  });

  it("falls back to the default profile where nothing else claims the point", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.governs({ scope: { workspace: WS, collection: "Unknown" } });
    expect(answer.point.default).toBe(true);
    expect(answer.point.profile).toBeUndefined();
  });

  it("flattens the workspace vocabulary, marking the discouraged term", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.governs({ scope: { workspace: WS } });
    const byTerm = Object.fromEntries(answer.terms.map((t) => [t.term, t]));
    expect(byTerm["log in"].discouraged).toBe(true);
    expect(byTerm["log in"].replacement).toBe("sign in");
    expect(answer.terms_total).toBe(42);
  });

  it("answers above a project with its collections, not a flat item list", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.lives!({ scope: { workspace: WS } });
    expect(answer.collections.map((c) => c.name)).toEqual(["Docs"]);
    expect(answer.items).toEqual([]);
    expect(answer.notes).not.toEqual([]);
  });

  it("carries the server's ship state into a project's answer", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.lives!({ scope: { workspace: WS, project: "p-site" } });
    const byLocale = Object.fromEntries(answer.coverage.map((row) => [row.locale, row]));
    expect(byLocale["nb-NO"].ship_state).toBe("pending");
    expect(byLocale["de-DE"].ship_state).toBe("governed");
    expect(answer.items[0].document).toBe("docs/help/billing.md");
  });

  it("narrows a project's answer to one language", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.lives!({
      scope: { workspace: WS, project: "p-site", locale: "de-DE" },
    });
    expect(answer.coverage.map((row) => row.locale)).toEqual(["de-DE"]);
  });

  it("answers which projects use a concept from the graph, not a content scan", async () => {
    const api = fakeApi();
    const source = createRestContextSource(api, WS);
    const answer = await source.relates!(
      { kind: "concept", id: "c-signin", label: "sign in" },
      { scope: { workspace: WS } },
    );
    expect(api.getConceptProjects).toHaveBeenCalled();
    expect(api.getConceptBlastRadius).not.toHaveBeenCalled();
    expect(answer.reach).toBe("workspace");
    expect(answer.projects.map((p) => p.project_name)).toEqual(["Northsea site", "Northsea app"]);
    expect(answer.occurrences_total).toBe(49);
    expect(answer.occurrences[0].project).toBe("Northsea site");
    expect(answer.occurrences[0].snippet).toBe("Log in to see your invoices.");
  });

  it("narrows the projects list by asking the same query with the project pinned", async () => {
    const api = fakeApi({
      getConceptProjects: vi.fn().mockResolvedValue({
        concept_id: "c-signin",
        source: "graph",
        at: "2026-08-16T00:00:00Z",
        blocks: 2,
        occurrences: 7,
        uses_total: 0,
        projects: [
          { project_id: "p-app", project_name: "Northsea app", blocks: 2, occurrences: 7 },
        ],
        uses: [],
      }),
    });
    const source = createRestContextSource(api, WS);
    const answer = await source.relates!(
      { kind: "concept", id: "c-signin" },
      { scope: { workspace: WS, project: "p-app" } },
    );
    expect(api.getConceptProjects).toHaveBeenCalledWith(
      WS,
      "c-signin",
      expect.objectContaining({ project: "p-app" }),
    );
    expect(answer.projects.map((p) => p.project)).toEqual(["p-app"]);
  });

  it("carries how the concept stands in each project it names", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.relates!(
      { kind: "concept", id: "c-signin", label: "sign in" },
      { scope: { workspace: WS } },
    );
    const byProject = Object.fromEntries(answer.projects.map((p) => [p.project, p]));
    expect(byProject["p-site"].discouraged).toBe(true);
    expect(byProject["p-site"].status).toBe("deprecated");
    expect(byProject["p-site"].replacement).toBe("sign in");
    expect(byProject["p-site"].terms?.[0].term).toBe("log in");
    expect(byProject["p-app"].discouraged).toBe(false);
    expect(byProject["p-app"].status).toBe("preferred");
  });

  it("asks the workspace vocabulary at an instant, not as declared", async () => {
    const api = fakeApi();
    const source = createRestContextSource(api, WS);
    await source.governs({ scope: { workspace: WS } });
    expect(api.listConcepts).toHaveBeenCalledWith(
      WS,
      expect.objectContaining({ at: expect.any(String) }),
    );
  });

  it("says a floor is a floor when the answer came back partial", async () => {
    const api = fakeApi({
      getConceptProjects: vi.fn().mockResolvedValue({
        concept_id: "c-signin",
        source: "scan",
        at: "2026-08-16T00:00:00Z",
        blocks: 1,
        occurrences: 1,
        uses_total: 0,
        projects: [
          { project_id: "p-site", project_name: "Northsea site", blocks: 1, occurrences: 1 },
        ],
        uses: [],
        partial: true,
        partial_reason:
          "the scan reached this report's time budget before it had covered the workspace",
        notes: ["no push has recorded this concept in the workspace graph yet"],
      }),
    });
    const source = createRestContextSource(api, WS);
    const answer = await source.relates!(
      { kind: "concept", id: "c-signin" },
      { scope: { workspace: WS } },
    );
    expect(answer.projects).toHaveLength(1);
    expect(answer.notes.join(" ")).toContain("floor");
    expect(answer.notes.join(" ")).toContain("not reached");
  });

  it("groups search results by kind and never merges them", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const answer = await source.search!("sign in", { scope: { workspace: WS } });
    expect(answer.groups.map((g) => g.kind)).toEqual(["terms", "precedent"]);
  });

  it("offers the projects a workspace holds and the streams a project has", async () => {
    const api = fakeApi();
    const source = createRestContextSource(api, WS);
    const projects = await source.options!({ workspace: WS }, "project");
    expect(projects.map((o) => o.label)).toEqual(["Northsea site", "Northsea app"]);

    const streams = await source.options!({ workspace: WS, project: "p-site" }, "stream");
    expect(streams.map((o) => o.value)).toEqual(["main", "next"]);
    expect(api.listStreams).toHaveBeenCalledWith(WS, "p-site");

    // Above a project there is no stream to choose: streams belong to one.
    expect(await source.options!({ workspace: WS }, "stream")).toEqual([]);
  });

  it("offers every coordinate the workspace's points name", async () => {
    const source = createRestContextSource(fakeApi(), WS);
    const points = await source.options!({ workspace: WS }, "coordinate");
    expect(points.map((o) => o.value)).toEqual(["support/docs"]);
  });
});
