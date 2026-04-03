import type { Meta, StoryObj } from "@storybook/react-vite";
import { BrandMCPGuide } from "../../brand/BrandMCPGuide";

const meta: Meta<typeof BrandMCPGuide> = {
  title: "Brand/BrandMCPGuide",
  component: BrandMCPGuide,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BrandMCPGuide>;

/** Default local development setup. */
export const Default: Story = {
  args: {},
};

/** Custom server URL and token. */
export const CustomServer: Story = {
  args: {
    serverUrl: "https://bowrain.example.com",
    apiToken: "br_tok_abc123def456",
  },
};
