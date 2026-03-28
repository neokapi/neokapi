import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { PersonalityTagPicker } from "../../brand/PersonalityTagPicker";

const meta: Meta<typeof PersonalityTagPicker> = {
  title: "Brand/PersonalityTagPicker",
  component: PersonalityTagPicker,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof PersonalityTagPicker>;

/** No tags selected — shows all suggested categories. */
export const Empty: Story = {
  args: {
    tags: [],
    onChange: fn(),
  },
};

/** Three tags already selected — some suggestions dimmed. */
export const WithTags: Story = {
  args: {
    tags: ["friendly", "professional", "warm"],
    onChange: fn(),
  },
};
