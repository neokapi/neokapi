/**
 * `sourceLocale` threading from ProjectView/CollectionItemsView through
 * FilePreview to DocumentPreview. Scoped to its own file (rather than
 * extending FilePreview.test.tsx) because it stubs out DocumentPreview
 * entirely to capture the prop it receives — FilePreview.test.tsx's other
 * tests rely on DocumentPreview's real iframe rendering, and a shared mock
 * would break them.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { QueryClientProvider, QueryClient } from "@tanstack/react-query";

import { ProjectView } from "../components/ProjectView";
import { CollectionItemsView } from "../components/collections/CollectionItemsView";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import { createMockAdapter } from "../stories/mock-adapter";
import { sampleProject } from "../stories/fixtures";
import type {
  ItemTranslationStats,
  LocaleTranslationStats,
  ProjectInfo,
  Workspace,
} from "../types/api";

const capturedProps: { sourceLocale?: string }[] = [];

vi.mock("../components/editor/DocumentPreview", () => ({
  DocumentPreview: (props: { sourceLocale?: string }) => {
    capturedProps.push(props);
    return null;
  },
}));

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function wrap(ui: React.ReactNode) {
  const adapter = createMockAdapter();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
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

describe("ProjectView → FilePreview → DocumentPreview — sourceLocale", () => {
  it("falls back to the project's default source language", async () => {
    capturedProps.length = 0;
    const project: ProjectInfo = {
      ...sampleProject,
      items: [{ name: "docs/releasing.md", format: "markdown", block_count: 7, word_count: 120 }],
      collections: [],
    };
    render(
      wrap(
        <ProjectView
          project={project}
          onBack={vi.fn()}
          onOpenFile={vi.fn()}
          onUploadFiles={vi.fn()}
          onRemoveFile={vi.fn()}
          preview={{ itemName: "docs/releasing.md", onOpen: vi.fn(), onClose: vi.fn() }}
        />,
      ),
    );
    await screen.findByTestId("file-preview");
    expect(capturedProps.at(-1)?.sourceLocale).toBe(sampleProject.default_source_language);
  });
});

describe("CollectionItemsView → FilePreview → DocumentPreview — sourceLocale", () => {
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

  it("carries the binding's sourceLocale through — two hops from where it's known", async () => {
    capturedProps.length = 0;
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
            sourceLocale: "ar-EG",
          }}
        />,
      ),
    );
    await screen.findByTestId("file-preview");
    expect(capturedProps.at(-1)?.sourceLocale).toBe("ar-EG");
  });

  it("is undefined, not a wrong guess, when the binding carries no sourceLocale", async () => {
    capturedProps.length = 0;
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
      ),
    );
    await screen.findByTestId("file-preview");
    expect(capturedProps.at(-1)?.sourceLocale).toBeUndefined();
  });
});
