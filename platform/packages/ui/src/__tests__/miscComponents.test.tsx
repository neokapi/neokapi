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

import { FileProgressTable } from "../components/FileProgressTable";
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
    expect(screen.getByText("strings.json")).toBeInTheDocument();
    expect(screen.getByText("errors.json")).toBeInTheDocument();
    expect(screen.getByText("fr-FR")).toBeInTheDocument();
    expect(screen.getByText("de-DE")).toBeInTheDocument();
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
    expect(within(rows[1]).getByText("alpha.json")).toBeInTheDocument();

    // Click File column header to toggle to desc
    const fileHeader = screen.getByRole("columnheader", { name: /^File/ });
    await userEvent.click(fileHeader);
    const rowsAfter = screen.getAllByRole("row");
    expect(within(rowsAfter[1]).getByText("beta.json")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 4. CollectionHeatmap
// ---------------------------------------------------------------------------

import { CollectionHeatmap } from "../components/CollectionHeatmap";
import type { CollectionTranslationStats } from "../types/api";

describe("CollectionHeatmap", () => {
  it("renders collection names and locale columns", () => {
    const stats: CollectionTranslationStats[] = [
      {
        collection_id: "c1",
        collection_name: "UI Strings",
        item_count: 5,
        block_count: 50,
        word_count: 200,
        locales: [
          {
            locale: "fr-FR",
            translated_blocks: 40,
            total_blocks: 50,
            translated_words: 160,
            total_words: 200,
            percentage: 80,
          },
        ],
      },
    ];
    render(<CollectionHeatmap collectionStats={stats} locales={["fr-FR", "ja-JP"]} />);
    expect(screen.getByText("UI Strings")).toBeInTheDocument();
    expect(screen.getByText("fr-FR")).toBeInTheDocument();
    expect(screen.getByText("ja-JP")).toBeInTheDocument();
    // 80% for fr-FR, 0% for ja-JP
    expect(screen.getByText("80%")).toBeInTheDocument();
    expect(screen.getByText("0%")).toBeInTheDocument();
  });

  it("shows 'Default' for collections with empty name", () => {
    const stats: CollectionTranslationStats[] = [
      {
        collection_id: "c1",
        collection_name: "",
        item_count: 1,
        block_count: 10,
        word_count: 50,
        locales: [],
      },
    ];
    render(<CollectionHeatmap collectionStats={stats} locales={[]} />);
    expect(screen.getByText("Default")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 5. CollectionTabs
// ---------------------------------------------------------------------------

import { CollectionTabs } from "../components/CollectionTabs";
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

describe("CollectionTabs", () => {
  it("renders tab labels with item counts", () => {
    const collections = [
      makeCollection({ id: "c1", name: "Strings", item_count: 12 }),
      makeCollection({ id: "c2", name: "Docs", item_count: 5 }),
    ];
    render(
      <CollectionTabs
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

  it("shows 'All Items' for default collection", () => {
    const collections = [
      makeCollection({ id: "c1", is_default: true, name: "Default" }),
      makeCollection({ id: "c2", name: "Other" }),
    ];
    render(
      <CollectionTabs
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
      />,
    );
    expect(screen.getByText("All Items")).toBeInTheDocument();
  });

  it("calls onSelectCollection when a tab is clicked", async () => {
    const onSelect = vi.fn();
    const collections = [
      makeCollection({ id: "c1", name: "First" }),
      makeCollection({ id: "c2", name: "Second" }),
    ];
    render(
      <CollectionTabs
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={onSelect}
      />,
    );
    await userEvent.click(screen.getByText("Second"));
    expect(onSelect).toHaveBeenCalledWith("c2");
  });

  it("returns null for single collection without onCreate", () => {
    const collections = [makeCollection()];
    const { container } = render(
      <CollectionTabs
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
      />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("shows create button when onCreateCollection is provided", () => {
    const collections = [makeCollection()];
    render(
      <CollectionTabs
        collections={collections}
        activeCollectionId="c1"
        onSelectCollection={() => {}}
        onCreateCollection={() => {}}
      />,
    );
    // The create button contains "Collection" text (hidden on sm)
    expect(screen.getByText("Collection")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 6. WordCountChart — recharts uses SVG which jsdom does not fully support.
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
// 7. ActivityTaskIndicators
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
    summary: "extracted 50 strings",
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
      makeTask({ id: "t2", title: "QA German" }),
    ];
    render(<TaskIndicator tasks={tasks} />);
    await userEvent.click(screen.getByTitle("My tasks"));
    expect(screen.getByText("Review French")).toBeInTheDocument();
    expect(screen.getByText("QA German")).toBeInTheDocument();
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
// 8. FilterBar — complex component, test key behaviors
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
// 9. WorkspaceSwitcher — requires SidebarProvider context
// ---------------------------------------------------------------------------

import { WorkspaceSwitcher } from "../components/WorkspaceSwitcher";
import { SidebarProvider } from "../components/ui/sidebar";
import { TooltipProvider } from "../components/ui/tooltip";
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

import { BreadcrumbProvider, useBreadcrumb, useSetBreadcrumb } from "../context/BreadcrumbContext";

function BreadcrumbDisplay() {
  const breadcrumb = useBreadcrumb();
  return <div data-testid="breadcrumb">{breadcrumb}</div>;
}

function BreadcrumbSetter({ node }: { node: React.ReactNode }) {
  useSetBreadcrumb(node);
  return null;
}

describe("BreadcrumbContext", () => {
  it("provides null breadcrumb by default", () => {
    render(
      <BreadcrumbProvider>
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByTestId("breadcrumb").textContent).toBe("");
  });

  it("sets and reads breadcrumb", () => {
    render(
      <BreadcrumbProvider>
        <BreadcrumbSetter node={<span>Home / Project</span>} />
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByText("Home / Project")).toBeInTheDocument();
  });

  it("clears breadcrumb on unmount", () => {
    const { rerender } = render(
      <BreadcrumbProvider>
        <BreadcrumbSetter node={<span>Page A</span>} />
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.getByText("Page A")).toBeInTheDocument();

    // Rerender without the setter — should clear
    rerender(
      <BreadcrumbProvider>
        <BreadcrumbDisplay />
      </BreadcrumbProvider>,
    );
    expect(screen.queryByText("Page A")).not.toBeInTheDocument();
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
