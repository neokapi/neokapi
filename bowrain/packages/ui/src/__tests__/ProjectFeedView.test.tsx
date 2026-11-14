import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import type { ApiAdapter } from "../api/adapter";
import { ProjectFeedView } from "../project-feed/ProjectFeedView";
import { feedEveryKind } from "../project-feed/fixtures";

function renderFeed(listProjectFeed: ApiAdapter["listProjectFeed"], onOpenLink = vi.fn()) {
  const adapter = { listProjectFeed } as unknown as ApiAdapter;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <ProjectFeedView workspace="acme" projectId="p1" onOpenLink={onOpenLink} />
      </ApiProvider>
    </QueryClientProvider>,
  );
  return onOpenLink;
}

describe("ProjectFeedView", () => {
  it("lists one row per entry with its title, actor and main count", async () => {
    renderFeed(vi.fn().mockResolvedValue({ entries: feedEveryKind }));
    const rows = await screen.findAllByRole("option");
    expect(rows).toHaveLength(feedEveryKind.length);
    expect(rows[0].textContent).toContain("Push · main");
    expect(rows[0].textContent).toContain("Ada Lindqvist");
    expect(rows[0].textContent).toContain("40 blocks");
    expect(screen.getByText("Today")).toBeTruthy();
  });

  it("shows the selected entry's counts and opens its link", async () => {
    const onOpen = renderFeed(vi.fn().mockResolvedValue({ entries: feedEveryKind }));
    const detail = await waitFor(() => {
      const el = document.querySelector('[data-slot="feed-entry-detail"]');
      if (!el) throw new Error("no detail");
      return el as HTMLElement;
    });
    expect(within(detail).getByText("files")).toBeTruthy();
    await userEvent.click(within(detail).getByRole("button", { name: "Open" }));
    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ id: "e-push" }));
  });

  it("asks the server for the kinds a filter selects", async () => {
    const list = vi.fn().mockResolvedValue({ entries: feedEveryKind });
    renderFeed(list);
    await screen.findAllByRole("option");
    await userEvent.click(document.querySelector('[data-slot="feed-filter-push"]') as HTMLElement);
    await waitFor(() =>
      expect(list).toHaveBeenLastCalledWith(
        "acme",
        "p1",
        expect.objectContaining({ kinds: ["push"] }),
      ),
    );
  });

  it("says when the project has no activity", async () => {
    renderFeed(vi.fn().mockResolvedValue({ entries: [] }));
    expect(await screen.findByText("No activity yet")).toBeTruthy();
  });
});
