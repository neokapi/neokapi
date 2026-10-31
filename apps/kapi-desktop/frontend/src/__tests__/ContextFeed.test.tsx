import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent, waitFor } from "./testUtils";

import { ContextFeedList, type ContextFeedProps } from "../components/ContextFeed";
import {
  AWAITING_FEED,
  CANDIDATE,
  CONTEXT_FEED,
  EMPTY_FEED,
  IN_FORCE,
  MANY_SESSIONS_FEED,
  OBSERVATION,
  RESET,
  RESET_FEED,
  SECOND_CANDIDATE,
  HOOK_ONLY_SESSION,
  MIXED_SESSION,
  checkRun,
  feedEntry,
  feedGroup,
} from "../stories/fixtures/contextFeed";
import type { ContextFeed } from "../types/api";

const PERSON_DAY = "actor:person/asgeir:2026-09-21";

const SIGN_IN_FINDING = { file: "docs/pricing.md", rule: "terms.vocabulary", quote: "sign in" };

const runResultText = (run: HTMLElement) =>
  run.querySelector("[data-slot='check-run-result']")?.textContent;

function renderFeed(feed: ContextFeed = CONTEXT_FEED, props: Partial<ContextFeedProps> = {}) {
  const onKeep = vi.fn();
  const onDrop = vi.fn();
  const onResetSession = vi.fn();
  const onResetBefore = vi.fn();
  const onWiden = vi.fn();
  const view = render(
    <ContextFeedList
      feed={feed}
      onKeep={onKeep}
      onDrop={onDrop}
      onResetSession={onResetSession}
      onResetBefore={onResetBefore}
      onWiden={onWiden}
      {...props}
    />,
  );
  return { view, onKeep, onDrop, onResetSession, onResetBefore, onWiden };
}

const sessionRow = (id: string) =>
  document.querySelector(`[data-slot='context-feed-session'][data-session='${id}']`) as HTMLElement;
const detail = () => document.querySelector("[data-slot='context-feed-detail']") as HTMLElement;
const entry = (id: string) => document.querySelector(`[data-entry='${id}']`) as HTMLElement;
const selectedSession = () =>
  document.querySelector("[data-slot='context-feed-session'][aria-selected='true']");

