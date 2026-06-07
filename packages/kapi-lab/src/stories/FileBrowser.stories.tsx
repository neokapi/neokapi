import type { Meta, StoryObj } from "@storybook/react-vite";
import { FileBrowser } from "@neokapi/ui-primitives/preview";
import type { BrowserFile } from "@neokapi/ui-primitives/preview";
import { ALL_TREES } from "./previewFixtures";

// FileBrowser shows many files across formats in list or grid views (a toggle).
// Each item is a small FormatPreview thumbnail; selecting one opens it inline in
// a DocumentViewer (or via the onOpen callback so a host can route it).

const meta: Meta<typeof FileBrowser> = {
  title: "Lab/PreviewKit/FileBrowser",
  component: FileBrowser,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof FileBrowser>;

const fakeBytes = (n: number) => new Uint8Array(n);
const files: BrowserFile[] = ALL_TREES.map((f, i) => ({
  ...f,
  bytes: fakeBytes(900 + i * 1300),
}));

export const Grid: Story = {
  render: () => <FileBrowser files={files} defaultView="grid" />,
};

export const ListView: Story = {
  name: "List",
  render: () => <FileBrowser files={files} defaultView="list" />,
};

export const Callback: Story = {
  name: "onOpen callback (no inline viewer)",
  render: () => <FileBrowser files={files} onOpen={(f) => window.alert(`Open ${f.filename}`)} />,
};
