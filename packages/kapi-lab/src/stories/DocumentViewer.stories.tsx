import type { Meta, StoryObj } from "@storybook/react-vite";
import DocumentViewer from "../DocumentViewer";
import { docxTree, jsonTree, pptxTree } from "./previewFixtures";

// DocumentViewer wraps FormatPreview with view-switching tabs (Preview · Blocks ·
// Stats · Download), a source↔target locale toggle, a transition selector and an
// annotations toggle.

const meta: Meta<typeof DocumentViewer> = {
  title: "Lab/PreviewKit/DocumentViewer",
  component: DocumentViewer,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DocumentViewer>;

const docxBytes = new TextEncoder().encode("PK (a fake docx zip for download)");

export const Slides: Story = {
  render: () => (
    <DocumentViewer tree={pptxTree} filename="deck.pptx" bytes={docxBytes} className="max-w-2xl" />
  ),
};

export const Document: Story = {
  render: () => (
    <DocumentViewer
      tree={docxTree}
      filename="welcome.docx"
      bytes={docxBytes}
      defaultTransition="typewriter"
      className="max-w-2xl"
    />
  ),
};

export const EntryList: Story = {
  name: "Entry list (json)",
  render: () => <DocumentViewer tree={jsonTree} filename="messages.json" className="max-w-2xl" />,
};

export const NoBytes: Story = {
  name: "No download bytes",
  render: () => <DocumentViewer tree={jsonTree} filename="messages.json" className="max-w-2xl" />,
};
