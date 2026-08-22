import type { Meta, StoryObj } from "@storybook/react-vite";
import { ContextScanAxes } from "../../context-scan/ContextScanAxes";
import { withProviders } from "../decorators";
import { sampleContextScanDraft } from "../mock-adapter";

const meta: Meta<typeof ContextScanAxes> = {
  title: "Context Scan/ContextScanAxes",
  component: ContextScanAxes,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextScanAxes>;

/**
 * The two shapes an axis approval takes, together.
 *
 * `product` is derived from a collection's `channel:`, so approving it is a
 * claim about one collection and the row asks which. `audience` is declared on
 * the project's default point and inherited by every collection, so naming one
 * would be a narrower claim than the axis makes — that row has no collection
 * control at all.
 */
export const BothShapes: Story = {
  args: { axes: sampleContextScanDraft.axes ?? [] },
};

/**
 * The axis names are the corpus's own. A workspace that governs by market or
 * tenant gets those, and neither is structural, so both are approved against
 * the project alone.
 */
export const WorkspaceNamedAxes: Story = {
  args: {
    axes: [
      {
        axis: "market",
        values: ["emea", "japan"],
        evidence: ["Prices shown in EUR for EMEA customers"],
        confidence: 0.61,
      },
      {
        axis: "tenant",
        values: ["acme", "globex"],
        evidence: ["Two distinct company names across the uploaded decks"],
        confidence: 0.44,
      },
    ],
  },
};

/**
 * A uniform corpus proposes no axes. The card renders nothing rather than an
 * empty state implying the scan failed at something.
 */
export const NoAxes: Story = {
  args: { axes: [] },
};
