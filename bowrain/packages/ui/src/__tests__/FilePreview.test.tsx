/**
 * Component tests for the item preview: a row opens the reading rather than an
 * editor, a row's own actions do not, the editors are reached only from the
 * preview's explicit actions, and the document is one paged, cached query.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { FilePreview } from "../components/FilePreview";
import { ProjectView } from "../components/ProjectView";
import { CollectionItemsView } from "../components/collections/CollectionItemsView";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import { createMockAdapter, type MockAdapter } from "../stories/mock-adapter";
import { sampleProject } from "../stories/fixtures";
import type {
  ItemTranslationStats,
  LocaleTranslationStats,
  ProjectInfo,
  Workspace,
} from "../types/api";

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function wrap(ui: React.ReactNode, adapter: MockAdapter, qc?: QueryClient) {
  const client = qc ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          <BreadcrumbProvider>{ui}</BreadcrumbProvider>
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>
  );
}

const itemStats: ItemTranslationStats[] = [
  {
    item_id: "i-1",
    item_name: "docs/releasing.md",
    format: "markdown",
    word_count: 120,
    locales: [{ locale: "fr-FR", percentage: 40, translated_blocks: 2, total_blocks: 5 }],
  },
];

const localeStats: LocaleTranslationStats[] = [
  {
    locale: "fr-FR",
    display_name: "French",
    total_words: 120,
    translated_words: 48,
    total_blocks: 5,
    translated_blocks: 2,
    percentage: 40,
  },
];

describe("FilePreview — the sheet reads an item and hands off deliberately", () => {
  it("stays closed until an item is named, then shows its name and format", async () => {
    const adapter = createMockAdapter();
    const { rerender } = render(
      wrap(
        <FilePreview projectId="proj-1" itemName={null} format="markdown" onClose={vi.fn()} />,
        adapter,
      ),
    );
    expect(screen.queryByTestId("file-preview")).toBeNull();

    rerender(
      wrap(
        <FilePreview
          projectId="proj-1"
          itemName="docs/releasing.md"
          format="markdown"
          onClose={vi.fn()}
        />,
        adapter,
      ),
    );
    const sheet = await screen.findByTestId("file-preview");
    expect(sheet).toHaveTextContent("docs/releasing.md");
    expect(sheet).toHaveTextContent("markdown");
  });

  it("reads the document as one paged query per locale, not one per block", async () => {
    const adapter = createMockAdapter();
    const list = vi.spyOn(adapter, "getFileBlocks");
    const blockHTML = vi.spyOn(adapter, "renderBlockHTML");

    render(
      wrap(
        <FilePreview
          projectId="proj-1"
          itemName="docs/releasing.md"
          targetLocales={["fr-FR"]}
          onClose={vi.fn()}
        />,
        adapter,
      ),
    );

    await waitFor(() => expect(list).toHaveBeenCalled());
    expect(list.mock.calls[0][4]).toMatchObject({ locale: "fr-FR", limit: 500, offset: 0 });
    expect(blockHTML).not.toHaveBeenCalled();
  });

  it("serves a second mount of the same item from the cache", async () => {
    const adapter = createMockAdapter();
    const list = vi.spyOn(adapter, "getFileBlocks");
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const props = {
      projectId: "proj-1",
      itemName: "docs/releasing.md",
      targetLocales: ["fr-FR"],
      onClose: vi.fn(),
    };

    render(wrap(<FilePreview {...props} />, adapter, qc));
    await waitFor(() => expect(list).toHaveBeenCalled());
    const first = list.mock.calls.length;

    render(wrap(<FilePreview {...props} />, adapter, qc));
    await waitFor(() => expect(screen.getAllByTestId("file-preview").length).toBe(2));
    expect(list.mock.calls.length).toBe(first);
  });

  it("offers only the actions the consumer wired, and passes the item name back", async () => {
    const adapter = createMockAdapter();
    const onOpenTranslate = vi.fn();
    render(
      wrap(
        <FilePreview
          projectId="proj-1"
          itemName="docs/releasing.md"
          onClose={vi.fn()}
          onOpenTranslate={onOpenTranslate}
        />,
        adapter,
      ),
    );

    expect(screen.queryByTestId("file-preview-review")).toBeNull();
    expect(screen.queryByTestId("file-preview-pre-process")).toBeNull();

    await userEvent.click(screen.getByTestId("file-preview-translate"));
    expect(onOpenTranslate).toHaveBeenCalledWith("docs/releasing.md");
  });

  it("closes on Escape", async () => {
    const adapter = createMockAdapter();
    const onClose = vi.fn();
    render(
      wrap(
        <FilePreview projectId="proj-1" itemName="docs/releasing.md" onClose={onClose} />,
        adapter,
      ),
    );
    await screen.findByTestId("file-preview");
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("names the single target language on the toggle, and offers no selector for it", async () => {
    const adapter = createMockAdapter();
    render(
      wrap(
        <FilePreview
          projectId="proj-1"
          itemName="docs/releasing.md"
          targetLocales={["fr-FR"]}
          onClose={vi.fn()}
        />,
        adapter,
      ),
    );
    const toggle = await screen.findByTestId("file-preview-side");
    expect(toggle).toHaveTextContent("Source");
    expect(screen.queryByTestId("file-preview-locale")).toBeNull();
  });

  it("offers a locale selector once the project targets more than one language", async () => {
    const adapter = createMockAdapter();
    render(
      wrap(
        <FilePreview
          projectId="proj-1"
          itemName="docs/releasing.md"
          targetLocales={["fr-FR", "de-DE"]}
          onClose={vi.fn()}
        />,
        adapter,
      ),
    );
    expect(await screen.findByTestId("file-preview-locale")).toBeInTheDocument();
  });

  it("has no reading toggle when the project targets no language", async () => {
    const adapter = createMockAdapter();
    render(
      wrap(
        <FilePreview
          projectId="proj-1"
          itemName="docs/releasing.md"
          targetLocales={[]}
          onClose={vi.fn()}
        />,
        adapter,
      ),
    );
    await screen.findByTestId("file-preview");
    expect(screen.queryByTestId("file-preview-side")).toBeNull();
  });
});

describe("CollectionItemsView — a row opens the reading, not an editor", () => {
  it("fires onOpen with the item name and never the editor callback", async () => {
    const adapter = createMockAdapter();
    const onOpen = vi.fn();
    const onOpenItem = vi.fn();

    render(
      wrap(
        <CollectionItemsView
          title="All items"
          itemStats={itemStats}
          localeStats={localeStats}
          onBack={vi.fn()}
          onOpenItem={onOpenItem}
          preview={{
            projectId: "proj-1",
            itemName: null,
            onOpen,
            onClose: vi.fn(),
            targetLocales: ["fr-FR"],
          }}
        />,
        adapter,
      ),
    );

    await userEvent.click(screen.getByRole("button", { name: /releasing\.md/ }));
    expect(onOpen).toHaveBeenCalledWith("docs/releasing.md");
    expect(onOpenItem).not.toHaveBeenCalled();
  });

  it("shows the open item's format, taken from the row it was opened from", async () => {
    const adapter = createMockAdapter();
    render(
      wrap(
        <CollectionItemsView
          title="All items"
          itemStats={itemStats}
          localeStats={localeStats}
          onBack={vi.fn()}
          preview={{
            projectId: "proj-1",
            itemName: "docs/releasing.md",
            onOpen: vi.fn(),
            onClose: vi.fn(),
            targetLocales: ["fr-FR"],
          }}
        />,
        adapter,
      ),
    );
    expect(await screen.findByTestId("file-preview")).toHaveTextContent("markdown");
  });
});

describe("ProjectView — the row reads the file, the row's own action does not", () => {
  const project: ProjectInfo = {
    ...sampleProject,
    items: [{ name: "docs/releasing.md", format: "markdown", block_count: 7, word_count: 120 }],
    collections: [],
  };

  it("opens the preview from a click anywhere on the row", async () => {
    const adapter = createMockAdapter();
    const onOpen = vi.fn();
    const onOpenFile = vi.fn();

    render(
      wrap(
        <ProjectView
          project={project}
          onBack={vi.fn()}
          onOpenFile={onOpenFile}
          onUploadFiles={vi.fn()}
          onRemoveFile={vi.fn()}
          preview={{ itemName: null, onOpen, onClose: vi.fn() }}
        />,
        adapter,
      ),
    );

    await userEvent.click(screen.getByTestId("file-row-docs/releasing.md"));
    expect(onOpen).toHaveBeenCalledWith("docs/releasing.md");
    expect(onOpenFile).not.toHaveBeenCalled();
  });

  it("does not open the preview when the row's remove action is clicked", async () => {
    const adapter = createMockAdapter();
    const onOpen = vi.fn();
    const onRemoveFile = vi.fn();

    render(
      wrap(
        <ProjectView
          project={project}
          onBack={vi.fn()}
          onOpenFile={vi.fn()}
          onUploadFiles={vi.fn()}
          onRemoveFile={onRemoveFile}
          preview={{ itemName: null, onOpen, onClose: vi.fn() }}
        />,
        adapter,
      ),
    );

    await userEvent.click(screen.getByTestId("remove-file-docs/releasing.md"));
    expect(onRemoveFile).toHaveBeenCalledWith("docs/releasing.md");
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("reads the collection from the binding, so the URL decides which is open", async () => {
    const adapter = createMockAdapter();
    const onSelect = vi.fn();
    const withCollections: ProjectInfo = {
      ...project,
      items: [
        { name: "a.json", format: "json", block_count: 1, word_count: 1, collection_id: "c1" },
        { name: "b.json", format: "json", block_count: 1, word_count: 1, collection_id: "c2" },
      ],
      collections: [
        {
          id: "c1",
          project_id: project.id,
          name: "bowrain-app",
          kind: "connected",
          item_label: "file",
          is_default: false,
          item_count: 1,
          coordinates: { product: "bowrain", channel: "app" },
          created_at: "",
          updated_at: "",
        },
        {
          id: "c2",
          project_id: project.id,
          name: "neokapi-cli",
          kind: "connected",
          item_label: "file",
          is_default: false,
          item_count: 1,
          coordinates: { product: "neokapi", channel: "cli" },
          created_at: "",
          updated_at: "",
        },
      ],
    };

    render(
      wrap(
        <ProjectView
          project={withCollections}
          onBack={vi.fn()}
          onOpenFile={vi.fn()}
          onUploadFiles={vi.fn()}
          onRemoveFile={vi.fn()}
          collection={{ id: "c2", onSelect }}
        />,
        adapter,
      ),
    );

    // The bound collection is the one being read — not the first one.
    expect(screen.getByTestId("file-row-b.json")).toBeInTheDocument();
    expect(screen.queryByTestId("file-row-a.json")).toBeNull();

    // Selecting reports outward rather than being kept here, so the consumer
    // can put it in the URL.
    await userEvent.click(screen.getByTestId("collection-c1"));
    expect(onSelect).toHaveBeenCalledWith("c1");
  });

  it("falls back to the first collection when the bound one is not in the project", () => {
    const adapter = createMockAdapter();
    const withCollections: ProjectInfo = {
      ...project,
      items: [
        { name: "a.json", format: "json", block_count: 1, word_count: 1, collection_id: "c1" },
      ],
      collections: [
        {
          id: "c1",
          project_id: project.id,
          name: "Uploads",
          kind: "uploaded",
          item_label: "file",
          is_default: false,
          item_count: 1,
          created_at: "",
          updated_at: "",
        },
      ],
    };

    render(
      wrap(
        <ProjectView
          project={withCollections}
          onBack={vi.fn()}
          onOpenFile={vi.fn()}
          onUploadFiles={vi.fn()}
          onRemoveFile={vi.fn()}
          collection={{ id: "gone", onSelect: vi.fn() }}
        />,
        adapter,
      ),
    );

    expect(screen.getByTestId("file-row-a.json")).toBeInTheDocument();
  });

  it("keeps opening the editor directly when no preview is bound", async () => {
    const adapter = createMockAdapter();
    const onOpenFile = vi.fn();

    render(
      wrap(
        <ProjectView
          project={project}
          onBack={vi.fn()}
          onOpenFile={onOpenFile}
          onUploadFiles={vi.fn()}
          onRemoveFile={vi.fn()}
        />,
        adapter,
      ),
    );

    await userEvent.click(screen.getByTestId("open-file-docs/releasing.md"));
    expect(onOpenFile).toHaveBeenCalledWith("docs/releasing.md");
    expect(screen.queryByTestId("file-preview")).toBeNull();
  });
});

// The offer has to be on the page a reviewer opens a file from, not only on the
// project page: `?collection=` lands here, and a toggle that appeared on one
// route and not the other would read as the feature being broken.
describe("CollectionItemsView — in-context reading follows the collection", () => {
  // The offer needs a deployment with an embed origin to frame through: the
  // application's content policy names exactly that origin, so without one
  // there is nothing a frame could legally load. Stated here rather than
  // assumed, because a deployment without it must offer nothing at all.
  // The story index the collection's host publishes, served here rather than
  // fetched. Which items have a reading is decided by what this carries, so a
  // test that stubbed nothing would only ever be testing the absence.
  const STORY_INDEX = {
    entries: {
      "components-reachpanel--default": {
        id: "components-reachpanel--default",
        title: "Components/ReachPanel",
        name: "Default",
        importPath: "./src/brand-hub/experiments/ReachPanel.stories.tsx",
      },
    },
  };

  beforeEach(() => {
    vi.stubEnv("VITE_EMBED_ORIGIN", "https://embed.example.dev");
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) =>
        String(input).includes("/preview/index")
          ? Promise.resolve(new Response(JSON.stringify(STORY_INDEX)))
          : Promise.resolve(new Response("{}", { status: 404 })),
      ),
    );
  });
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  const rollup = (preview: { preview_kind?: string; preview_url?: string }) => ({
    collection_id: "c1",
    collection_name: "bowrain-app",
    item_count: 1,
    block_count: 1,
    word_count: 1,
    locales: localeStats,
    ...preview,
  });

  // A component catalog, because only one can have a story: the join runs on
  // the `file` property, which one reader writes.
  const STORIED_ITEM = "ui/src/brand-hub/experiments/ReachPanel.kbf.json";

  const storiedStats: ItemTranslationStats[] = [
    {
      ...itemStats[0],
      item_id: "i-2",
      item_name: STORIED_ITEM,
      format: "kbf",
      source_path: "../ui/src/brand-hub/experiments/ReachPanel.tsx",
    },
  ];

  const openPreview = {
    projectId: "proj-1",
    itemName: STORIED_ITEM,
    onOpen: vi.fn(),
    onClose: vi.fn(),
    targetLocales: ["fr-FR"],
  };

  it("offers it when a story in the collection's host renders the item", async () => {
    render(
      wrap(
        <CollectionItemsView
          collection={rollup({
            preview_kind: "storybook",
            preview_url: "https://neokapi.github.io/storybook/bowrain/",
          })}
          title="bowrain-app"
          itemStats={storiedStats}
          localeStats={localeStats}
          onBack={vi.fn()}
          preview={openPreview}
        />,
        createMockAdapter(),
      ),
    );

    expect(await screen.findByTestId("file-preview-reading")).toBeInTheDocument();
  });

  it("offers nothing when the collection declares no host", async () => {
    render(
      wrap(
        <CollectionItemsView
          collection={rollup({})}
          title="neokapi-docs"
          itemStats={storiedStats}
          localeStats={localeStats}
          onBack={vi.fn()}
          preview={openPreview}
        />,
        createMockAdapter(),
      ),
    );

    await screen.findByRole("dialog");
    expect(screen.queryByTestId("file-preview-reading")).not.toBeInTheDocument();
  });

  // A kind this client cannot resolve a view within is not a Storybook at an
  // unfamiliar address.
  it("offers nothing for a kind it cannot resolve", async () => {
    render(
      wrap(
        <CollectionItemsView
          collection={rollup({ preview_kind: "ladle", preview_url: "https://example.dev/ladle/" })}
          title="bowrain-app"
          itemStats={storiedStats}
          localeStats={localeStats}
          onBack={vi.fn()}
          preview={openPreview}
        />,
        createMockAdapter(),
      ),
    );

    await screen.findByRole("dialog");
    expect(screen.queryByTestId("file-preview-reading")).not.toBeInTheDocument();
  });

  // A collection declaring a host is a claim about the collection, not about
  // every item in it. Offered here, the toggle led to "No story renders this
  // item's components" — an offer made and then withdrawn, which reads as the
  // feature being broken rather than as a story being unwritten.
  it("offers nothing for an item the host publishes no story for", async () => {
    render(
      wrap(
        <CollectionItemsView
          collection={rollup({
            preview_kind: "storybook",
            preview_url: "https://neokapi.github.io/storybook/bowrain/",
          })}
          title="bowrain-app"
          itemStats={[...storiedStats, ...itemStats]}
          localeStats={localeStats}
          onBack={vi.fn()}
          preview={{ ...openPreview, itemName: itemStats[0].item_name }}
        />,
        createMockAdapter(),
      ),
    );

    await screen.findByRole("dialog");
    expect(screen.queryByTestId("file-preview-reading")).not.toBeInTheDocument();
  });
});