describe("History as a list of sessions beside a detail pane", () => {
  it("lists one row per session, newest first, with who worked and how many operations", () => {
    renderFeed();
    const rows = Array.from(document.querySelectorAll("[data-slot='context-feed-session']"));
    expect(rows.map((r) => r.getAttribute("data-session"))).toEqual(["session:sess-1", PERSON_DAY]);
    const first = sessionRow("session:sess-1");
    expect(first.getAttribute("role")).toBe("option");
    expect(first.querySelector("[data-slot='actor-mark']")).not.toBeNull();
    expect(first.querySelector("[data-slot='session-summary']")?.textContent).toBe(
      "claudeon studio",
    );
    expect(first.querySelector("time")).not.toBeNull();
    expect(first.textContent).toMatch(/2$/);
  });

  it("opens on the newest session and shows its operations in the detail", () => {
    renderFeed();
    expect(selectedSession()?.getAttribute("data-session")).toBe("session:sess-1");
    expect(detail().getAttribute("data-session")).toBe("session:sess-1");
    const entries = Array.from(detail().querySelectorAll("[data-slot='context-feed-entry']"));
    expect(entries.map((e) => e.getAttribute("data-entry"))).toEqual([CANDIDATE.id, IN_FORCE.id]);
    expect(entry(CANDIDATE.id).querySelector("[data-slot='kind-icon']")).not.toBeNull();
  });

  it("selects a session by click, by the list's keys and by j and k from the page", () => {
    renderFeed();
    fireEvent.click(sessionRow(PERSON_DAY));
    expect(detail().getAttribute("data-session")).toBe(PERSON_DAY);
    expect(entry(OBSERVATION.id)).not.toBeNull();

    const list = screen.getByRole("listbox", { name: "Sessions" });
    fireEvent.keyDown(list, { key: "ArrowUp" });
    expect(detail().getAttribute("data-session")).toBe("session:sess-1");

    fireEvent.keyDown(window, { key: "j" });
    expect(detail().getAttribute("data-session")).toBe(PERSON_DAY);
    fireEvent.keyDown(window, { key: "k" });
    expect(detail().getAttribute("data-session")).toBe("session:sess-1");
  });

  it("puts a proposal's evidence in a disclosure under its row", () => {
    renderFeed();
    const row = entry(CANDIDATE.id);
    expect(row.querySelector("[data-slot='context-quote']")).toBeNull();
    fireEvent.click(row.querySelector("[data-slot='context-details-toggle']") as HTMLElement);
    expect(row.textContent).toContain("docs/pricing.md");
    expect(row.textContent).toContain("pricing-hero");
    const quote = row.querySelector("[data-slot='context-quote']");
    expect(quote?.textContent).toBe("Please sign in to see your prices.");
    expect(quote?.getAttribute("translate")).toBe("no");
  });

  it("marks a term and a path as the reader's own content", () => {
    renderFeed();
    const row = entry(CANDIDATE.id);
    fireEvent.click(row.querySelector("[data-slot='context-details-toggle']") as HTMLElement);
    const own = Array.from(row.querySelectorAll("[translate='no']")).map((el) => el.textContent);
    expect(own).toContain("sign in");
    expect(own).toContain("docs/pricing.md");
  });

  it("confirms and discards a proposal from the buttons on its row", () => {
    const { onKeep, onDrop } = renderFeed(AWAITING_FEED);
    fireEvent.click(
      entry(CANDIDATE.id).querySelector("[data-slot='keep-suggestion']") as HTMLElement,
    );
    expect(onKeep).toHaveBeenCalledTimes(1);
    expect(onKeep.mock.calls[0][0].id).toBe(CANDIDATE.id);
    expect(onKeep.mock.calls[0][1]).toBeUndefined();

    fireEvent.click(
      entry(SECOND_CANDIDATE.id).querySelector("[data-slot='drop-suggestion']") as HTMLElement,
    );
    expect(onDrop).toHaveBeenCalledTimes(1);
    expect(onDrop.mock.calls[0][0].id).toBe(SECOND_CANDIDATE.id);
  });

  it("keeps and drops with a and r, passing the keys to the next proposal", () => {
    const { onKeep, onDrop } = renderFeed(AWAITING_FEED);
    expect(document.querySelector("[data-active]")?.getAttribute("data-entry")).toBe(CANDIDATE.id);

    fireEvent.keyDown(window, { key: "a" });
    expect(onKeep).toHaveBeenCalledTimes(1);
    expect(onKeep.mock.calls[0][0].id).toBe(CANDIDATE.id);
    expect(document.querySelector("[data-active]")?.getAttribute("data-entry")).toBe(
      SECOND_CANDIDATE.id,
    );

    fireEvent.keyDown(window, { key: "r" });
    expect(onDrop).toHaveBeenCalledTimes(1);
    expect(onDrop.mock.calls[0][0].id).toBe(SECOND_CANDIDATE.id);
  });

  it("gives the keys to the proposal the person clicks", () => {
    const { onKeep } = renderFeed(AWAITING_FEED);
    fireEvent.click(entry(SECOND_CANDIDATE.id));
    expect(document.querySelector("[data-active]")?.getAttribute("data-entry")).toBe(
      SECOND_CANDIDATE.id,
    );
    fireEvent.keyDown(window, { key: "a" });
    expect(onKeep.mock.calls[0][0].id).toBe(SECOND_CANDIDATE.id);
  });

  it("shows the keys on the proposal they act on, and nowhere else", () => {
    renderFeed(AWAITING_FEED);
    const keep = entry(CANDIDATE.id).querySelector("[data-slot='keep-suggestion']");
    expect(keep?.getAttribute("aria-keyshortcuts")).toBe("a");
    expect(keep?.querySelector("kbd")?.textContent).toBe("a");
    const other = entry(SECOND_CANDIDATE.id).querySelector("[data-slot='keep-suggestion']");
    expect(other?.getAttribute("aria-keyshortcuts")).toBeNull();
    expect(other?.querySelector("kbd")).toBeNull();
    expect(document.querySelector("[data-slot='context-feed-shortcuts']")).toBeNull();
  });

  it("dims a decided proposal until the feed says so, and restores it when the decision fails", async () => {
    const onKeep = vi.fn().mockRejectedValue(new Error("refused"));
    renderFeed(AWAITING_FEED, { onKeep });
    fireEvent.keyDown(window, { key: "a" });
    expect(entry(CANDIDATE.id).hasAttribute("data-decided")).toBe(true);
    expect(entry(CANDIDATE.id).querySelector("[data-slot='keep-suggestion']")).toBeNull();
    await waitFor(() => expect(entry(CANDIDATE.id).hasAttribute("data-decided")).toBe(false));
    expect(entry(CANDIDATE.id).querySelector("[data-slot='keep-suggestion']")).not.toBeNull();
  });

  it("edits the replacement before accepting, and hands the edit to the caller", async () => {
    const { onKeep } = renderFeed();
    fireEvent.keyDown(window, { key: "e" });
    const form = await waitFor(() => {
      const el = document.querySelector("[data-slot='context-edit']");
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    const input = form.querySelector("input") as HTMLInputElement;
    expect(input.value).toBe("log in");
    fireEvent.change(input, { target: { value: "sign in" } });
    fireEvent.click(form.querySelector("[data-slot='keep-edited']") as HTMLElement);

    expect(onKeep).toHaveBeenCalledTimes(1);
    expect(onKeep.mock.calls[0][1]).toMatchObject({ replacement: "sign in" });
  });

  it("opens the edit from the Edit button too", () => {
    renderFeed();
    fireEvent.click(
      entry(CANDIDATE.id).querySelector("[data-slot='edit-suggestion']") as HTMLElement,
    );
    expect(entry(CANDIDATE.id).querySelector("[data-slot='context-edit']")).not.toBeNull();
  });

  it("leaves the keys alone while a field has focus", async () => {
    const { onKeep } = renderFeed();
    fireEvent.keyDown(window, { key: "e" });
    const input = await waitFor(() => {
      const el = document.querySelector("[data-slot='context-edit'] input");
      expect(el).not.toBeNull();
      return el as HTMLInputElement;
    });
    fireEvent.keyDown(input, { key: "a" });
    fireEvent.keyDown(input, { key: "j" });
    expect(onKeep).not.toHaveBeenCalled();
    expect(detail().getAttribute("data-session")).toBe("session:sess-1");
  });

  it("offers dropping and widening on a rule in force, and neither on a proposal", () => {
    const { onDrop, onWiden } = renderFeed();
    const confirmed = entry(IN_FORCE.id);
    expect(confirmed.querySelector("[data-slot='keep-suggestion']")).toBeNull();
    fireEvent.click(confirmed.querySelector("[data-slot='drop-rule']") as HTMLElement);
    expect(onDrop).toHaveBeenCalledWith(expect.objectContaining({ id: IN_FORCE.id }));
    expect(onWiden).not.toHaveBeenCalled();

    const candidate = entry(CANDIDATE.id);
    expect(candidate.querySelector("[data-slot='drop-rule']")).toBeNull();
    expect(candidate.querySelector("[data-slot='widen-picker']")).toBeNull();
  });

  it("offers widening on a rule in force", () => {
    renderFeed();
    expect(entry(IN_FORCE.id).querySelector("[data-slot='widen-picker']")).not.toBeNull();
  });

  it("decides on a project with no copy on this machine, and says where the decision lands", () => {
    const feed: ContextFeed = {
      ...CONTEXT_FEED,
      groups: [
        feedGroup({
          id: "session:sess-1",
          session: "sess-1",
          recipe: undefined,
          entries: [feedEntry({ ...CANDIDATE, recipe: undefined })],
        }),
      ],
    };
    renderFeed(feed);
    expect(document.querySelector("[data-slot='keep-suggestion']")).not.toBeNull();
    expect(document.querySelectorAll("[data-slot='context-no-checkout']")).toHaveLength(1);
    expect(document.querySelector("[data-slot='context-no-checkout']")?.textContent).toContain(
      "No copy of this project is on this machine. A decision is recorded in the workspace",
    );
  });

  it("reads a session that only ran the check as its check counts, with no operations", () => {
    renderFeed(
      {
        ...CONTEXT_FEED,
        groups: [
          feedGroup({
            id: "session:hook",
            session: "hook",
            files: 1,
            checked: 2,
            failing: 1,
            failed: true,
            entries: [],
          }),
          ...CONTEXT_FEED.groups,
        ],
      },
      { defaultSelected: "session:hook" },
    );
    // Who worked and when, and no count of operations after them.
    const hook = sessionRow("session:hook");
    expect(hook.textContent?.endsWith(hook.querySelector("time")?.textContent ?? "-")).toBe(true);
    expect(detail().querySelector("[data-slot='session-counts']")?.textContent).toBe(
      "1 file · 2 checks · 1 failing · 0 caught",
    );
    expect(detail().querySelectorAll("[data-slot='context-feed-entry']")).toHaveLength(0);
  });

  it("reads out a session's check counts after its suggestions", () => {
    renderFeed({
      ...CONTEXT_FEED,
      groups: [
        { ...CONTEXT_FEED.groups[0], files: 2, checked: 3, caught: 1, failed: true },
        ...CONTEXT_FEED.groups.slice(1),
      ],
    });
    expect(detail().querySelector("[data-slot='session-counts']")?.textContent).toBe(
      "2 suggestions, 1 kept · 2 files · 3 checks · 1 caught",
    );
  });

  it("lists a session's check runs under its counts, newest first", () => {
    renderFeed(
      { ...CONTEXT_FEED, groups: [HOOK_ONLY_SESSION, ...CONTEXT_FEED.groups] },
      { defaultSelected: HOOK_ONLY_SESSION.id },
    );
    expect(detail().querySelector("[data-slot='session-counts']")?.textContent).toBe(
      "2 files · 3 checks · 2 caught",
    );
    const runs = Array.from(
      detail().querySelectorAll<HTMLElement>("[data-slot='context-check-run']"),
    );
    expect(runs.map((r) => r.getAttribute("data-run"))).toEqual(["run-3", "run-2", "run-1"]);
    expect(runs.map((r) => r.getAttribute("data-result"))).toEqual(["clean", "failing", "failing"]);
    expect(runs.map(runResultText)).toEqual([
      "clean",
      "1 failingterms.vocabulary",
      "2 failingterms.vocabulary",
    ]);
    expect(runs.map((r) => r.querySelector("[data-slot='check-run-files']")?.textContent)).toEqual([
      "pricing.md",
      "pricing.md",
      "pricing.md, welcome.md",
    ]);
    expect(runs[0].querySelector("[data-slot='check-run-findings']")).toBeNull();
    expect(detail().querySelectorAll("[data-slot='context-feed-entry']")).toHaveLength(0);
  });

  it("marks each run with the hook that ran it", () => {
    renderFeed(
      { ...CONTEXT_FEED, groups: [HOOK_ONLY_SESSION, ...CONTEXT_FEED.groups] },
      { defaultSelected: HOOK_ONLY_SESSION.id },
    );
    const runs = Array.from(
      detail().querySelectorAll<HTMLElement>("[data-slot='context-check-run']"),
    );
    expect(runs.map((r) => r.getAttribute("data-hook"))).toEqual(["stop", "edit", "stop"]);
    expect(
      runs.map((r) => r.querySelector("[data-slot='operation-mark']")?.getAttribute("aria-label")),
    ).toEqual(["checked", "checked on edit", "checked"]);
  });

  it("marks a finding the next run no longer found as caught, on the run that last reported it", () => {
    renderFeed(
      { ...CONTEXT_FEED, groups: [HOOK_ONLY_SESSION, ...CONTEXT_FEED.groups] },
      { defaultSelected: HOOK_ONLY_SESSION.id },
    );
    const findings = (id: string) =>
      Array.from(
        detail().querySelectorAll<HTMLElement>(
          `[data-run='${id}'] [data-slot='check-run-finding']`,
        ),
      ).map((f) => f.textContent);
    expect(findings("run-2")).toEqual(["“sign in”terms.vocabularypricing.mdcaught"]);
    expect(findings("run-1")).toEqual([
      "“sign in”terms.vocabularypricing.md",
      "“utilise”terms.vocabularywelcome.mdcaught",
    ]);
    expect(detail().querySelectorAll("[data-slot='finding-caught']")).toHaveLength(2);
  });

  it("lists a session's operations and check runs in one list by time", () => {
    renderFeed(
      { ...CONTEXT_FEED, groups: [MIXED_SESSION, ...CONTEXT_FEED.groups.slice(1)] },
      { defaultSelected: MIXED_SESSION.id },
    );
    const rows = Array.from(
      detail().querySelectorAll<HTMLElement>(
        "[data-slot='context-feed-entries'] > li > [data-slot='context-feed-entry'], [data-slot='context-feed-entries'] > li > [data-slot='context-check-run']",
      ),
    );
    expect(
      rows.map((r) => r.getAttribute("data-entry") ?? `run:${r.getAttribute("data-run")}`),
    ).toEqual([CANDIDATE.id, "run:run-b", IN_FORCE.id, "run:run-a"]);
    // The proposal keeps its decisions beside the runs.
    expect(entry(CANDIDATE.id).querySelector("[data-slot='keep-suggestion']")).not.toBeNull();
    expect(detail().querySelector("[data-slot='session-counts']")?.textContent).toBe(
      "2 suggestions, 1 kept · 1 file · 2 checks · 1 caught",
    );
  });

  it("says a run that reached no verdict did not run, and counts the findings it does not list", () => {
    renderFeed(
      {
        ...CONTEXT_FEED,
        groups: [
          feedGroup({
            id: "session:hook",
            session: "hook",
            checked: 2,
            failed: true,
            failing: 30,
            entries: [],
            runs: [
              checkRun({
                id: "run-big",
                at: "2026-09-21T09:10:00Z",
                edited: false,
                files: [],
                file_count: 140,
                clean: false,
                failing: 30,
                rules: ["terms.vocabulary"],
                findings: [{ ...SIGN_IN_FINDING, caught: false }],
              }),
              checkRun({
                id: "run-none",
                at: "2026-09-21T09:00:00Z",
                reached: false,
                clean: false,
              }),
            ],
          }),
        ],
      },
      { defaultSelected: "session:hook" },
    );
    const big = detail().querySelector<HTMLElement>("[data-run='run-big']")!;
    expect(big.querySelector("[data-slot='check-run-files']")?.textContent).toBe("140 files");
    expect(
      big.querySelector("[data-slot='check-run-findings']")?.lastElementChild?.textContent,
    ).toBe("29 more");
    const none = detail().querySelector<HTMLElement>("[data-run='run-none']")!;
    expect(runResultText(none)).toBe("did not run");
    expect(none.getAttribute("data-result")).toBe("did-not-run");
  });

  it("reads out what the selected session did, and says who worked and where", () => {
    renderFeed();
    const pane = detail();
    expect(pane.querySelector("[data-slot='session-counts']")?.textContent).toContain(
      "2 suggestions",
    );
    const title = pane.querySelector("[data-slot='list-detail-toolbar']") as HTMLElement;
    expect(title.textContent).toContain("claude");
    expect(title.textContent).toContain("studio");
    expect(pane.querySelector("[data-slot='session-awaiting']")?.textContent).toContain(
      "1 suggested",
    );
    expect(
      sessionRow("session:sess-1").querySelector("[data-slot='session-awaiting-mark']"),
    ).not.toBeNull();
  });

  it("puts the reset to before an agent session in the toolbar, as a hand-back", () => {
    const { onResetSession } = renderFeed();
    const toolbar = detail().querySelector("[data-slot='list-detail-toolbar']") as HTMLElement;
    const reset = toolbar.querySelector("[data-slot='reset-session']") as HTMLElement;
    expect(reset.getAttribute("data-variant")).toBe("warning");
    fireEvent.click(reset);
    expect(onResetSession).toHaveBeenCalledWith(expect.objectContaining({ id: "session:sess-1" }));
  });

  it("offers no reset for a person's day", () => {
    renderFeed(CONTEXT_FEED, { defaultSelected: PERSON_DAY });
    expect(detail().getAttribute("data-session")).toBe(PERSON_DAY);
    expect(document.querySelector("[data-slot='reset-session']")).toBeNull();
  });

  it("says nothing has been recorded rather than showing an empty list", () => {
    renderFeed(EMPTY_FEED);
    expect(screen.getByText("Nothing recorded yet")).toBeInTheDocument();
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("says older operations are cut off below the list", () => {
    renderFeed(MANY_SESSIONS_FEED);
    expect(document.querySelectorAll("[data-slot='context-feed-session']")).toHaveLength(40);
    expect(screen.getByText(/Older operations are not shown/)).toBeInTheDocument();
  });
});

describe("History through a refetch", () => {
  const callbacks = {
    onKeep: vi.fn(),
    onDrop: vi.fn(),
    onResetSession: vi.fn(),
    onWiden: vi.fn(),
  };

  it("keeps the selected session selected when new operations arrive", () => {
    const { view } = renderFeed(CONTEXT_FEED, { defaultSelected: PERSON_DAY });
    expect(selectedSession()?.getAttribute("data-session")).toBe(PERSON_DAY);

    // Another process recorded a new session while this was on screen: a new
    // feed arrives with it at the top.
    view.rerender(
      <ContextFeedList
        {...callbacks}
        feed={{
          ...CONTEXT_FEED,
          groups: [
            feedGroup({
              id: "session:sess-9",
              session: "sess-9",
              entries: [feedEntry({ ...CANDIDATE, id: "40" })],
            }),
            ...CONTEXT_FEED.groups,
          ],
        }}
      />,
    );
    expect(document.querySelectorAll("[data-slot='context-feed-session']")).toHaveLength(3);
    expect(selectedSession()?.getAttribute("data-session")).toBe(PERSON_DAY);
    expect(detail().getAttribute("data-session")).toBe(PERSON_DAY);
  });

  it("falls back to the newest session when the selected one is gone", () => {
    const { view } = renderFeed(CONTEXT_FEED, { defaultSelected: PERSON_DAY });
    view.rerender(
      <ContextFeedList
        {...callbacks}
        feed={{ ...CONTEXT_FEED, groups: CONTEXT_FEED.groups.slice(0, 1) }}
      />,
    );
    expect(selectedSession()?.getAttribute("data-session")).toBe("session:sess-1");
  });

  it("keeps an open edit through a refresh that leaves the proposal standing", async () => {
    const { view } = renderFeed();
    fireEvent.keyDown(window, { key: "e" });
    await waitFor(() => {
      expect(document.querySelector("[data-slot='context-edit']")).not.toBeNull();
    });
    const input = document.querySelector("[data-slot='context-edit'] input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "half typed" } });

    view.rerender(
      <ContextFeedList
        {...callbacks}
        feed={{
          ...CONTEXT_FEED,
          groups: [
            feedGroup({
              id: "session:sess-1",
              session: "sess-1",
              entries: [feedEntry({ ...CANDIDATE, id: "14" }), CANDIDATE, IN_FORCE],
            }),
          ],
        }}
      />,
    );
    const still = document.querySelector("[data-slot='context-edit'] input") as HTMLInputElement;
    expect(still).not.toBeNull();
    expect(still.value).toBe("half typed");
  });
});

describe("taking a reset back", () => {
  it("puts Take this reset back in the toolbar of the session that holds the reset", () => {
    const { onResetBefore } = renderFeed(RESET_FEED);
    expect(detail().getAttribute("data-session")).toBe("actor:person/asgeir:2026-09-22");
    const take = detail().querySelector(
      "[data-slot='list-detail-toolbar'] [data-slot='reset-reset']",
    ) as HTMLElement;
    expect(take.textContent?.trim()).toBe("Take this reset back");
    fireEvent.click(take);
    expect(onResetBefore).toHaveBeenCalledWith(RESET);
  });

  it("offers it on a session with a reset only", () => {
    renderFeed(RESET_FEED);
    expect(document.querySelectorAll("[data-slot='reset-reset']")).toHaveLength(1);
    fireEvent.click(sessionRow("session:sess-1"));
    expect(document.querySelector("[data-slot='reset-reset']")).toBeNull();
  });

  it("offers nothing in a workspace open for reading only", () => {
    renderFeed({ ...RESET_FEED, read_only: true });
    expect(document.querySelector("[data-slot='reset-reset']")).toBeNull();
    expect(screen.getByText(/open for reading only/)).toBeInTheDocument();
  });

  it("counts the reset in its session's detail", () => {
    renderFeed(RESET_FEED);
    expect(detail().querySelector("[data-slot='session-counts']")?.textContent).toBe("1 reset");
    expect(entry(RESET.id).querySelector("[data-slot='operation-mark']")).not.toBeNull();
  });
});
