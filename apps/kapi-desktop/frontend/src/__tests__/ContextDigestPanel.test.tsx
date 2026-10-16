import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "./testUtils";

import { ContextDigestPanel } from "../components/ContextDigestPanel";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { LAST_LOOKED, SAMPLE_DIGEST, SAMPLE_IDS } from "../stories/fixtures/contextDigest";

// The panel reads the digest from the backend and writes each decision back
// through it. These pin the calls each action makes, so a change to the view
// cannot change what reaches the backend.

const PROJECT = SAMPLE_DIGEST.project;

beforeEach(() => {
  vi.spyOn(api, "projectContextDigest").mockResolvedValue({ ...SAMPLE_DIGEST, since: LAST_LOOKED });
  vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue(undefined as never);
  vi.spyOn(api, "keepContextSuggestion").mockResolvedValue(undefined as never);
  vi.spyOn(api, "chooseContextSide").mockResolvedValue(undefined as never);
  vi.spyOn(api, "dropContextSuggestion").mockResolvedValue(undefined as never);
  vi.spyOn(api, "keepContextGroup").mockResolvedValue(undefined as never);
});

afterEach(() => {
  vi.restoreAllMocks();
});

async function openPanel() {
  render(<ContextDigestPanel tabID="t1" />);
  await screen.findByRole("listbox", { name: "What's new" });
}

const row = (id: string) =>
  document.querySelector<HTMLElement>(`[data-slot='digest-item'][data-id='${id}']`)!;
const toolbar = () => document.querySelector<HTMLElement>("[data-slot='list-detail-toolbar']")!;

describe("the digest panel", () => {
  it("reads the digest for the tab and marks it seen", async () => {
    await openPanel();
    expect(api.projectContextDigest).toHaveBeenCalledWith("t1", "");
    await waitFor(() => expect(api.markProjectContextDigestSeen).toHaveBeenCalledWith("t1"));
  });

  it("reads the home's standings again once the marker moves", async () => {
    // The home counts what is new since the marker, off the workspace list.
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    render(<ContextDigestPanel tabID="t1" />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({ queryKey: qk.workspaceProjects() }),
    );
  });

  it("keeps a suggestion through keepContextSuggestion", async () => {
    await openPanel();
    fireEvent.click(row(SAMPLE_IDS.kapiMart));
    fireEvent.click(within(toolbar()).getByRole("button", { name: "Keep" }));
    await waitFor(() =>
      expect(api.keepContextSuggestion).toHaveBeenCalledWith({
        project: PROJECT,
        id: SAMPLE_IDS.kapiMart,
        replacement: undefined,
      }),
    );
  });

  it("keeps a suggestion in another form through keepContextSuggestion", async () => {
    await openPanel();
    fireEvent.click(row(SAMPLE_IDS.cart));
    fireEvent.click(within(toolbar()).getByRole("button", { name: "Change" }));
    const input = document.querySelector<HTMLInputElement>("[data-slot='digest-change'] input")!;
    fireEvent.change(input, { target: { value: "shopping cart" } });
    fireEvent.click(screen.getByRole("button", { name: "Keep with this form" }));
    await waitFor(() =>
      expect(api.keepContextSuggestion).toHaveBeenCalledWith({
        project: PROJECT,
        id: SAMPLE_IDS.cart,
        replacement: "shopping cart",
      }),
    );
  });

  it("drops a suggestion through dropContextSuggestion", async () => {
    await openPanel();
    fireEvent.click(row(SAMPLE_IDS.shopper));
    fireEvent.click(within(toolbar()).getByRole("button", { name: "Drop" }));
    await waitFor(() =>
      expect(api.dropContextSuggestion).toHaveBeenCalledWith({
        project: PROJECT,
        id: SAMPLE_IDS.shopper,
      }),
    );
  });

  it("chooses a side through chooseContextSide", async () => {
    await openPanel();
    const side = document.querySelector<HTMLElement>(
      `[data-slot='digest-side'][data-id='${SAMPLE_IDS.signIn}']`,
    )!;
    fireEvent.click(within(side).getByRole("button", { name: "Choose this side" }));
    await waitFor(() =>
      expect(api.chooseContextSide).toHaveBeenCalledWith({
        project: PROJECT,
        id: SAMPLE_IDS.signIn,
        replacement: undefined,
      }),
    );
    expect(api.keepContextSuggestion).not.toHaveBeenCalled();
  });

  it("keeps a theme and collection at once through keepContextGroup", async () => {
    await openPanel();
    fireEvent.click(row(SAMPLE_IDS.kapiMart));
    fireEvent.keyDown(window, { key: "g" });
    await waitFor(() =>
      expect(api.keepContextGroup).toHaveBeenCalledWith(PROJECT, [
        SAMPLE_IDS.checkout,
        SAMPLE_IDS.kapiMart,
      ]),
    );
  });

  it("reads the digest again after a decision", async () => {
    await openPanel();
    const reads = vi.mocked(api.projectContextDigest).mock.calls.length;
    fireEvent.click(row(SAMPLE_IDS.kapiMart));
    fireEvent.keyDown(window, { key: "a" });
    await waitFor(() =>
      expect(vi.mocked(api.projectContextDigest).mock.calls.length).toBeGreaterThan(reads),
    );
  });
});
