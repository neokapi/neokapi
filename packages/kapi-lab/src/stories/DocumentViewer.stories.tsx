import type { Meta, StoryObj } from "@storybook/react-vite";
import { DocumentViewer } from "@neokapi/ui-primitives/preview";
import { docxTree, jsonTree, pptxTree, doclangTree } from "./previewFixtures";

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
      className="max-w-2xl"
    />
  ),
};

export const EntryList: Story = {
  name: "Entry list (json)",
  render: () => <DocumentViewer tree={jsonTree} filename="messages.json" className="max-w-2xl" />,
};

// A DocLang/Docling document carries the WS1 structural layer (roles + page
// geometry), so the viewer surfaces the extra Structure (outline) and Layout
// (spatial) tabs alongside Preview/Blocks.
export const Structured: Story = {
  name: "Structured (DocLang: + Structure & Layout tabs)",
  render: () => (
    <DocumentViewer tree={doclangTree} filename="report.dclg.xml" className="max-w-2xl" />
  ),
};

export const StructureTab: Story = {
  name: "Structured · Structure tab first",
  render: () => (
    <DocumentViewer
      tree={doclangTree}
      filename="report.dclg.xml"
      defaultTab="structure"
      className="max-w-2xl"
    />
  ),
};

export const LayoutTab: Story = {
  name: "Structured · Layout tab first",
  render: () => (
    <DocumentViewer
      tree={doclangTree}
      filename="report.dclg.xml"
      defaultTab="layout"
      className="max-w-2xl"
    />
  ),
};

export const NoBytes: Story = {
  name: "No download bytes",
  render: () => <DocumentViewer tree={jsonTree} filename="messages.json" className="max-w-2xl" />,
};

// extraTabs appends host-supplied pills after a divider — the convert lab uses
// this for one pill per output format, each re-serializing the same model.
export const OutputFormatPills: Story = {
  name: "Output-format pills (extraTabs)",
  render: () => (
    <DocumentViewer
      tree={doclangTree}
      filename="report.dclg.xml"
      className="max-w-2xl"
      extraTabs={[
        {
          value: "out:markdown",
          label: "Markdown",
          content: (
            <pre className="rounded-md border bg-muted/30 p-3 text-xs"># Report{"\n\n"}Body…</pre>
          ),
        },
        {
          value: "out:json",
          label: "JSON",
          content: <pre className="rounded-md border bg-muted/30 p-3 text-xs">{"[ … ]"}</pre>,
        },
      ]}
    />
  ),
};
