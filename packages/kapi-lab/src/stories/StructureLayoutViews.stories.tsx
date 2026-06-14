import type { Meta, StoryObj } from "@storybook/react-vite";
import { StructureView, LayoutView } from "@neokapi/ui-primitives/preview";
import { doclangTree } from "./previewFixtures";

// The WS5 editor views, standalone. StructureView is the reflowable outline
// (roles, reading order, nesting); LayoutView is the spatial page view (bounding
// boxes by reading order). Both are pure projections of the ContentTree's WS1
// structural layer — a DocLang doc, a Docling PDF, or a DOCX outline the same.

const meta: Meta = {
  title: "Lab/PreviewKit/StructureLayout",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const Structure: Story = {
  render: () => (
    <div className="max-w-xl rounded-lg border p-3">
      <StructureView tree={doclangTree} />
    </div>
  ),
};

export const StructureTargetSide: Story = {
  name: "Structure (French target)",
  render: () => (
    <div className="max-w-xl rounded-lg border p-3">
      <StructureView tree={doclangTree} side="fr-FR" />
    </div>
  ),
};

export const Layout: Story = {
  render: () => (
    <div className="rounded-lg border p-3">
      <LayoutView tree={doclangTree} />
    </div>
  ),
};
