import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { CollectionRail } from "../../components/CollectionRail";
import type { CollectionInfo } from "../../types/api";

const meta: Meta<typeof CollectionRail> = {
  title: "Workspace/Collections/CollectionRail",
  component: CollectionRail,
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="w-64 rounded-lg border border-border bg-card p-3">
        <Story />
      </div>
    ),
  ],
  args: {
    onSelectCollection: fn(),
    onCreateCollection: fn(),
    onEditCollection: fn(),
    onDeleteCollection: fn(),
  },
};
export default meta;
type Story = StoryObj<typeof CollectionRail>;

function collection(over: Partial<CollectionInfo>): CollectionInfo {
  return {
    id: "c",
    project_id: "p1",
    name: "collection",
    kind: "connected",
    item_label: "file",
    is_default: false,
    editable: false,
    item_count: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

/** One collection per surface, at a point: `<product>/<channel>`. */
function surface(product: string, channel: string, itemCount: number): CollectionInfo {
  return collection({
    id: `${product}-${channel}`,
    name: `${product}-${channel}`,
    coordinates: { product, channel },
    item_count: itemCount,
  });
}

/**
 * A project shaped like this repository's own: two products, a surface each for
 * the app, the docs, the emails and the rest. The rail groups by the coordinate
 * the recipe declares, so the products read as products rather than as twelve
 * unrelated names.
 */
export const TwoProducts: Story = {
  args: {
    activeCollectionId: "bowrain-app",
    collections: [
      surface("neokapi", "cli", 42),
      surface("neokapi", "engine", 18),
      surface("neokapi", "desktop", 88),
      surface("neokapi", "docs", 62),
      surface("bowrain", "app", 297),
      surface("bowrain", "ctrl", 21),
      surface("bowrain", "docs", 113),
      surface("bowrain", "email", 12),
      surface("bowrain", "landing", 18),
    ],
  },
};

/** A project whose collections declare no point: a flat list, no headers. */
export const NoCoordinates: Story = {
  args: {
    activeCollectionId: "c2",
    collections: [
      collection({
        id: "c1",
        name: "Marketing pages",
        kind: "uploaded",
        editable: true,
        item_count: 12,
      }),
      collection({
        id: "c2",
        name: "Product strings",
        kind: "uploaded",
        editable: true,
        item_count: 340,
      }),
    ],
  },
};

/** The default collection sits above the groups: it stands for all of them. */
export const WithDefaultAndUngrouped: Story = {
  args: {
    activeCollectionId: "all",
    collections: [
      collection({ id: "all", name: "All Items", is_default: true, item_count: 507 }),
      surface("bowrain", "app", 297),
      surface("bowrain", "docs", 113),
      collection({ id: "u1", name: "Uploads", kind: "uploaded", editable: true, item_count: 4 }),
    ],
  },
};
