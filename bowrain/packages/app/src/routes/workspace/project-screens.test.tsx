// @vitest-environment jsdom
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ApiAdapter, JobState, PermissionName, ReaderFixResult } from "@neokapi/ui";
import type { RulesDecideView } from "@neokapi/project-screens";
import { DECIDE_IDS, DECIDE_VIEW, READER_WITH_FIXES } from "@neokapi/project-screens/fixtures";
import { ProjectMapPage } from "./project-map";
import { ProjectReaderPage } from "./project-reader";
import { ProjectRulesPage } from "./project-rules";
import { FERNWELL_MEMBERS, fixtureScreensPort } from "./project-screens-fixture-port";
import {
  assignableMembers,
  bowrainOpens,
  permittedPort,
  proposalsByFinding,
  restScreensPort,
} from "./project-screens-port";
import { rulesSearch } from "./review-inbox";

/** What a member holding exactly `held` may do. */
const holding =
  (...held: PermissionName[]) =>
  (p: PermissionName) =>
    held.includes(p);

const MEMBER = holding("view_content", "review", "manage_terms");
const VIEWER = holding();

describe("the Map route", () => {
  it("offers the project-wide check to a member who may review", async () => {
    const runChecks = vi.fn(async () => {});
    render(
      <ProjectMapPage
        port={permittedPort(fixtureScreensPort({ runChecks }), holding("view_content", "review"))}
        projectName="Fernwell"
        navigate={vi.fn()}
      />,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Run on everything" }));
    expect(runChecks).toHaveBeenCalled();
  });

  it("draws no check action for a member who may only view", async () => {
    render(
      <ProjectMapPage
        port={permittedPort(fixtureScreensPort(), holding("view_content"))}
        projectName="Fernwell"
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("heading", { name: "Fernwell" });
    expect(screen.queryByRole("button", { name: "Run on everything" })).toBeNull();
  });

  it("opens a file in the Reader by its path", async () => {
    const navigate = vi.fn();
    render(
      <ProjectMapPage
        port={permittedPort(fixtureScreensPort(), MEMBER)}
        projectName="Fernwell"
        navigate={navigate}
      />,
    );
    const open = await screen.findAllByRole("button", { name: "Open" });
    await userEvent.click(open[0]);
    expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ kind: "reader" }));
  });
});

