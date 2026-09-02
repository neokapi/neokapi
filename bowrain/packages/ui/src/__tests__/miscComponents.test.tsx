import { SidebarProvider, TooltipProvider } from "@neokapi/ui-primitives";
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// ---------------------------------------------------------------------------
// 1. ConfirmDialog
// ---------------------------------------------------------------------------

import { ConfirmDialog } from "../components/ConfirmDialog";

describe("ConfirmDialog", () => {
  it("renders title and description when open", () => {
    render(
      <ConfirmDialog
        open={true}
        onOpenChange={() => {}}
        title="Delete item?"
        description="This action cannot be undone."
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText("Delete item?")).toBeInTheDocument();
    expect(screen.getByText("This action cannot be undone.")).toBeInTheDocument();
  });

  it("does not render content when closed", () => {
    render(
      <ConfirmDialog
        open={false}
        onOpenChange={() => {}}
        title="Delete item?"
        description="This action cannot be undone."
        onConfirm={() => {}}
      />,
    );
    expect(screen.queryByText("Delete item?")).not.toBeInTheDocument();
  });

  it("calls onConfirm when confirm button is clicked", async () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        open={true}
        onOpenChange={() => {}}
        title="Confirm"
        description="Are you sure?"
        onConfirm={onConfirm}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("calls onOpenChange(false) when cancel button is clicked", async () => {
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open={true}
        onOpenChange={onOpenChange}
        title="Confirm"
        description="Are you sure?"
        onConfirm={() => {}}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("shows loading state with '...' when loading=true", () => {
    render(
      <ConfirmDialog
        open={true}
        onOpenChange={() => {}}
        title="Working"
        description="Please wait."
        onConfirm={() => {}}
        loading={true}
      />,
    );
    expect(screen.getByRole("button", { name: "..." })).toBeInTheDocument();
  });

  it("uses custom confirm and cancel labels", () => {
    render(
      <ConfirmDialog
        open={true}
        onOpenChange={() => {}}
        title="Remove?"
        description="Remove this thing?"
        confirmLabel="Yes, remove"
        cancelLabel="No, keep"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: "Yes, remove" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "No, keep" })).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 2. NotificationCenter
// ---------------------------------------------------------------------------

import { NotificationCenter } from "../components/NotificationCenter";
import type { NotificationInfo } from "../types/api";

function makeNotification(overrides: Partial<NotificationInfo> = {}): NotificationInfo {
  return {
    id: "n1",
    user_id: "u1",
    type: "review.assigned",
    title: "Review requested",
    body: "Please review the translation.",
    read: false,
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("NotificationCenter", () => {
  const baseProps = {
    notifications: [] as NotificationInfo[],
    unreadCount: 0,
    onMarkRead: vi.fn(),
    onMarkAllRead: vi.fn(),
    onDelete: vi.fn(),
  };

  it("renders bell button", () => {
    render(<NotificationCenter {...baseProps} />);
    expect(screen.getByTitle("Notifications")).toBeInTheDocument();
  });

  it("shows unread count badge when > 0", () => {
    render(<NotificationCenter {...baseProps} unreadCount={5} />);
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("does not show badge when unreadCount is 0", () => {
    render(<NotificationCenter {...baseProps} unreadCount={0} />);
    // No badge element with a number
    expect(screen.queryByText("0")).not.toBeInTheDocument();
  });

  it("shows 99+ for large counts", () => {
    render(<NotificationCenter {...baseProps} unreadCount={150} />);
    expect(screen.getByText("99+")).toBeInTheDocument();
  });

  it("opens dropdown on click and shows 'No notifications' when empty", async () => {
    render(<NotificationCenter {...baseProps} />);
    await userEvent.click(screen.getByTitle("Notifications"));
    expect(screen.getByText("No notifications")).toBeInTheDocument();
  });

  it("renders notifications with titles when opened", async () => {
    const notifications = [
      makeNotification({ id: "n1", title: "First notification" }),
      makeNotification({ id: "n2", title: "Second notification" }),
    ];
    render(<NotificationCenter {...baseProps} notifications={notifications} unreadCount={2} />);
    await userEvent.click(screen.getByTitle("Notifications"));
    expect(screen.getByText("First notification")).toBeInTheDocument();
    expect(screen.getByText("Second notification")).toBeInTheDocument();
  });

  it("calls onMarkAllRead when 'Mark all read' is clicked", async () => {
    const onMarkAllRead = vi.fn();
    const notifications = [makeNotification()];
    render(
      <NotificationCenter
        {...baseProps}
        notifications={notifications}
        unreadCount={1}
        onMarkAllRead={onMarkAllRead}
      />,
    );
    await userEvent.click(screen.getByTitle("Notifications"));
    await userEvent.click(screen.getByText("Mark all read"));
    expect(onMarkAllRead).toHaveBeenCalledOnce();
  });

  it("calls onDelete with notification id via delete button", async () => {
    const onDelete = vi.fn();
    const notifications = [makeNotification({ id: "del-1" })];
    render(
      <NotificationCenter
        {...baseProps}
        notifications={notifications}
        unreadCount={1}
        onDelete={onDelete}
      />,
    );
    await userEvent.click(screen.getByTitle("Notifications"));
    await userEvent.click(screen.getByTitle("Delete"));
    expect(onDelete).toHaveBeenCalledWith("del-1");
  });
});

// ---------------------------------------------------------------------------
// 3. FileProgressTable
// ---------------------------------------------------------------------------

import { FileProgressTable, type FileProgressPaging } from "../components/FileProgressTable";
import type { ItemTranslationStats } from "../types/api";

function makeItemStat(overrides: Partial<ItemTranslationStats> = {}): ItemTranslationStats {
  return {
    item_name: "messages.json",
    item_id: "item-1",
    format: "json",
    collection_id: "c1",
    block_count: 10,
    word_count: 100,
    locales: [
      {
        locale: "fr-FR",
        translated_blocks: 8,
        total_blocks: 10,
        translated_words: 80,
        total_words: 100,
        percentage: 80,
      },
    ],
    ...overrides,
  };
}

describe("FileProgressTable", () => {
  it("renders file names and locale columns", () => {
    const items = [
      makeItemStat({ item_name: "strings.json", item_id: "i1" }),
      makeItemStat({ item_name: "errors.json", item_id: "i2" }),
    ];
    render(<FileProgressTable itemStats={items} locales={["fr-FR", "de-DE"]} />);
    expect(screen.getByText("strings")).toBeInTheDocument();
    expect(screen.getByText("errors")).toBeInTheDocument();
    // LanguageLabel renders display names with variant="short" and hideCode
    expect(screen.getByText("French")).toBeInTheDocument();
    expect(screen.getByText("German")).toBeInTheDocument();
  });

  it("names a column without a sort indicator bleeding into it", () => {
    // The header is a translated text run, so an indicator returning null used
    // to arrive as the string "null" — "Wordsnull", "Avg %null".
    render(<FileProgressTable itemStats={[makeItemStat()]} locales={["fr-FR"]} />);
    for (const header of ["Words", "Avg %", "Format"]) {
      expect(screen.getByRole("columnheader", { name: new RegExp(`^${header}`) })).toBeVisible();
    }
    expect(screen.queryByText(/null/)).not.toBeInTheDocument();
  });

  it("marks the sorted column for assistive technology and no other", () => {
    render(<FileProgressTable itemStats={[makeItemStat()]} locales={[]} />);
    expect(screen.getByRole("columnheader", { name: /^File/ })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
    expect(screen.getByRole("columnheader", { name: /^Words/ })).toHaveAttribute(
      "aria-sort",
      "none",
    );
  });

  it("lays out on declared column widths, so two pages of one list agree", () => {
    const { container, rerender } = render(
      <FileProgressTable itemStats={[makeItemStat()]} locales={["fr-FR"]} />,
    );
    const widths = () =>
      Array.from(container.querySelectorAll("colgroup col")).map(
        (c) => (c as HTMLElement).style.width,
      );
    const first = widths();
    // A different page of the same collection: different rows, same columns.
    rerender(
      <FileProgressTable
        itemStats={[
          makeItemStat({
            item_id: "i2",
            item_name: "a-considerably-longer-file-name.json",
            word_count: 9_999_999,
          }),
        ]}
        locales={["fr-FR"]}
      />,
    );
    expect(widths()).toEqual(first);
    expect(container.querySelector("table")).toHaveClass("table-fixed");
  });

  it("shows names relative to the base every row shares", () => {
    const items = [
      makeItemStat({ item_id: "i1", item_name: "bowrain/packages/app/src/i18n.ts" }),
      makeItemStat({ item_id: "i2", item_name: "bowrain/packages/app/src/queries.ts" }),
    ];
    render(
      <FileProgressTable itemStats={items} locales={[]} itemBase="bowrain/packages/app/src/" />,
    );
    expect(screen.getByText("i18n")).toBeInTheDocument();
    expect(screen.getByText("queries")).toBeInTheDocument();
    // The whole name stays reachable — it is what identifies the file.
    expect(screen.getByTitle("bowrain/packages/app/src/i18n.ts")).toBeInTheDocument();
  });

  it("leaves a name the base does not prefix whole rather than half-trimmed", () => {
    render(
      <FileProgressTable
        itemStats={[makeItemStat({ item_name: "elsewhere/stray.json" })]}
        locales={[]}
        itemBase="bowrain/packages/app/src/"
      />,
    );
    expect(screen.getByText("elsewhere/stray")).toBeInTheDocument();
  });

  it("renders format and word count", () => {
    const items = [makeItemStat({ format: "xliff", word_count: 500 })];
    render(<FileProgressTable itemStats={items} locales={["fr-FR"]} />);
    expect(screen.getByText("xliff")).toBeInTheDocument();
    expect(screen.getByText("500")).toBeInTheDocument();
  });

  it("toggles sort direction on column header click", async () => {
    const items = [
      makeItemStat({ item_name: "alpha.json", item_id: "i1", word_count: 50 }),
      makeItemStat({ item_name: "beta.json", item_id: "i2", word_count: 200 }),
    ];
    render(<FileProgressTable itemStats={items} locales={[]} />);

    // Default sort is by name asc — alpha should come first
    const rows = screen.getAllByRole("row");
    // Row 0 is the header, row 1 is first data row
    expect(within(rows[1]).getByText("alpha")).toBeInTheDocument();

    // Click File column header to toggle to desc
    await userEvent.click(screen.getByRole("button", { name: /^File/ }));
    const rowsAfter = screen.getAllByRole("row");
    expect(within(rowsAfter[1]).getByText("beta")).toBeInTheDocument();
  });

  it("states the sort in aria-sort rather than in the column's name", async () => {
    render(<FileProgressTable itemStats={[makeItemStat()]} locales={[]} />);

    const file = screen.getByRole("columnheader", { name: /^File/ });
    const words = screen.getByRole("columnheader", { name: /^Words/ });
    expect(file).toHaveAttribute("aria-sort", "ascending");
    expect(words).toHaveAttribute("aria-sort", "none");

    // The direction lives in the attribute and in an aria-hidden icon, so the
    // column is named "File" in every state — never "File ↑", never "Filenull".
    expect(file).toHaveAccessibleName("File");
    expect(words).toHaveAccessibleName("Words");

    await userEvent.click(screen.getByRole("button", { name: /^File/ }));
    expect(screen.getByRole("columnheader", { name: /^File/ })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
  });

  it("caps rendering at 500 rows and shows an honest cap row", () => {
    const items = Array.from({ length: 520 }, (_, i) =>
      makeItemStat({ item_name: `file-${i}.json`, item_id: `i${i}`, locales: [] }),
    );
    render(<FileProgressTable itemStats={items} locales={[]} />);
    // 1 header row + 500 data rows
    expect(screen.getAllByRole("row")).toHaveLength(501);
    const capRow = screen.getByTestId("list-cap-row");
    expect(capRow).toHaveTextContent("Showing first 500 of 520 files");
  });

  it("shows no cap row when under the limit", () => {
    render(<FileProgressTable itemStats={[makeItemStat()]} locales={[]} />);
    expect(screen.queryByTestId("list-cap-row")).not.toBeInTheDocument();
  });
});

describe("FileProgressTable (server paging)", () => {
  function makePaging(overrides: Partial<FileProgressPaging> = {}): FileProgressPaging {
    return {
      total: 3,
      sortField: "name",
      sortDir: "asc",
      onSortChange: () => {},
      hasMore: true,
      onLoadMore: () => {},
      ...overrides,
    };
  }

  it("renders the page as-is (server-sorted) with an honest N-of-M count", () => {
    // Deliberately not name-sorted: controlled mode must not re-sort locally.
    const items = [
      makeItemStat({ item_name: "zulu.json", item_id: "i1" }),
      makeItemStat({ item_name: "alpha.json", item_id: "i2" }),
    ];
    render(<FileProgressTable itemStats={items} locales={[]} paging={makePaging()} />);
    const rows = screen.getAllByRole("row");
    expect(within(rows[1]).getByText("zulu")).toBeInTheDocument();
    expect(within(rows[2]).getByText("alpha")).toBeInTheDocument();
    expect(screen.getByTestId("file-progress-count")).toHaveTextContent("Showing 2 of 3 files");
  });

  it("delegates header clicks to onSortChange instead of sorting locally", async () => {
    const onSortChange = vi.fn();
    const items = [makeItemStat()];
    render(
      <FileProgressTable itemStats={items} locales={[]} paging={makePaging({ onSortChange })} />,
    );

    // Same column: toggles direction.
    await userEvent.click(screen.getByRole("button", { name: /^File/ }));
    expect(onSortChange).toHaveBeenCalledWith("name", "desc");

    // Different column: starts ascending.
    await userEvent.click(screen.getByRole("button", { name: /^Words/ }));
    expect(onSortChange).toHaveBeenCalledWith("words", "asc");
  });

  it("loads the next page via onLoadMore while more rows exist", async () => {
    const onLoadMore = vi.fn();
    render(
      <FileProgressTable
        itemStats={[makeItemStat()]}
        locales={[]}
        paging={makePaging({ onLoadMore })}
      />,
    );
    await userEvent.click(screen.getByTestId("file-progress-load-more"));
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("hides the load-more button when every row is loaded", () => {
    render(
      <FileProgressTable
        itemStats={[makeItemStat()]}
        locales={[]}
        paging={makePaging({ total: 1, hasMore: false })}
      />,
    );
    expect(screen.queryByTestId("file-progress-load-more")).not.toBeInTheDocument();
    expect(screen.getByTestId("file-progress-count")).toHaveTextContent("Showing 1 of 1 file");
  });
});

// ---------------------------------------------------------------------------
// 4. CollectionRail
// ---------------------------------------------------------------------------

import { CollectionRail } from "../components/CollectionRail";
import type { CollectionInfo } from "../types/api";

function makeCollection(overrides: Partial<CollectionInfo> = {}): CollectionInfo {
  return {
    id: "c1",
    project_id: "p1",
    name: "My Collection",
    kind: "manual",
    item_label: "items",
    is_default: false,
    item_count: 10,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("CollectionRail", () => {
  it("renders every collection with its item count", () => {
    const collections = [
      makeCollection({ id: "c1", name: "Strings", item_count: 12 }),
      makeCollection({ id: "c2", name: "Docs", item_count: 5 }),
    ];
    render(
      <CollectionRail
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
      />,
    );
    expect(screen.getByText("Strings")).toBeInTheDocument();
    expect(screen.getByText("Docs")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("groups collections under the product they are declared at", () => {
    const collections = [
      makeCollection({
        id: "c1",
        name: "bowrain-app",
        item_count: 297,
        coordinates: { product: "bowrain", channel: "app" },
      }),
      makeCollection({
        id: "c2",
        name: "bowrain-docs",
        item_count: 113,
        coordinates: { product: "bowrain", channel: "docs" },
      }),
      makeCollection({
        id: "c3",
        name: "neokapi-cli",
        item_count: 12,
        coordinates: { product: "neokapi", channel: "cli" },
      }),
    ];
    render(
      <CollectionRail
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
      />,
    );

    // The group states the product once, and the rows carry the channel — the
    // name spelled it out ("bowrain-app") and flattened the coordinate.
    expect(screen.getByTestId("collection-group-bowrain")).toHaveTextContent("bowrain");
    expect(screen.getByTestId("collection-group-bowrain")).toHaveTextContent("410");
    expect(screen.getByTestId("collection-group-neokapi")).toHaveTextContent("neokapi");
    expect(screen.getByTestId("collection-c1")).toHaveTextContent("app");
    expect(screen.getByTestId("collection-c3")).toHaveTextContent("cli");
  });

  it("files a collection declared at no point under Ungrouped, after the rest", () => {
    const collections = [
      makeCollection({ id: "c1", name: "Uploads" }),
      makeCollection({ id: "c2", name: "bowrain-app", coordinates: { product: "bowrain" } }),
    ];
    render(
      <CollectionRail
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
      />,
    );
    const groups = screen.getAllByRole("button", { expanded: true });
    expect(groups.map((g) => g.textContent)).toEqual([
      expect.stringContaining("bowrain"),
      expect.stringContaining("Ungrouped"),
    ]);
  });

  it("shows 'All items' for the default collection", () => {
    const collections = [
      makeCollection({ id: "c1", is_default: true, name: "Default" }),
      makeCollection({ id: "c2", name: "Other" }),
    ];
    render(
      <CollectionRail
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
      />,
    );
    expect(screen.getByText("All items")).toBeInTheDocument();
  });

  it("calls onSelectCollection when a row is clicked", async () => {
    const onSelect = vi.fn();
    const collections = [
      makeCollection({ id: "c1", name: "First" }),
      makeCollection({ id: "c2", name: "Second" }),
    ];
    render(
      <CollectionRail
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={onSelect}
      />,
    );
    await userEvent.click(screen.getByText("Second"));
    expect(onSelect).toHaveBeenCalledWith("c2");
  });

  it("collapses a group without losing the selection", async () => {
    // Three collections over two products, so the grouping axis is `product`
    // without ambiguity: with one collection each, product and channel cover
    // and cut the set identically and the tie falls alphabetically.
    const collections = [
      makeCollection({
        id: "c1",
        name: "app",
        coordinates: { product: "bowrain", channel: "app" },
      }),
      makeCollection({
        id: "c2",
        name: "cli",
        coordinates: { product: "neokapi", channel: "cli" },
      }),
      makeCollection({
        id: "c3",
        name: "desktop",
        coordinates: { product: "neokapi", channel: "desktop" },
      }),
    ];
    render(
      <CollectionRail
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
      />,
    );
    await userEvent.click(screen.getByTestId("collection-group-neokapi"));
    expect(screen.queryByTestId("collection-c2")).not.toBeInTheDocument();
    expect(screen.getByTestId("collection-c1")).toBeInTheDocument();
  });

  it("shows the create button when onCreateCollection is provided", () => {
    render(
      <CollectionRail
        collections={[makeCollection()]}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
        onCreateCollection={() => {}}
      />,
    );
    expect(screen.getByTestId("create-collection")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 5. WordCountChart — recharts uses SVG which jsdom does not fully support.
//    We test that the component renders without crashing and shows the card title.
// ---------------------------------------------------------------------------

import { WordCountChart } from "../components/WordCountChart";

describe("WordCountChart", () => {
  it("renders the card title", () => {
    render(
      <WordCountChart
        localeStats={[
          {
            locale: "fr-FR",
            translated_blocks: 8,
            total_blocks: 10,
            translated_words: 80,
            total_words: 100,
            percentage: 80,
          },
        ]}
      />,
    );
    expect(screen.getByText("Word Count by Language")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 6. ActivityTaskIndicators
// ---------------------------------------------------------------------------

import { ActivityIndicator, TaskIndicator } from "../components/ActivityTaskIndicators";
import type { ActivityInfo, TaskInfo } from "../types/api";

function makeActivity(overrides: Partial<ActivityInfo> = {}): ActivityInfo {
  return {
    id: "a1",
    workspace_id: "w1",
    actor_id: "u1",
    actor_name: "Alice",
    type: "extraction.completed",
    summary: "extracted 50 blocks",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

function makeTask(overrides: Partial<TaskInfo> = {}): TaskInfo {
  return {
    id: "t1",
    workspace_id: "w1",
    project_id: "p1",
    type: "review",
    status: "open",
    priority: "normal",
    title: "Review French translations",
    created_by: "u2",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("ActivityIndicator", () => {
  it("renders the activity button", () => {
    render(<ActivityIndicator activities={[]} />);
    expect(screen.getByTitle("Recent activity")).toBeInTheDocument();
  });

  it("shows 'No recent activity' when opened with empty list", async () => {
    render(<ActivityIndicator activities={[]} />);
    await userEvent.click(screen.getByTitle("Recent activity"));
    expect(screen.getByText("No recent activity")).toBeInTheDocument();
  });

  it("renders activity items when opened", async () => {
    const activities = [
      makeActivity({ id: "a1", actor_name: "Alice", summary: "pushed 10 files" }),
      makeActivity({ id: "a2", actor_name: "Bob", summary: "merged stream" }),
    ];
    render(<ActivityIndicator activities={activities} />);
    await userEvent.click(screen.getByTitle("Recent activity"));
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("pushed 10 files")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("merged stream")).toBeInTheDocument();
  });
});

describe("TaskIndicator", () => {
  it("renders the task button", () => {
    render(<TaskIndicator tasks={[]} />);
    expect(screen.getByTitle("My tasks")).toBeInTheDocument();
  });

  it("shows actionable count badge for open/in_progress tasks", () => {
    const tasks = [
      makeTask({ id: "t1", status: "open" }),
      makeTask({ id: "t2", status: "in_progress" }),
      makeTask({ id: "t3", status: "completed" }),
    ];
    render(<TaskIndicator tasks={tasks} />);
    // 2 actionable tasks
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("shows 'No tasks assigned to you' when opened with empty list", async () => {
    render(<TaskIndicator tasks={[]} />);
    await userEvent.click(screen.getByTitle("My tasks"));
    expect(screen.getByText("No tasks assigned to you")).toBeInTheDocument();
  });

  it("renders task titles when opened", async () => {
    const tasks = [
      makeTask({ id: "t1", title: "Review French" }),
      makeTask({ id: "t2", title: "Check German" }),
    ];
    render(<TaskIndicator tasks={tasks} />);
    await userEvent.click(screen.getByTitle("My tasks"));
    expect(screen.getByText("Review French")).toBeInTheDocument();
    expect(screen.getByText("Check German")).toBeInTheDocument();
  });

  it("calls onCompleteTask with stopPropagation when Done is clicked", async () => {
    const onComplete = vi.fn();
    const onTaskClick = vi.fn();
    const tasks = [makeTask({ id: "t1", title: "Review", status: "open" })];
    render(<TaskIndicator tasks={tasks} onCompleteTask={onComplete} onTaskClick={onTaskClick} />);
    await userEvent.click(screen.getByTitle("My tasks"));
    await userEvent.click(screen.getByText("Done"));
    expect(onComplete).toHaveBeenCalledWith("t1");
    // The row click handler should NOT have been called because of stopPropagation
    expect(onTaskClick).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 7. FilterBar — complex component, test key behaviors
// ---------------------------------------------------------------------------

import { FilterBar, type FilterToken, type FilterField } from "../components/FilterBar";

describe("FilterBar", () => {
  const fields: FilterField[] = [
    { key: "project", label: "Project", values: [{ value: "my-app", label: "My App" }] },
    { key: "locale", label: "Locale" },
  ];

  it("renders the search input with placeholder", () => {
    render(
      <FilterBar
        filters={[]}
        onFiltersChange={() => {}}
        search=""
        onSearchChange={() => {}}
        fields={fields}
        placeholder="Search items..."
      />,
    );
    expect(screen.getByPlaceholderText("Search items...")).toBeInTheDocument();
  });

  it("renders active filter tokens as badges", () => {
    const filters: FilterToken[] = [{ key: "project", value: "my-app" }];
    render(
      <FilterBar
        filters={filters}
        onFiltersChange={() => {}}
        search=""
        onSearchChange={() => {}}
        fields={fields}
      />,
    );
    expect(screen.getByText(/project/i)).toBeInTheDocument();
    expect(screen.getByText(/my-app/i)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 8. WorkspaceSwitcher — requires SidebarProvider context
// ---------------------------------------------------------------------------

import { WorkspaceSwitcher } from "../components/WorkspaceSwitcher";
import type { Workspace } from "../types/api";

function makeWorkspace(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: "w1",
    name: "Acme Corp",
    slug: "acme",
    description: "",
    logo_url: "",
    type: "team",
    role: "owner",
    ...overrides,
  };
}

describe("WorkspaceSwitcher", () => {
  it("renders the active workspace icon", () => {
    const ws = makeWorkspace({ name: "Beta Inc" });
    render(
      <TooltipProvider>
        <SidebarProvider>
          <WorkspaceSwitcher workspaces={[ws]} activeWorkspace={ws} onSelectWorkspace={() => {}} />
        </SidebarProvider>
      </TooltipProvider>,
    );
    // WorkspaceIcon shows the first letter
    expect(screen.getByTitle("Beta Inc")).toBeInTheDocument();
  });

  it("shows '?' when no active workspace", () => {
    render(
      <TooltipProvider>
        <SidebarProvider>
          <WorkspaceSwitcher workspaces={[]} activeWorkspace={null} onSelectWorkspace={() => {}} />
        </SidebarProvider>
      </TooltipProvider>,
    );
    expect(screen.getByText("?")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 10. TopBar — requires ThemeProvider context
// ---------------------------------------------------------------------------

import { TopBar } from "../components/TopBar";
import { ThemeProvider } from "../context/ThemeContext";
import type { User } from "../types/api";

describe("TopBar", () => {
  const user: User = {
    id: "u1",
    email: "alice@example.com",
    name: "Alice Smith",
    avatar_url: "",
  };

  it("renders theme toggle button", () => {
    render(
      <ThemeProvider>
        <TopBar user={user} onSignOut={() => {}} />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme-toggle")).toBeInTheDocument();
  });

  it("renders user initials in account menu trigger", () => {
    render(
      <ThemeProvider>
        <TopBar user={user} onSignOut={() => {}} />
      </ThemeProvider>,
    );
    // UserAvatar shows initials "AS" for "Alice Smith"
    expect(screen.getByText("AS")).toBeInTheDocument();
  });

  it("renders leftSlot content", () => {
    render(
      <ThemeProvider>
        <TopBar user={null} leftSlot={<span data-testid="left">Left Content</span>} />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("left")).toBeInTheDocument();
  });

  it("shows offline pending changes indicator", () => {
    render(
      <ThemeProvider>
        <TopBar user={null} connectionState="offline" pendingChanges={3} />
      </ThemeProvider>,
    );
    expect(screen.getByText("3 pending")).toBeInTheDocument();
  });

  it("renders notification center when notification props are provided", () => {
    render(
      <ThemeProvider>
        <TopBar
          user={null}
          notifications={[]}
          unreadCount={0}
          onMarkNotificationRead={() => {}}
          onMarkAllNotificationsRead={() => {}}
          onDeleteNotification={() => {}}
        />
      </ThemeProvider>,
    );
    expect(screen.getByTitle("Notifications")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 11. BreadcrumbContext
// ---------------------------------------------------------------------------

import {
  BreadcrumbProvider,
  useBreadcrumb,
  useBreadcrumbExtra,
  type BreadcrumbItem,
} from "../context/BreadcrumbContext";

function BreadcrumbDisplay() {
  const items = useBreadcrumb();
  return <div data-testid="breadcrumb">{items.map((i) => i.label).join(" / ")}</div>;
}

function BreadcrumbExtra({ items }: { items: BreadcrumbItem[] }) {
  useBreadcrumbExtra(items);
  return null;
}

describe("BreadcrumbContext", () => {
  it("is empty by default", () => {
    render(
      <BreadcrumbProvider>
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByTestId("breadcrumb").textContent).toBe("");
  });

  it("reads the base trail the shell supplies", () => {
    render(
      <BreadcrumbProvider base={[{ label: "Acme" }, { label: "Projects" }]}>
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByTestId("breadcrumb").textContent).toBe("Acme / Projects");
  });

  it("appends the route's own steps after the base", () => {
    render(
      <BreadcrumbProvider base={[{ label: "Acme" }]}>
        <BreadcrumbExtra items={[{ label: "Docs collection" }]} />
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByTestId("breadcrumb").textContent).toBe("Acme / Docs collection");
  });

  it("clears the route's steps when it unmounts", () => {
    const { rerender } = render(
      <BreadcrumbProvider base={[{ label: "Acme" }]}>
        <BreadcrumbExtra items={[{ label: "Page A" }]} />
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByTestId("breadcrumb").textContent).toBe("Acme / Page A");

    rerender(
      <BreadcrumbProvider base={[{ label: "Acme" }]}>
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByTestId("breadcrumb").textContent).toBe("Acme");
  });

  // A caller building a fresh array each render is the normal case, and the
  // effect keys on the labels precisely so that it settles instead of looping.
  it("settles when the route passes a new array identity every render", () => {
    let renders = 0;
    function Unstable() {
      renders++;
      useBreadcrumbExtra([{ label: "Leaf", onClick: () => {} }]);
      return null;
    }
    render(
      <BreadcrumbProvider base={[{ label: "Acme" }]}>
        <Unstable />
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByTestId("breadcrumb").textContent).toBe("Acme / Leaf");
    expect(renders).toBeLessThan(10);
  });
});

// ---------------------------------------------------------------------------
// 12. StreamContext
// ---------------------------------------------------------------------------

import { StreamProvider, useStream } from "../context/StreamContext";

function StreamDisplay() {
  const { activeStream, setActiveStream } = useStream();
  return (
    <div>
      <span data-testid="stream">{activeStream}</span>
      <button onClick={() => setActiveStream("feature-1")}>Switch</button>
    </div>
  );
}

describe("StreamContext", () => {
  it("defaults to 'main'", () => {
    render(
      <StreamProvider>
        <StreamDisplay />
      </StreamProvider>,
    );
    expect(screen.getByTestId("stream").textContent).toBe("main");
  });

  it("accepts initialStream prop", () => {
    render(
      <StreamProvider initialStream="develop">
        <StreamDisplay />
      </StreamProvider>,
    );
    expect(screen.getByTestId("stream").textContent).toBe("develop");
  });

  it("updates active stream on setActiveStream", async () => {
    render(
      <StreamProvider>
        <StreamDisplay />
      </StreamProvider>,
    );
    await userEvent.click(screen.getByText("Switch"));
    expect(screen.getByTestId("stream").textContent).toBe("feature-1");
  });

  it("calls onStreamChange callback", async () => {
    const onChange = vi.fn();
    render(
      <StreamProvider onStreamChange={onChange}>
        <StreamDisplay />
      </StreamProvider>,
    );
    await userEvent.click(screen.getByText("Switch"));
    expect(onChange).toHaveBeenCalledWith("feature-1");
  });

  it("falls back to 'main' when empty string is set", async () => {
    function EmptySwitch() {
      const { activeStream, setActiveStream } = useStream();
      return (
        <div>
          <span data-testid="stream">{activeStream}</span>
          <button onClick={() => setActiveStream("")}>Clear</button>
        </div>
      );
    }
    render(
      <StreamProvider>
        <EmptySwitch />
      </StreamProvider>,
    );
    await userEvent.click(screen.getByText("Clear"));
    expect(screen.getByTestId("stream").textContent).toBe("main");
  });
});
