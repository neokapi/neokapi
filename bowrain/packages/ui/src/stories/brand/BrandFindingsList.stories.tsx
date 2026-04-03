import type { Meta, StoryObj } from "@storybook/react-vite";
import { BrandFindingsList } from "../../brand/BrandFindingsList";
import { sampleFindings } from "./fixtures";

const meta: Meta<typeof BrandFindingsList> = {
  title: "Brand/BrandFindingsList",
  component: BrandFindingsList,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BrandFindingsList>;

/** Multiple findings with mixed severities. */
export const MixedSeverities: Story = {
  args: { findings: sampleFindings },
};

/** No findings — fully compliant. */
export const NoFindings: Story = {
  args: { findings: [] },
};

/** Single critical finding. */
export const SingleCritical: Story = {
  args: { findings: [sampleFindings[2]] },
};

/** Only minor findings. */
export const MinorOnly: Story = {
  args: {
    findings: sampleFindings.filter((f) => f.severity === "minor"),
  },
};
