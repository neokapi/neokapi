import { describe, expect, it } from "vite-plus/test";

import {
  createBowrainClient,
  inboxProjectFromServer,
  nextPlanStep,
  projectLanguagesAfterSync,
  reviewLanguagesFromServer,
} from "../src/index.ts";

const languages = {
  followed: ["nb"],
  saved: true,
  allowed: ["nb", "sv"],
  source: "en",
  project: ["nb", "sv"],
  pending_project: ["nb", "sv", "de"],
  can_manage: true,
};

function recording(body: object) {
  const calls: { url: string; init?: RequestInit }[] = [];
  const client = createBowrainClient({
    baseUrl: "http://server",
    getAccessToken: () => "t",
    fetch: (async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      return new Response(JSON.stringify(body), { status: 200 });
    }) as typeof fetch,
  });
  return { client, calls };
}

describe("followed languages", () => {
  it("reads, saves and changes a project's languages", async () => {
    const { client, calls } = recording(languages);
    const got = await client.reviewLanguages("ws", "p");
    expect(got).toEqual({
      followed: ["nb"],
      saved: true,
      allowed: ["nb", "sv"],
      source: "en",
      project: ["nb", "sv"],
      pendingProject: ["nb", "sv", "de"],
      canManage: true,
    });
    expect(calls[0]?.url).toBe("http://server/api/v1/ws/p/review/languages");

    await client.setFollowedLanguages("ws", "p", ["sv"]);
    expect(calls[1]?.url).toBe("http://server/api/v1/ws/p/review/languages");
    expect(calls[1]?.init?.method).toBe("PUT");
    expect(JSON.parse(String(calls[1]?.init?.body))).toEqual({ followed: ["sv"] });

    await client.setProjectLanguage("ws", "p", "de", true);
    expect(calls[2]?.url).toBe("http://server/api/v1/ws/p/review/languages/project");
    expect(calls[2]?.init?.method).toBe("POST");
    expect(JSON.parse(String(calls[2]?.init?.body))).toEqual({ locale: "de", enabled: true });
  });

  it("reads an answer with nothing saved and nothing pending", () => {
    const got = reviewLanguagesFromServer({ allowed: ["nb"], followed: ["nb"], project: ["nb"] });
    expect(got.saved).toBe(false);
    expect(got.canManage).toBe(false);
    expect(got.pendingProject).toBeUndefined();
    expect(projectLanguagesAfterSync(got)).toEqual(["nb"]);
    expect(projectLanguagesAfterSync(reviewLanguagesFromServer(languages))).toEqual([
      "nb",
      "sv",
      "de",
    ]);
  });
});

describe("inbox projects", () => {
  it("maps the plan in brief and drops a level it does not know", async () => {
    const project = {
      workspace: "ws",
      workspace_name: "WS",
      project: "p",
      project_name: "Site",
      followed: ["nb"],
      saved: false,
      can_manage: true,
      waiting: 40,
      plan: {
        involvement: "context",
        next: "key_language",
        steps: [
          { level: "coordinates", state: "done", count: 0, progress: 100, done: true },
          { level: "key_language", state: "not_started", count: 0, progress: 0, scan: "queued" },
          { level: "exceptions", count: 3, progress: 140 },
          { level: "interfaces", count: 34, progress: 0 },
          { level: "policy", count: 800, progress: 0, settle: "held" },
          { level: "something_new", count: 1 },
        ],
        held: [
          { locale: "nb", count: 1200, reason: "key_language" },
          { locale: "sv", count: 3, reason: "something_new" },
        ],
      },
    };
    const { client } = recording({ context: [], languages: [], projects: [project] });
    const home = await client.reviewHome();
    const p = home?.projects[0];
    expect(p?.projectName).toBe("Site");
    expect(p?.canManage).toBe(true);
    expect(p?.plan?.next).toBe("key_language");
    expect(p?.plan?.steps.map((s) => s.level)).toEqual([
      "coordinates",
      "key_language",
      "exceptions",
      "interfaces",
      "policy",
    ]);
    expect(p?.plan?.steps[2]?.progress).toBe(100);
    expect(p?.plan?.steps[4]?.settle).toBe("held");
    expect(p?.plan?.steps[1]?.changed).toBe(0);
    expect(p?.plan?.steps[1]).toMatchObject({ state: "not_started", scan: "queued" });
    expect(p?.plan?.steps[3]?.state).toBe("open");
    expect(p?.plan?.held).toEqual([{ locale: "nb", count: 1200, reason: "key_language" }]);
    expect(nextPlanStep(p!.plan!.steps, p!.plan!.next)?.level).toBe("key_language");
  });

  it("reads a project with no plan", () => {
    const p = inboxProjectFromServer({
      workspace: "ws",
      workspace_name: "WS",
      project: "p",
      project_name: "Site",
      saved: true,
      followed: [],
    });
    expect(p.plan).toBeUndefined();
    expect(p.waiting).toBe(0);
    expect(nextPlanStep([], null)).toBeUndefined();
  });
});
