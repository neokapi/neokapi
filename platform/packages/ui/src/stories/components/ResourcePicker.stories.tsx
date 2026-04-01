import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ResourcePicker, type ResourcePickerProps } from "../../components/ResourcePicker";

const meta: Meta<typeof ResourcePicker> = {
  title: "Workspace/Resources/ResourcePicker",
  component: ResourcePicker,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 380, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ResourcePicker>;

const sampleTMs = [
  { name: "project-memory", entryCount: 12450 },
  { name: "legacy-tm", entryCount: 85000 },
  { name: "small-test", entryCount: 120 },
];

const sampleTermbases = [
  { name: "glossary", entryCount: 340 },
  { name: "brand-terms", entryCount: 52 },
];

const sampleSRX = [
  { name: "custom-rules" },
  { name: "japanese-rules" },
];

function StatefulPicker(props: Omit<ResourcePickerProps, "onChange">) {
  const [value, setValue] = useState(props.value);
  return <ResourcePicker {...props} value={value} onChange={setValue} />;
}

export const Default: Story = {
  render: () => (
    <StatefulPicker
      value=""
      label="Translation Memory"
      resourceKind="tm"
      resources={sampleTMs}
    />
  ),
};

export const WithNamedResource: Story = {
  render: () => (
    <StatefulPicker
      value="tm:project-memory"
      label="Translation Memory"
      resourceKind="tm"
      resources={sampleTMs}
      resolvedPath="~/.config/kapi/tm/project-memory.db"
    />
  ),
};

export const WithFilePath: Story = {
  render: () => (
    <StatefulPicker
      value="./resources/my-tm.db"
      label="Translation Memory"
      resourceKind="tm"
      resources={sampleTMs}
      resolvedPath="/home/user/project/resources/my-tm.db"
    />
  ),
};

export const TermbaseKind: Story = {
  render: () => (
    <StatefulPicker
      value="termbase:glossary"
      label="Terminology"
      resourceKind="termbase"
      resources={sampleTermbases}
      resolvedPath="~/.config/kapi/termbases/glossary.db"
    />
  ),
};

export const SrxKind: Story = {
  render: () => (
    <StatefulPicker
      value=""
      label="Segmentation Rules"
      resourceKind="srx"
      resources={sampleSRX}
    />
  ),
};

export const FileOnly: Story = {
  render: () => (
    <StatefulPicker
      value="./scripts/transform.xslt"
      label="XSLT Stylesheet"
      resolvedPath="/home/user/project/scripts/transform.xslt"
    />
  ),
};

export const DirectoryType: Story = {
  render: () => (
    <StatefulPicker
      value=""
      label="TM Directory"
      resourceKind="tm"
      pathType="directory"
      resources={sampleTMs}
    />
  ),
};

export const OutputRole: Story = {
  render: () => (
    <StatefulPicker
      value=""
      label="Report Output"
      role="output"
      placeholder="Leave empty for auto-placement"
    />
  ),
};

export const Disabled: Story = {
  render: () => (
    <StatefulPicker
      value="tm:project-memory"
      label="Translation Memory"
      resourceKind="tm"
      resources={sampleTMs}
      disabled
    />
  ),
};
