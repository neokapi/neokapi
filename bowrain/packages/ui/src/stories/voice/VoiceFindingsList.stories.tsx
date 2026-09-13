import type { Meta, StoryObj } from "@storybook/react-vite";
import { VoiceFindingsList } from "../../voice/VoiceFindingsList";
import { sampleFindings } from "./fixtures";

const meta: Meta<typeof VoiceFindingsList> = {
  title: "Brand/VoiceFindingsList",
  component: VoiceFindingsList,
  tags: ["autodocs"],
  args: { scanned: true },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoiceFindingsList>;

/** Multiple findings with mixed severities. */
export const MixedSeverities: Story = {
  args: { findings: sampleFindings },
};

/** A scan that found nothing: the content is fully compliant. */
export const NoFindings: Story = {
  args: { findings: [] },
};

/** Nothing was scanned, so the empty list says so rather than claiming compliance. */
export const NothingScanned: Story = {
  args: { findings: [], scanned: false },
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
