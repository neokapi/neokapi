import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "./testUtils";

import { ContextFeedList } from "../components/ContextFeed";
import {
  CANDIDATE,
  CONTEXT_FEED,
  EMPTY_FEED,
  IN_FORCE,
  feedEntry,
  feedGroup,
} from "../stories/fixtures/contextFeed";
import type { ContextFeed } from "../types/api";

function renderFeed(feed: ContextFeed = CONTEXT_FEED) {
  const onKeep = vi.fn();
  const onDrop = vi.fn();
  const onRevert = vi.fn();
  const onRevertSession = vi.fn();
  const onWiden = vi.fn();
  const view = render(
    <ContextFeedList
      feed={feed}
      onKeep={onKeep}
      onDrop={onDrop}
      onRevert={onRevert}
      onRevertSession={onRevertSession}
      onWiden={onWiden}
    />,
  );
  return { view, onKeep, onDrop, onRevert, onRevertSession, onWiden };
}

describe("the feed of recorded context operations", () => {
  it("shows a candidate's evidence without anyone opening anything", () => {
    renderFeed();
    const entry = document.querySelector(`[data-entry='${CANDIDATE.id}']`);
    expect(entry).not.toBeNull();
    expect(entry?.textContent).toContain("docs/pricing.md");
    expect(entry?.textContent).toContain("pricing-hero");
    expect(entry?.querySelector("[data-slot='context-quote']")?.textContent).toBe(
      "Please sign in to see your prices.",
    );
  });

  it("marks a term, a quotation and a path as the reader's own content", () => {
    renderFeed();
    const entry = document.querySelector(`[data-entry='${CANDIDATE.id}']`);
    const quote = entry?.querySelector("[data-slot='context-quote']");
    expect(quote?.getAttribute("translate")).toBe("no");
    const term = Array.from(entry?.querySelectorAll("[translate='no']") ?? []).map(
      (el) => el.textContent,
    );
    expect(term).toContain("sign in");
    expect(term).toContain("docs/pricing.md");
  });

  it("confirms and discards a candidate from its buttons", () => {
    const { onKeep, onDrop } = renderFeed();
    const entry = document.querySelector(`[data-entry='${CANDIDATE.id}']`) as HTMLElement;
    fireEvent.click(entry.querySelector("[data-slot='keep-suggestion']") as HTMLElement);
    expect(onKeep).toHaveBeenCalledTimes(1);
    expect(onKeep.mock.calls[0][0].id).toBe(CANDIDATE.id);
    expect(onKeep.mock.calls[0][1]).toBeUndefined();

    fireEvent.click(entry.querySelector("[data-slot='drop-suggestion']") as HTMLElement);
    expect(onDrop).toHaveBeenCalledTimes(1);
  });

  it("confirms with the keyboard, the way the review session does", () => {
    const { onKeep, onDrop } = renderFeed();
    fireEvent.keyDown(window, { key: "a" });
    expect(onKeep).toHaveBeenCalledTimes(1);
    expect(onKeep.mock.calls[0][0].id).toBe(CANDIDATE.id);

    fireEvent.keyDown(window, { key: "r" });
    expect(onDrop).toHaveBeenCalledTimes(1);
    expect(onDrop.mock.calls[0][0].id).toBe(CANDIDATE.id);
  });

  it("moves the cursor with j and k over the candidates alone", () => {
    const second = feedEntry({
      ...CANDIDATE,
      id: "13",
      subject: { ...CANDIDATE.subject, term: "utilise" },
    });
    const feed: ContextFeed = {
      ...CONTEXT_FEED,
      groups: [
        feedGroup({
          id: "session:sess-1",
          session: "sess-1",
          entries: [CANDIDATE, second, IN_FORCE],
        }),
      ],
    };
    const { onKeep } = renderFeed(feed);
    expect(document.querySelector("[data-active]")?.getAttribute("data-entry")).toBe(CANDIDATE.id);

    fireEvent.keyDown(window, { key: "j" });
    expect(document.querySelector("[data-active]")?.getAttribute("data-entry")).toBe("13");
    fireEvent.keyDown(window, { key: "k" });
    expect(document.querySelector("[data-active]")?.getAttribute("data-entry")).toBe(CANDIDATE.id);

    fireEvent.keyDown(window, { key: "j" });
    fireEvent.keyDown(window, { key: "a" });
    expect(onKeep.mock.calls[0][0].id).toBe("13");
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

  it("leaves the keys alone while a field has focus", async () => {
    const { onKeep } = renderFeed();
    fireEvent.keyDown(window, { key: "e" });
    const input = await waitFor(() => {
      const el = document.querySelector("[data-slot='context-edit'] input");
      expect(el).not.toBeNull();
      return el as HTMLInputElement;
    });
    fireEvent.keyDown(input, { key: "a" });
    expect(onKeep).not.toHaveBeenCalled();
  });

  it("offers undo and widening on a rule in force, and neither on a candidate", () => {
    const { onRevert, onWiden } = renderFeed();
    const confirmed = document.querySelector(`[data-entry='${IN_FORCE.id}']`) as HTMLElement;
    expect(confirmed.querySelector("[data-slot='keep-suggestion']")).toBeNull();
    fireEvent.click(confirmed.querySelector("[data-slot='revert-entry']") as HTMLElement);
    expect(onRevert).toHaveBeenCalledTimes(1);
    expect(confirmed.querySelector("[data-slot='widen-picker']")).not.toBeNull();
    expect(onWiden).not.toHaveBeenCalled();

    const candidate = document.querySelector(`[data-entry='${CANDIDATE.id}']`) as HTMLElement;
    expect(candidate.querySelector("[data-slot='revert-entry']")).toBeNull();
  });

  it("offers no decision for a project with no copy on this machine", () => {
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
    expect(document.querySelector("[data-slot='keep-suggestion']")).toBeNull();
    expect(document.querySelector("[data-slot='context-undecidable']")?.textContent).toContain(
      "No copy of this project is on this machine",
    );
  });

  it("reads out what a session did, and says who worked and where", () => {
    renderFeed();
    const session = document.querySelector("[data-session='session:sess-1']") as HTMLElement;
    expect(session.querySelector("[data-slot='session-summary']")?.textContent).toContain(
      "2 recorded",
    );
    expect(session.textContent).toContain("claude");
    expect(session.textContent).toContain("studio");
    expect(session.querySelector("[data-slot='session-awaiting']")?.textContent).toContain(
      "1 awaiting you",
    );
  });

  it("offers undoing a whole session only where something is in force", () => {
    const { onRevertSession } = renderFeed();
    const session = document.querySelector("[data-session='session:sess-1']") as HTMLElement;
    fireEvent.click(session.querySelector("[data-slot='revert-session']") as HTMLElement);
    expect(onRevertSession).toHaveBeenCalledTimes(1);

    const day = document.querySelector(
      "[data-session='actor:person/asgeir:2026-09-21']",
    ) as HTMLElement;
    expect(day.querySelector("[data-slot='revert-session']")).toBeNull();
  });

  it("says nothing has been recorded rather than showing an empty list", () => {
    renderFeed(EMPTY_FEED);
    expect(screen.getByText("Nothing recorded yet")).toBeInTheDocument();
    expect(document.querySelector("[data-slot='context-feed-shortcuts']")).toBeNull();
  });

  it("keeps an open edit through a refresh that leaves the candidate standing", async () => {
    const { view } = renderFeed();
    fireEvent.keyDown(window, { key: "e" });
    await waitFor(() => {
      expect(document.querySelector("[data-slot='context-edit']")).not.toBeNull();
    });
    const input = document.querySelector("[data-slot='context-edit'] input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "half typed" } });

    // Another process recorded something while the form was open: the same
    // feed arrives with one more operation in it.
    view.rerender(
      <ContextFeedList
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
        onKeep={vi.fn()}
        onDrop={vi.fn()}
        onRevert={vi.fn()}
        onRevertSession={vi.fn()}
        onWiden={vi.fn()}
      />,
    );
    const still = document.querySelector("[data-slot='context-edit'] input") as HTMLInputElement;
    expect(still).not.toBeNull();
    expect(still.value).toBe("half typed");
  });
});
