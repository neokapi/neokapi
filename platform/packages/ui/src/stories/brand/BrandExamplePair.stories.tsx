import type { Meta, StoryObj } from "@storybook/react-vite";
import { BrandExamplePair } from "../../brand/BrandExamplePair";
import { sampleExamples } from "./fixtures";

const meta: Meta<typeof BrandExamplePair> = {
  title: "Brand/BrandExamplePair",
  component: BrandExamplePair,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 600, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BrandExamplePair>;

/** Typical before/after with explanation. */
export const WithExplanation: Story = {
  args: { example: sampleExamples[0] },
};

/** Before/after without explanation. */
export const WithoutExplanation: Story = {
  args: {
    example: { before: "Click here to learn more.", after: "Select Learn more." },
  },
};

/** Long text to test wrapping. */
export const LongText: Story = {
  args: {
    example: {
      before:
        "If you need any help setting up the integration, please don't hesitate to reach out to our friendly support team who will be happy to assist you with any questions.",
      after:
        "For integration setup assistance, contact support. The team responds within one business day.",
      explanation: "Shortened, removed hedging language, added concrete SLA.",
    },
  },
};
