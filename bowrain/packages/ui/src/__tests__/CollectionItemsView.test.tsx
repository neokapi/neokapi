import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CollectionItemsView } from "../components/collections/CollectionItemsView";
import type {
  CollectionTranslationStats,
  ItemTranslationStats,
  LocaleTranslationStats,
} from "../types/api";

const localeStats: LocaleTranslationStats[] = [
  {
    locale: "nb",
    display_name: "Norwegian Bokmål",
    translated_blocks: 8,
    total_blocks: 10,
    translated_words: 80,
    total_words: 100,
    percentage: 80,
    approved_blocks: 5,
  },
];

const collection: CollectionTranslationStats = {
  collection_id: "col-app",
  collection_name: "bowrain-app",
  channel: "bowrain/app",
  coordinates: { product: "bowrain", channel: "app" },
  item_count: 2,
  block_count: 10,
  word_count: 100,
  locales: localeStats,
};

const items: ItemTranslationStats[] = [
  {
    item_name: "src/i18n/en.kbf.json",
    item_id: "i1",
    format: "json",
    collection_id: "col-app",
    collection_name: "bowrain-app",
    block_count: 6,
    word_count: 60,
    locales: localeStats,
  },
];

describe("CollectionItemsView", () => {
  it("restates which collection the items belong to", () => {
    render(
      <CollectionItemsView
        collection={collection}
        title={collection.collection_name!}
        itemStats={items}
        localeStats={localeStats}
        onBack={() => {}}
      />,
    );
    expect(screen.getByRole("heading", { name: "bowrain-app" })).toBeInTheDocument();
    expect(screen.getByText("bowrain/app")).toBeInTheDocument();
    expect(screen.getByText("2 items")).toBeInTheDocument();
    expect(screen.getByTestId("locale-rail-nb")).toBeInTheDocument();
  });

  it("lists the scope's items", () => {
    render(
      <CollectionItemsView
        collection={collection}
        title={collection.collection_name!}
        itemStats={items}
        localeStats={localeStats}
        onBack={() => {}}
      />,
    );
    expect(screen.getByText(/en\.kbf/)).toBeInTheDocument();
  });

  it("returns to the overview", async () => {
    const user = userEvent.setup();
    const onBack = vi.fn();
    render(
      <CollectionItemsView
        collection={collection}
        title={collection.collection_name!}
        itemStats={items}
        localeStats={localeStats}
        onBack={onBack}
      />,
    );
    await user.click(screen.getByTestId("collection-items-back"));
    expect(onBack).toHaveBeenCalledOnce();
  });

  it("opens an item when the caller can navigate to one", async () => {
    const user = userEvent.setup();
    const onOpenItem = vi.fn();
    render(
      <CollectionItemsView
        collection={collection}
        title={collection.collection_name!}
        itemStats={items}
        localeStats={localeStats}
        onBack={() => {}}
        onOpenItem={onOpenItem}
      />,
    );
    await user.click(screen.getByText(/en\.kbf/));
    expect(onOpenItem).toHaveBeenCalledWith(items[0]);
  });

  it("omits the rollup header for the unscoped all-items list", () => {
    render(
      <CollectionItemsView
        title="All items"
        itemStats={items}
        localeStats={localeStats}
        onBack={() => {}}
      />,
    );
    expect(screen.getByRole("heading", { name: "All items" })).toBeInTheDocument();
    expect(screen.queryByTestId("locale-rail-nb")).toBeNull();
  });

  it("counts the scope, not the page, when the server pages the list", () => {
    render(
      <CollectionItemsView
        collection={collection}
        title={collection.collection_name!}
        itemStats={items}
        localeStats={localeStats}
        paging={{
          total: 2,
          sortField: "name",
          sortDir: "asc",
          onSortChange: () => {},
          hasMore: true,
          onLoadMore: () => {},
        }}
        onBack={() => {}}
      />,
    );
    expect(screen.getByTestId("file-progress-count")).toHaveTextContent("Showing 1 of 2 files");
  });

  it("carries the scope into review", async () => {
    const user = userEvent.setup();
    const onReview = vi.fn();
    render(
      <CollectionItemsView
        collection={collection}
        title={collection.collection_name!}
        itemStats={items}
        localeStats={localeStats}
        onBack={() => {}}
        onReview={onReview}
      />,
    );
    const header = screen.getByTestId("collection-items-view");
    await user.click(within(header).getByRole("button", { name: "Review" }));
    expect(onReview).toHaveBeenCalledOnce();
  });
});