describe("the Reader route", () => {
  it("files a fix as a proposal and shows its id under the finding", async () => {
    const { container } = render(
      <ProjectReaderPage
        port={permittedPort(fixtureScreensPort(), MEMBER)}
        projectId="p1"
        path="docs/billing.md"
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("heading", { name: "docs/billing.md" });
    const fix = screen.getAllByRole("button", { name: "Fix" });
    await userEvent.click(fix[0]);
    await waitFor(() =>
      expect(container.querySelector("[data-slot='reader-proposed']")).toHaveTextContent(
        "Proposed",
      ),
    );
  });

  it("draws no Fix for a member who may not file a proposal", async () => {
    render(
      <ProjectReaderPage
        port={permittedPort(fixtureScreensPort(), holding("view_content"))}
        projectId="p1"
        path="docs/billing.md"
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("heading", { name: "docs/billing.md" });
    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
  });

  it("goes back to the Map", async () => {
    const navigate = vi.fn();
    render(
      <ProjectReaderPage
        port={fixtureScreensPort()}
        projectId="p1"
        path="docs/billing.md"
        navigate={navigate}
      />,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Map" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "map" });
  });
});

describe("the Rules route", () => {
  it("offers Keep to a member who governs rules, and lists the kept rules", async () => {
    render(
      <ProjectRulesPage port={permittedPort(fixtureScreensPort(), MEMBER)} navigate={vi.fn()} />,
    );
    expect(await screen.findByRole("button", { name: /^Keep/ })).toBeInTheDocument();
    const kept = await screen.findByRole("region", { name: "Kept by" });
    expect(within(kept).getAllByRole("listitem").length).toBeGreaterThan(0);
  });

  it("draws no Keep, Not a rule or Later for a member who governs no rule", async () => {
    render(
      <ProjectRulesPage
        port={permittedPort(fixtureScreensPort(), holding("view_content", "review"))}
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("listbox", { name: "Suggestions to decide" });
    expect(screen.queryByRole("button", { name: /^Keep/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Not a rule/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Later/ })).toBeNull();
  });
});

describe("the Rules route's team queue", () => {
  // The name is assigned to the reader, the word to a teammate.
  const ASSIGNED: RulesDecideView = {
    ...DECIDE_VIEW,
    decisions: DECIDE_VIEW.decisions.map((d) => {
      if (d.id === DECIDE_IDS.quickcast) {
        return { ...d, assignee: { kind: "person", name: "Ada Lind" }, assignee_id: "u-ada" };
      }
      if (d.id === DECIDE_IDS.business) {
        return { ...d, assignee: { kind: "person", name: "Mira Holt" }, assignee_id: "u-mira" };
      }
      return d;
    }),
  };
  const assignees = (container: HTMLElement) =>
    Array.from(container.querySelectorAll("[data-slot='decide-assignee']")).map(
      (el) => el.textContent,
    );

  it("shows who holds each question and assigns one to a member", async () => {
    const assign = vi.fn(async () => {});
    const { container } = render(
      <ProjectRulesPage
        port={permittedPort(
          fixtureScreensPort({ decideView: async () => ASSIGNED, assign }),
          MEMBER,
        )}
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("listbox", { name: "Suggestions to decide" });
    expect(assignees(container)).toEqual(["–", "Ada Lind", "Mira Holt", "–"]);
    await userEvent.click(await screen.findByRole("combobox", { name: "Assigned to" }));
    await userEvent.click(await screen.findByRole("option", { name: "Mira Holt" }));
    expect(assign).toHaveBeenCalledWith(DECIDE_IDS.store, "u-mira");
  });

  it("opens on the questions assigned to the member", async () => {
    render(
      <ProjectRulesPage
        port={permittedPort(fixtureScreensPort({ decideView: async () => ASSIGNED }), MEMBER)}
        assignedToMe
        navigate={vi.fn()}
      />,
    );
    const list = await screen.findByRole("listbox", { name: "Suggestions to decide" });
    await waitFor(() =>
      expect(
        within(list)
          .getAllByRole("option")
          .map((o) => o.getAttribute("data-id")),
      ).toEqual([DECIDE_IDS.quickcast]),
    );
    expect(screen.getByRole("button", { name: /Assigned to me/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("draws no assignment for a member who governs no rule", async () => {
    const { container } = render(
      <ProjectRulesPage
        port={permittedPort(
          fixtureScreensPort({ decideView: async () => ASSIGNED }),
          holding("view_content", "review"),
        )}
        assignedToMe
        navigate={vi.fn()}
      />,
    );
    const list = await screen.findByRole("listbox", { name: "Suggestions to decide" });
    expect(within(list).getAllByRole("option")).toHaveLength(4);
    expect(assignees(container)).toEqual([]);
    expect(screen.queryByRole("combobox", { name: "Assigned to" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Assigned to me/ })).toBeNull();
  });

  it("offers every member but a viewer, the signed-in one marked", () => {
    const user = (id: string, name: string) => ({
      id,
      name,
      email: `${id}@fernwell.test`,
      avatar_url: "",
    });
    expect(
      assignableMembers(
        [
          { user_id: "u-ada", workspace_id: "ws", role: "admin", user: user("u-ada", "Ada Lind") },
          { user_id: "u-mira", workspace_id: "ws", role: "member", user: user("u-mira", " ") },
          { user_id: "u-viv", workspace_id: "ws", role: "viewer", user: user("u-viv", "Viv") },
          { user_id: "u-gone", workspace_id: "ws", role: "member" },
        ],
        "u-ada",
      ),
    ).toEqual([
      { id: "u-ada", name: "Ada Lind", me: true },
      { id: "u-mira", name: "u-mira@fernwell.test", me: undefined },
      { id: "u-gone", name: "u-gone", me: undefined },
    ]);
  });

  it("keeps the members from a member who assigns nothing", async () => {
    const port = permittedPort(fixtureScreensPort(), holding("view_content"));
    expect(port.members).toBeUndefined();
    await expect(permittedPort(fixtureScreensPort(), MEMBER).members?.()).resolves.toEqual(
      FERNWELL_MEMBERS,
    );
  });
});

describe("the review inbox's way to Rules", () => {
  const rules = [
    { projectId: "p1", projectName: "Fernwell", stream: "main", waiting: 4, assigned: 2 },
    { projectId: "p2", projectName: "Kapimart", stream: "main", waiting: 3, assigned: 0 },
  ];

  it("opens on the questions assigned to the member where they hold any", () => {
    expect(rulesSearch(rules, "p1")).toEqual({ assigned: "me" });
  });

  it("opens on every question where they hold none", () => {
    expect(rulesSearch(rules, "p2")).toEqual({});
    expect(rulesSearch(rules, "p3")).toEqual({});
  });
});

describe("the port", () => {
  it("opens the Map, the Reader, Rules, the kept rules and the Source content page", () => {
    expect(bowrainOpens({ kind: "map" })).toBe(true);
    expect(bowrainOpens({ kind: "reader", path: "a.md" })).toBe(true);
    expect(bowrainOpens({ kind: "rules" })).toBe(true);
    expect(bowrainOpens({ kind: "kept" })).toBe(true);
    expect(bowrainOpens({ kind: "blocks" })).toBe(true);
    expect(bowrainOpens({ kind: "checks" })).toBe(false);
    expect(bowrainOpens({ kind: "setup" })).toBe(false);
    expect(bowrainOpens({ kind: "connect-assistant" })).toBe(false);
  });

  it("sets each proposal of a whole-file fix on the findings of its block", () => {
    // READER_WITH_FIXES has two fixable findings, both on block p2.
    const result: ReaderFixResult = { proposals: ["sp-1"], refused: [] };
    expect(proposalsByFinding(READER_WITH_FIXES, result)).toEqual([
      { finding_id: "a1f0-1", proposal: "sp-1" },
      { finding_id: "b2c3-1", proposal: "sp-1" },
    ]);
  });

  it("skips a block the server refused", () => {
    const result: ReaderFixResult = {
      proposals: [],
      refused: [{ finding_id: "b2c3-1", reason: "The change was refused." }],
    };
    expect(proposalsByFinding(READER_WITH_FIXES, result)).toEqual([]);
  });
});

describe("who may answer a question", () => {
  it("counts the content memory permission the server reports as manage_memory", () => {
    expect(permittedPort(fixtureScreensPort(), holding("manage_memory")).decide).toBeDefined();
    expect(permittedPort(fixtureScreensPort(), VIEWER).decide).toBeUndefined();
  });

  it("shows the server's refusal in the question's pane", async () => {
    // A voice-only member sees Keep on a name; the server refuses it, since a
    // name takes the terms permission.
    const decide = vi.fn(async () => {
      throw new Error("deciding a term needs the manage_terms permission");
    });
    const { container } = render(
      <ProjectRulesPage
        port={permittedPort(fixtureScreensPort({ decide }), holding("manage_voice"))}
        navigate={vi.fn()}
      />,
    );
    await userEvent.click(await screen.findByRole("button", { name: /^Keep/ }));
    await waitFor(() =>
      expect(container.querySelector("[data-slot='decide-refused']")).toHaveTextContent(
        "deciding a term needs the manage_terms permission",
      ),
    );
  });
});

describe("following the check's job", () => {
  /** An adapter whose check starts job-1 and whose job answers each status in turn. */
  function jobAdapter(statuses: JobState["status"][]) {
    const getJob = vi.fn(async (_ws: string, id: string) => ({
      id,
      status: statuses.length > 1 ? statuses.shift()! : statuses[0],
      progress: 0,
    }));
    const api = {
      runProjectChecks: vi.fn(async () => ({ job_id: "job-1" })),
      getJob,
    } as unknown as ApiAdapter;
    return { api, getJob };
  }
  const scope = { ws: "acme", projectId: "p1", stream: "main" };

  afterEach(() => {
    vi.useRealTimers();
  });

  it("waits longer between reads, up to 5 s, and reads the Map again when the job completes", async () => {
    vi.useFakeTimers();
    const { api, getJob } = jobAdapter([
      "queued",
      "processing",
      "processing",
      "processing",
      "processing",
      "completed",
    ]);
    const port = restScreensPort(api, scope);
    const changed = vi.fn();
    port.subscribe!(changed);
    const done = port.runChecks!();
    for (const ms of [1000, 2000, 4000, 5000, 5000]) {
      await vi.advanceTimersByTimeAsync(ms - 1);
      const before = getJob.mock.calls.length;
      await vi.advanceTimersByTimeAsync(1);
      expect(getJob.mock.calls.length).toBe(before + 1);
    }
    await done;
    expect(getJob).toHaveBeenCalledTimes(6);
    expect(changed).toHaveBeenCalledWith("map");
  });

  it("gives up after 10 minutes", async () => {
    vi.useFakeTimers();
    const { api } = jobAdapter(["processing"]);
    const done = restScreensPort(api, scope).runChecks!();
    const failed = expect(done).rejects.toThrow("The check did not finish");
    await vi.advanceTimersByTimeAsync(10 * 60 * 1000 + 5000);
    await failed;
  });

  it("reports a failed job with the job's own error", async () => {
    const { api } = jobAdapter(["failed"]);
    (api.getJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: "job-1",
      status: "failed",
      progress: 0,
      error: "the stream holds no items",
    });
    await expect(restScreensPort(api, scope).runChecks!()).rejects.toThrow(
      "the stream holds no items",
    );
  });

  it("stops reading the job once disposed", async () => {
    vi.useFakeTimers();
    const { api, getJob } = jobAdapter(["processing"]);
    const port = restScreensPort(api, scope);
    const changed = vi.fn();
    port.subscribe!(changed);
    const done = port.runChecks!();
    await vi.advanceTimersByTimeAsync(0);
    expect(getJob).toHaveBeenCalledTimes(1);
    port.dispose();
    await done;
    await vi.advanceTimersByTimeAsync(60_000);
    expect(getJob).toHaveBeenCalledTimes(1);
    expect(changed).not.toHaveBeenCalled();
  });
});

describe("disposing the port", () => {
  const scope = { ws: "acme", projectId: "p1", stream: "main" };

  afterEach(() => {
    vi.useRealTimers();
  });

  it("schedules no wait when the job read in flight answers after dispose", async () => {
    vi.useFakeTimers();
    let answer: (v: JobState) => void = () => {};
    const getJob = vi.fn(
      () =>
        new Promise<JobState>((resolve) => {
          answer = resolve;
        }),
    );
    const api = {
      runProjectChecks: vi.fn(async () => ({ job_id: "job-1" })),
      getJob,
    } as unknown as ApiAdapter;
    const port = restScreensPort(api, scope);
    const done = port.runChecks!();
    await vi.advanceTimersByTimeAsync(0);
    port.dispose();
    answer({ id: "job-1", status: "processing", progress: 0 });
    await done;
    expect(vi.getTimerCount()).toBe(0);
    expect(getJob).toHaveBeenCalledTimes(1);
  });

  it("follows a job again after resume, as a StrictMode remount does", async () => {
    const api = {
      runProjectChecks: vi.fn(async () => ({ job_id: "job-1" })),
      getJob: vi.fn(async () => ({ id: "job-1", status: "completed", progress: 100 })),
    } as unknown as ApiAdapter;
    const port = restScreensPort(api, scope);
    port.dispose();
    port.resume();
    const changed = vi.fn();
    port.subscribe!(changed);
    await port.runChecks!();
    expect(changed).toHaveBeenCalledWith("map");
  });
});
