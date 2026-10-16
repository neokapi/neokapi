import { describe, it, expect, vi, beforeEach } from "vite-plus/test";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import {
  CANDIDATE,
  CONTEXT_FEED,
  IN_FORCE,
  feedEntry,
  feedGroup,
} from "../stories/fixtures/contextFeed";
import type { ContextFeed } from "../types/api";

const api = vi.hoisted(() => ({
  contextFeed: vi.fn(),
  keepContextSuggestion: vi.fn(),
  dropContextSuggestion: vi.fn(),
  contextResetScope: vi.fn(),
  resetContext: vi.fn(),
  contextWidenReach: vi.fn(),
  widenContextRule: vi.fn(),
}));
vi.mock("../hooks/useApi", () => ({ api }));

import { ContextFeedPanel } from "../components/ContextFeedPanel";

const PERSON_DAY = "actor:person/asgeir:2026-09-21";

function renderPanel(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  const view = render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
  return { client, view };
}

const detail = () => document.querySelector("[data-slot='context-feed-detail']") as HTMLElement;
const entry = (id: string) => document.querySelector(`[data-entry='${id}']`) as HTMLElement;

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset();
  api.contextFeed.mockResolvedValue(CONTEXT_FEED);
  api.keepContextSuggestion.mockResolvedValue(CANDIDATE);
  api.dropContextSuggestion.mockResolvedValue(CANDIDATE);
  api.contextResetScope.mockResolvedValue({
    before: "sess-1",
    set_aside: 2,
    decisions: 0,
    restored: 0,
    rules: [],
    subjects: [],
  });
  api.resetContext.mockResolvedValue({
    before: "sess-1",
    set_aside: 2,
    decisions: 0,
    restored: 0,
    rules: [],
    subjects: [],
  });
});

describe("History wired to the backend", () => {
  it("keeps a proposal through the same call the review session makes", async () => {
    renderPanel(<ContextFeedPanel projectKey="kapimart" keyboard={false} />);
    const keep = await waitFor(() => {
      const el = entry(CANDIDATE.id)?.querySelector("[data-slot='keep-suggestion']");
      expect(el).toBeTruthy();
      return el as HTMLElement;
    });
    fireEvent.click(keep);
    await waitFor(() => expect(api.keepContextSuggestion).toHaveBeenCalledTimes(1));
    expect(api.keepContextSuggestion).toHaveBeenCalledWith({
      project: "kapimart",
      id: CANDIDATE.id,
      replacement: undefined,
      advisory: undefined,
    });
    expect(api.contextFeed).toHaveBeenCalledWith("kapimart", 200);
  });

  it("drops a proposal and a rule in force through the drop call", async () => {
    renderPanel(<ContextFeedPanel projectKey="kapimart" keyboard={false} />);
    await waitFor(() => expect(entry(IN_FORCE.id)).not.toBeNull());
    fireEvent.click(entry(CANDIDATE.id).querySelector("[data-slot='drop-suggestion']")!);
    fireEvent.click(entry(IN_FORCE.id).querySelector("[data-slot='drop-rule']")!);
    await waitFor(() => expect(api.dropContextSuggestion).toHaveBeenCalledTimes(2));
    expect(api.dropContextSuggestion).toHaveBeenNthCalledWith(1, {
      project: "kapimart",
      id: CANDIDATE.id,
    });
    expect(api.dropContextSuggestion).toHaveBeenNthCalledWith(2, {
      project: "kapimart",
      id: IN_FORCE.id,
    });
  });

  it("resets to before the selected session after the confirmation", async () => {
    renderPanel(<ContextFeedPanel projectKey="kapimart" keyboard={false} />);
    const reset = await waitFor(() => {
      const el = document.querySelector("[data-slot='reset-session']");
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    fireEvent.click(reset);
    const confirm = await waitFor(() => {
      const el = document.querySelector("[data-slot='reset-confirm']");
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    expect(api.contextResetScope).toHaveBeenCalledWith({ project: "kapimart", before: "sess-1" });
    fireEvent.click(confirm);
    await waitFor(() =>
      expect(api.resetContext).toHaveBeenCalledWith({ project: "kapimart", before: "sess-1" }),
    );
  });

  it("keeps the selected session selected through a refetch", async () => {
    const { client } = renderPanel(<ContextFeedPanel keyboard={false} />);
    const day = await waitFor(() => {
      const el = document.querySelector(
        `[data-slot='context-feed-session'][data-session='${PERSON_DAY}']`,
      );
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    fireEvent.click(day);
    expect(detail().getAttribute("data-session")).toBe(PERSON_DAY);

    // An agent recorded a new session in another process; the watcher's
    // refetch brings it in at the top.
    const next: ContextFeed = {
      ...CONTEXT_FEED,
      groups: [
        feedGroup({
          id: "session:sess-9",
          session: "sess-9",
          entries: [feedEntry({ ...CANDIDATE, id: "40" })],
        }),
        ...CONTEXT_FEED.groups,
      ],
    };
    api.contextFeed.mockResolvedValue(next);
    await act(async () => {
      await client.invalidateQueries();
    });
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='context-feed-session']")).toHaveLength(3),
    );
    expect(
      document
        .querySelector("[data-slot='context-feed-session'][aria-selected='true']")
        ?.getAttribute("data-session"),
    ).toBe(PERSON_DAY);
    expect(detail().getAttribute("data-session")).toBe(PERSON_DAY);
  });

  it("renders a project with no checkout as the same list and detail, scoped to it", async () => {
    api.contextFeed.mockResolvedValue({
      ...CONTEXT_FEED,
      project_key: "handbook",
      groups: [
        feedGroup({
          id: "session:sess-1",
          session: "sess-1",
          project_key: "handbook",
          project_name: "Old Handbook",
          recipe: undefined,
          entries: [
            feedEntry({
              ...CANDIDATE,
              project_key: "handbook",
              project_name: "Old Handbook",
              recipe: undefined,
            }),
          ],
        }),
      ],
    } satisfies ContextFeed);
    renderPanel(<ContextFeedPanel projectKey="handbook" keyboard={false} />);
    await waitFor(() => expect(screen.getByRole("listbox", { name: "Sessions" })).toBeVisible());
    expect(api.contextFeed).toHaveBeenCalledWith("handbook", 200);
    expect(detail().querySelector("[data-slot='context-no-checkout']")).not.toBeNull();
    fireEvent.click(entry(CANDIDATE.id).querySelector("[data-slot='keep-suggestion']")!);
    await waitFor(() =>
      expect(api.keepContextSuggestion).toHaveBeenCalledWith(
        expect.objectContaining({ project: "handbook", id: CANDIDATE.id }),
      ),
    );
  });
});
