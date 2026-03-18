import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoConfigPanel } from "../../components/bravo/BravoConfigPanel";
import { sampleConfig, sampleTools } from "./fixtures";

const meta: Meta<typeof BravoConfigPanel> = {
  title: "Bravo/BravoConfigPanel",
  component: BravoConfigPanel,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, border: "1px solid #ddd", borderRadius: 8, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BravoConfigPanel>;

export const Default: Story = {
  args: {
    config: sampleConfig,
    tools: sampleTools,
    onSave: fn(),
  },
};

export const Disabled: Story = {
  args: {
    config: { ...sampleConfig, enabled: false },
    tools: sampleTools,
    onSave: fn(),
  },
};

export const AllToolsDenied: Story = {
  args: {
    config: {
      ...sampleConfig,
      denied_tools: sampleTools.map((t) => t.name),
      require_approval: [],
    },
    tools: sampleTools,
    onSave: fn(),
  },
};

export const AllToolsRequireApproval: Story = {
  args: {
    config: {
      ...sampleConfig,
      denied_tools: [],
      require_approval: sampleTools.map((t) => t.name),
    },
    tools: sampleTools,
    onSave: fn(),
  },
};

export const ManyTools: Story = {
  args: {
    config: {
      ...sampleConfig,
      denied_tools: ["execute_script"],
      require_approval: ["connector_push", "connector_pull", "create_version"],
    },
    tools: [
      ...sampleTools,
      { name: "create_version", require_approval: true },
      { name: "list_streams", require_approval: false },
      { name: "diff_streams", require_approval: false },
      { name: "merge_stream", require_approval: true },
      { name: "update_block", require_approval: false },
      { name: "term_search", require_approval: false },
      { name: "term_add", require_approval: false },
      { name: "tm_import", require_approval: false },
      { name: "execute_script", require_approval: false },
      { name: "check_vocabulary", require_approval: false },
      { name: "list_profiles", require_approval: false },
    ],
    onSave: fn(),
  },
};

export const Saving: Story = {
  args: {
    config: sampleConfig,
    tools: sampleTools,
    onSave: fn(),
    saving: true,
  },
};

export const NoTools: Story = {
  args: {
    config: sampleConfig,
    tools: [],
    onSave: fn(),
  },
};
