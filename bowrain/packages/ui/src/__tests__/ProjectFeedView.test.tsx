import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import type { ApiAdapter } from "../api/adapter";
import { ProjectFeedView } from "../project-feed/ProjectFeedView";
import { feedEveryKind, feedFolded } from "../project-feed/fixtures";

function renderFeed(
  listProjectFeed: ApiAdapter["listProjectFeed"],
  handlers: {
    onOpenLink?: ReturnType<typeof vi.fn>;
    onOpenLanguage?: ReturnType<typeof vi.fn>;
  } = {},
) {
  const adapter = { listProjectFeed } as unknown as ApiAdapter;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onOpenLink = handlers.onOpenLink ?? vi.fn();
  const onOpenLanguage = handlers.onOpenLanguage ?? vi.fn();
  render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <ProjectFeedView
          workspace="acme"
          projectId="p1"
          onOpenLink={onOpenLink}
          onOpenLanguage={onOpenLanguage}
        />
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { onOpenLink, onOpenLanguage };
}

function cards(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>('[data-slot="feed-card"]'));
}

describe("ProjectFeedView", () => {
  it("shows a card per entry with who did what", async () => {
    renderFeed(vi.fn().mockResolvedValue({ entries: feedEveryKind }));
    await waitFor(() => expect(cards()).toHaveLength(feedEveryKind.length));
    const text = cards().map((c) => c.textContent ?? "");
    expect(text[0]).toContain("Ada Lindqvist pushed 40 blocks to main");
    expect(text.find((t) => t.includes("claude-code"))).toContain("proposed 6 term rules");
    expect(text.find((t) => t.includes("claude-code"))).toContain("for Ada Lindqvist");
    expect(text.find((t) => t.includes("Convergence"))).toContain(
      "drafted 40 translations in 2 languages",
    );
    expect(text.find((t) => t.includes("content memory"))).toContain("+3 more");
    expect(screen.getByText("Today")).toBeTruthy();
  });

  it("opens a card's link, and a language chip's deck", async () => {
    const { onOpenLink, onOpenLanguage } = renderFeed(
      vi.fn().mockResolvedValue({ entries: feedEveryKind }),
    );
    await waitFor(() => expect(cards().length).toBeGreaterThan(0));
    await userEvent.click(screen.getByText(/pushed 40 blocks/));
    expect(onOpenLink).toHaveBeenCalledWith(expect.objectContaining({ id: "e-push" }));
    const chip = Array.from(
      document.querySelectorAll<HTMLElement>('[data-slot="feed-chip-link"]'),
    ).find((el) => el.textContent?.includes("French"));
    expect(chip).toBeTruthy();
    await userEvent.click(chip!);
    expect(onOpenLanguage).toHaveBeenCalledWith("fr");
  });

  it("folds one actor's runs into one card and leaves out quiet runs", async () => {
    renderFeed(vi.fn().mockResolvedValue({ entries: feedFolded }));
    await waitFor(() => expect(cards()).toHaveLength(1));
    expect(cards()[0].textContent).toContain("Convergence ran 3 times");
    await userEvent.click(screen.getByRole("button", { name: /Show 2 more/ }));
    expect(cards()[0].textContent).toContain("drafted 7 translations");
  });

  it("asks the server for the kinds a filter selects", async () => {
    const list = vi.fn().mockResolvedValue({ entries: feedEveryKind });
    renderFeed(list);
    await waitFor(() => expect(cards().length).toBeGreaterThan(0));
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
