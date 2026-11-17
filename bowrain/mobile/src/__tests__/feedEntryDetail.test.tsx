import type { FeedEntry, FeedPage } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import FeedEntryDetail from "../app/(tabs)/activity/[ws]/[project]/[entry]";
import { feedKey } from "../lib/feed";

jest.mock("expo-router", () => ({
  router: { push: jest.fn() },
  Stack: { Screen: () => null },
  useLocalSearchParams: () => ({ ws: "ws", project: "p", entry: "e1" }),
}));

const collections = Array.from({ length: 8 }, (_, i) => ({
  name: `col-${i}`,
  change: "added" as const,
  after: { product: "neokapi", channel: `ch${i}` },
}));

const recipe: FeedEntry = {
  id: "e1",
  kind: "recipe_change",
  at: "2026-11-15T14:45:00Z",
  actor: { id: "u1", kind: "person", name: "Ada", viewer: true },
  counts: { collections_added: 9 },
  stream: "main",
  diff: {
    collections: [
      ...collections,
      { name: "moved", change: "coordinates", before: { channel: "a" }, after: { channel: "b" } },
    ],
    targets: { before: ["nb"], after: ["nb", "ja"] },
  },
  link: { screen: "recipe_diff", id: "e1" },
};

describe("a feed entry's detail page", () => {
  it("reads a recipe change as a sentence and sections of collections", async () => {
    const qc = new QueryClient();
    qc.setQueryData<{ pages: FeedPage[]; pageParams: string[] }>(feedKey("ws", "p"), {
      pages: [{ entries: [recipe] }],
      pageParams: [""],
    });
    await render(
      <QueryClientProvider client={qc}>
        <FeedEntryDetail />
      </QueryClientProvider>,
    );
    expect(screen.getByText("You")).toBeTruthy();
    expect(screen.getByTestId("detail-languages")).toBeTruthy();
    expect(screen.getByText("Japanese")).toBeTruthy();
    expect(screen.getByText("channel: a → b")).toBeTruthy();
    // Six rows show, then Show all.
    expect(screen.queryByTestId("collection-col-6")).toBeNull();
    await fireEvent.press(screen.getByText("Show all 8"));
    expect(screen.getByTestId("collection-col-7")).toBeTruthy();
    expect(screen.getAllByText("product: neokapi")).toHaveLength(8);
  });
});
