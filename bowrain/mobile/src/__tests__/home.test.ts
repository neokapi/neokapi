import type { ReviewHome, ReviewInboxLanguage } from "@neokapi/bowrain-client";

import { nextUp, projectKey, summarize } from "../lib/home";

function lang(
  workspace: string,
  project: string,
  locale: string,
  waiting: number,
): ReviewInboxLanguage {
  return {
    workspace,
    workspace_name: workspace.toUpperCase(),
    project,
    project_name: `Project ${project}`,
    stream: "main",
    locale,
    waiting,
    fast: waiting,
    closer: 0,
    desk: 0,
    unclassified: 0,
  };
}

const home: ReviewHome = {
  context: [
    {
      workspace: "acme",
      workspace_name: "ACME",
      project: "p2",
      project_name: "Project p2",
      waiting: 4,
      by_kind: { concept: 3, note: 1 },
    },
  ],
  languages: [
    lang("acme", "p1", "nb", 12),
    lang("acme", "p1", "de", 5),
    lang("beta", "p9", "fr", 3),
    lang("beta", "p9", "es", 0),
  ],
};

describe("summarize", () => {
  it("totals what waits per workspace and per project", () => {
    const s = summarize(home);
    expect(s.total).toEqual({ translations: 20, context: 4, languages: 3 });
    expect(s.byWorkspace.get("acme")).toEqual({ translations: 17, context: 4, languages: 2 });
    expect(s.byWorkspace.get("beta")).toEqual({ translations: 3, context: 0, languages: 1 });
    const p1 = s.byProject.get(projectKey("acme", "p1"));
    expect(p1?.translations).toBe(17);
    expect([...(p1?.byLocale.keys() ?? [])]).toEqual(["nb", "de"]);
    expect(s.byProject.get(projectKey("acme", "p2"))?.contextEntry?.waiting).toBe(4);
    // A language with nothing waiting is not lit.
    expect(s.byProject.get(projectKey("beta", "p9"))?.byLocale.has("es")).toBe(false);
  });

  it("reads a missing inbox as nothing waiting", () => {
    expect(summarize(null).total).toEqual({ translations: 0, context: 0, languages: 0 });
  });
});

describe("nextUp", () => {
  it("offers the inbox's first language", () => {
    const n = nextUp(home);
    expect(n?.kind).toBe("language");
    expect(n?.kind === "language" && n.entry.locale).toBe("nb");
    expect(n?.more).toBe(3);
  });

  it("prefers a language waiting in the project opened last", () => {
    const n = nextUp(home, { ws: "beta", project: "p9" });
    expect(n?.kind === "language" && n.entry.locale).toBe("fr");
  });

  it("falls back to the inbox order when nothing waits in the project opened last", () => {
    const n = nextUp(home, { ws: "acme", project: "p2" });
    expect(n?.kind === "language" && n.entry.locale).toBe("nb");
  });

  it("offers the context when no translation waits", () => {
    const n = nextUp({ context: home.context, languages: [] });
    expect(n?.kind).toBe("context");
    expect(n?.more).toBe(0);
  });

  it("offers nothing when nothing waits", () => {
    expect(nextUp({ context: [], languages: [lang("a", "b", "nb", 0)] })).toBeNull();
    expect(nextUp(null)).toBeNull();
  });
});
