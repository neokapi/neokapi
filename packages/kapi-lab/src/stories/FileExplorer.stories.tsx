import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import FileExplorer from "../FileExplorer";
import FileSelectorField from "../FileSelectorField";
import { useFileLibrary } from "../fileLibrary";
import type { FileSelection } from "../fileLibrary";

const meta: Meta = {
  title: "Lab/File Explorer",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

function ExplorerDemo({ multiple, initial }: { multiple?: boolean; initial: FileSelection }) {
  const library = useFileLibrary({});
  const [sel, setSel] = useState<FileSelection>(initial);
  return (
    <div className="max-w-3xl">
      <FileExplorer
        library={library}
        selection={sel}
        onSelectionChange={setSel}
        multiple={multiple}
      />
      <pre className="mt-3 rounded-md bg-muted p-2 text-xs">{JSON.stringify(sel, null, 2)}</pre>
    </div>
  );
}

export const SingleSelect: Story = {
  render: () => (
    <ExplorerDemo multiple={false} initial={{ mode: "single", paths: ["messages.json"] }} />
  ),
};

export const MultiSelect: Story = {
  render: () => (
    <ExplorerDemo multiple initial={{ mode: "multi", paths: ["messages.json", "app.xliff"] }} />
  ),
};

export const GlobSelect: Story = {
  render: () => <ExplorerDemo multiple initial={{ mode: "glob", paths: [], pattern: "*.json" }} />,
};

function FieldDemo({ multiple }: { multiple?: boolean }) {
  const library = useFileLibrary({});
  const [sel, setSel] = useState<FileSelection>(
    multiple
      ? { mode: "glob", paths: [], pattern: "**/*.json" }
      : { mode: "single", paths: ["messages.json"] },
  );
  return (
    <div className="max-w-xl">
      <FileSelectorField
        label={multiple ? "Inputs" : "File"}
        library={library}
        selection={sel}
        onSelectionChange={setSel}
        multiple={multiple}
      />
    </div>
  );
}

export const CompactFieldSingle: Story = {
  name: "Compact field — single",
  render: () => <FieldDemo />,
};

export const CompactFieldGlob: Story = {
  name: "Compact field — glob",
  render: () => <FieldDemo multiple />,
};
