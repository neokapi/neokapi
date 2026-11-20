import type {
  PlanStep,
  ReviewHome,
  ReviewInboxLanguage,
  ReviewInboxProject,
} from "@neokapi/bowrain-client";

import { homeStep, projectKey, summarize } from "../lib/home";

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
  projects: [],
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

describe("homeStep", () => {
  const step = (level: PlanStep["level"]): PlanStep => ({
    level,
    state: "open",
    count: 1,
    progress: 0,
    done: false,
    changed: 0,
  });
  const quota = {
    level: "context" as const,
    reason: "waiting_quota" as const,
    quotaResetsAt: "2026-12-01T00:00:00Z",
  };
  function proj(
    workspace: string,
    project: string,
    over: Partial<ReviewInboxProject> & { next?: PlanStep["level"] | null } = {},
  ): ReviewInboxProject {
    const { next = null, ...rest } = over;
    return {
      workspace,
      workspaceName: workspace.toUpperCase(),
      project,
      projectName: `Project ${project}`,
      followed: ["nb"],
      saved: true,
      canManage: false,
      waiting: 1,
      plan: {
        involvement: "context",
        next,
        waiting: [],
        held: [],
        steps: [
          step("coordinates"),
          step("context"),
          step("exceptions"),
          step("interfaces"),
          step("policy"),
        ],
      },
      ...rest,
    };
  }
  const withProjects = (...projects: ReviewInboxProject[]): ReviewHome => ({ ...home, projects });

  it("opens the setup before anything else in a project not set up", () => {
    const n = homeStep(withProjects(proj("acme", "p1", { saved: false, next: "coordinates" })));
    expect(n?.kind).toBe("setup");
    expect(n?.more).toBe(4);
  });

  it("names the coordinates and the context as their own steps", () => {
    expect(homeStep(withProjects(proj("acme", "p1", { next: "coordinates" })))).toMatchObject({
      kind: "step",
      step: { level: "coordinates" },
    });
    expect(homeStep(withProjects(proj("acme", "p2", { next: "context" })))).toMatchObject({
      kind: "step",
      step: { level: "context" },
    });
  });

  it("opens the project's language deck for the exceptions and the pages", () => {
    const n = homeStep(withProjects(proj("acme", "p1", { next: "interfaces" })));
    expect(n?.kind).toBe("deck");
    expect(n?.kind === "deck" && n.entry.locale).toBe("nb");
    expect(n?.more).toBe(3);
  });

  it("carries the steps that only wait beside the next one", () => {
    const p = proj("acme", "p1", { next: "exceptions" });
    p.plan!.waiting = [quota];
    expect(homeStep(withProjects(p))?.waiting).toEqual([quota]);
  });

  it("prefers the project opened last when it has a next step", () => {
    const both = withProjects(
      proj("acme", "p1", { next: "interfaces" }),
      proj("beta", "p9", { next: "context" }),
    );
    expect(homeStep(both, { ws: "beta", project: "p9" })?.project.project).toBe("p9");
  });

  it("passes over the project opened last when it has nothing to act on", () => {
    const idle = proj("beta", "p9");
    idle.plan!.waiting = [quota];
    const both = withProjects(idle, proj("acme", "p1", { next: "interfaces" }));
    expect(homeStep(both, { ws: "beta", project: "p9" })?.project.project).toBe("p1");
  });

  it("says why when every step only waits or is done", () => {
    const idle = proj("acme", "p1");
    idle.plan!.waiting = [quota];
    expect(homeStep(withProjects(idle))).toMatchObject({ kind: "waiting", waiting: [quota] });
  });

  it("reads a plan cached by an older update without its waiting list", () => {
    const old = proj("acme", "p1");
    delete (old.plan as { waiting?: unknown }).waiting;
    expect(() => homeStep(withProjects(old))).not.toThrow();
  });

  it("offers nothing when nothing waits", () => {
    expect(homeStep(withProjects(proj("acme", "p1")))).toBeNull();
    expect(homeStep({ context: [], languages: [], projects: [] })).toBeNull();
    expect(homeStep(null)).toBeNull();
  });
});
