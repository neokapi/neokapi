import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BrandProfileEditor } from "../../brand/BrandProfileEditor";
import { sampleProfile } from "./fixtures";

const meta: Meta<typeof BrandProfileEditor> = {
  title: "Brand/BrandProfileEditor",
  component: BrandProfileEditor,
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
type Story = StoryObj<typeof BrandProfileEditor>;

/** Create a new profile from scratch. */
export const CreateNew: Story = {
  args: {
    onSave: fn(),
    onCancel: fn(),
  },
};

/** Edit an existing profile with pre-filled data. */
export const EditExisting: Story = {
  args: {
    profile: sampleProfile,
    onSave: fn(),
    onCancel: fn(),
  },
};
