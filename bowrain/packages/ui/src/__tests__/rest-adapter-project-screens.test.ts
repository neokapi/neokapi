import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import {
  keptRulesFixture,
  mapSummaryFixture,
  readerFileFixture,
  rulesDecideViewFixture,
} from "@neokapi/project-screens/fixtures.ts";
import { RestApiAdapter } from "../api/rest-adapter";

// The project screens' routes on the server (bowrain/server
// handlers_rules_decide.go and handlers_map_reader.go). The Map, Reader,
// Decide and kept-rule bodies are the JSON host/screens marshals for each
// screen, pinned by its fixtures test, which the server answers with; the
// other bodies are the ones the handler tests decode.

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: status < 400,
    status,
    json: async () => body,
    text: async () => JSON.stringify(body),
    headers: { get: () => null },
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

/** The path, query and init of the first request the adapter made. */
function call(fetchMock: ReturnType<typeof mockFetch>) {
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit | undefined];
  const u = new URL(url);
  return {
    path: u.pathname,
    query: Object.fromEntries(u.searchParams),
    method: init?.method ?? "GET",
    body: init?.body ? JSON.parse(String(init.body)) : undefined,
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const api = () => new RestApiAdapter("http://server");

describe("the Map and the Reader", () => {
  it("reads the Map of a stream", async () => {
    const fetchMock = mockFetch(mapSummaryFixture);
    const out = await api().getMapSummary("acme", "proj 1", "release");
    expect(call(fetchMock)).toMatchObject({
      path: "/api/v1/acme/proj%201/map/release",
      method: "GET",
    });
    expect(out.file_rows).toEqual(mapSummaryFixture.file_rows);
  });

  it("reads one item for the Reader by its name, on main when no stream is named", async () => {
    const fetchMock = mockFetch(readerFileFixture);
    const out = await api().getReaderFile("acme", "p1", "docs/release notes.md");
    expect(call(fetchMock)).toMatchObject({
      path: "/api/v1/acme/p1/reader/main",
      query: { item: "docs/release notes.md" },
    });
    expect(out.blocks).toHaveLength(readerFileFixture.blocks.length);
  });

  it("starts the project-wide check and follows its job", async () => {
    const started = mockFetch({ job_id: "job-7" }, 202);
    await expect(api().runProjectChecks("acme", "p1")).resolves.toEqual({ job_id: "job-7" });
    expect(call(started)).toMatchObject({ path: "/api/v1/acme/p1/map/main/check", method: "POST" });

    vi.unstubAllGlobals();
    const job = mockFetch({ id: "job-7", status: "processing", progress: 40, total_blocks: 5 });
    await expect(api().getJob("acme", "job-7")).resolves.toMatchObject({
      status: "processing",
      progress: 40,
    });
    expect(call(job).path).toBe("/api/v1/acme/jobs/job-7");
  });

  it("files one finding's fix and answers with the proposal", async () => {
    const fetchMock = mockFetch({ proposals: ["sp-1"], refused: [] });
    const out = await api().fixFinding("acme", "p1", "docs/a.md", "a1f0-1", "main");
    expect(call(fetchMock)).toMatchObject({
      path: "/api/v1/acme/p1/reader/main/fix",
      method: "POST",
      body: { item: "docs/a.md", finding_id: "a1f0-1" },
    });
    expect(out).toEqual({ proposals: ["sp-1"], refused: [] });
  });

  it("files every fix in an item, with the ones refused", async () => {
    const fetchMock = mockFetch({
      proposals: [],
      refused: [{ finding_id: "c4d5-1", reason: "This finding has no fix." }],
    });
    const out = await api().fixFile("acme", "p1", "docs/a.md");
    expect(call(fetchMock)).toMatchObject({
      path: "/api/v1/acme/p1/reader/main/fix-file",
      body: { item: "docs/a.md" },
    });
    expect(out.refused[0].reason).toBe("This finding has no fix.");
  });
});

describe("Decide", () => {
  it("reads the questions", async () => {
    const fetchMock = mockFetch(rulesDecideViewFixture);
    const out = await api().getDecideView("acme", "p1", "main");
    expect(call(fetchMock).path).toBe("/api/v1/acme/p1/rules/main/decide");
    expect(out.decisions).toHaveLength(rulesDecideViewFixture.decisions.length);
  });

  it("reads what keeping a typed form does", async () => {
    const fetchMock = mockFetch({ consequence: "Sign-in becomes the name in 3 blocks." });
    const out = await api().getDecideConsequence("acme", "p1", "s-1", "Sign-in");
    expect(call(fetchMock)).toMatchObject({
      path: "/api/v1/acme/p1/rules/main/decide/consequence",
      query: { id: "s-1", form: "Sign-in" },
    });
    expect(out).toBe("Sign-in becomes the name in 3 blocks.");
  });

  it("records an answer with the form a kept name writes", async () => {
    const fetchMock = mockFetch({ answer: "keep", kept: 1, dropped: 0, later: 0 });
    const out = await api().decideRule("acme", "p1", {
      id: "s-1",
      answer: "keep",
      form: "Sign-in",
    });
    expect(call(fetchMock)).toMatchObject({
      path: "/api/v1/acme/p1/rules/main/decide",
      method: "POST",
      body: { id: "s-1", answer: "keep", form: "Sign-in" },
    });
    expect(out.kept).toBe(1);
  });

  it("assigns a question, and assigns it to no one with null", async () => {
    const assigned = mockFetch({ id: "s-1", assignee: { kind: "person", name: "member-b" } });
    await api().assignSuggestion("acme", "p1", "s-1", "member-b");
    expect(call(assigned)).toMatchObject({
      path: "/api/v1/acme/p1/rules/main/assign",
      body: { id: "s-1", user_id: "member-b" },
    });

    vi.unstubAllGlobals();
    const cleared = mockFetch({ id: "s-1", assignee: null });
    await expect(api().assignSuggestion("acme", "p1", "s-1", null)).resolves.toEqual({
      id: "s-1",
      assignee: null,
    });
    expect(call(cleared).body).toEqual({ id: "s-1", user_id: null });
  });

  it("reads the rules in force", async () => {
    const fetchMock = mockFetch(keptRulesFixture);
    const out = await api().getKeptRules("acme", "p1");
    expect(call(fetchMock).path).toBe("/api/v1/acme/p1/rules/main/kept");
    expect(out).toEqual(keptRulesFixture);
  });

  it("reads the members who decide in the project", async () => {
    const members = [{ id: "u-ada", name: "Ada Lind", me: true }];
    const fetchMock = mockFetch(members);
    const out = await api().getRuleAssignees("acme", "p1", "main");
    expect(call(fetchMock).path).toBe("/api/v1/acme/p1/rules/main/assignees");
    expect(out).toEqual(members);
  });

  it("reads the workspace's queue", async () => {
    const fetchMock = mockFetch({
      projects: [{ project_id: "p1", project_name: "Fernwell", waiting: 2, assigned: 1 }],
    });
    const out = await api().getContextQueue("acme");
    expect(call(fetchMock).path).toBe("/api/v1/acme/context/queue");
    expect(out.projects[0]).toEqual({
      project_id: "p1",
      project_name: "Fernwell",
      waiting: 2,
      assigned: 1,
    });
  });

  it("reports the server's refusal", async () => {
    mockFetch({ error: "no suggestion s-9 waits for a decision" }, 404);
    await expect(api().decideRule("acme", "p1", { id: "s-9", answer: "drop" })).rejects.toThrow(
      /no suggestion s-9/,
    );
  });
});
