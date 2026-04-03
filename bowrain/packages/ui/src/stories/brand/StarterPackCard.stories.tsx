import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StarterPackCard } from "../../brand/StarterPackCard";
import { starterPacks } from "../../brand/data/starter-packs";

const meta: Meta<typeof StarterPackCard> = {
  title: "Brand/StarterPackCard",
  component: StarterPackCard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 280, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StarterPackCard>;

/** Formal, authoritative B2B voice. */
export const ProfessionalB2B: Story = {
  args: {
    pack: starterPacks[0],
    onClick: fn(),
  },
};

/** Casual, warm direct-to-consumer voice. */
export const FriendlyDTC: Story = {
  args: {
    pack: starterPacks[1],
    onClick: fn(),
  },
};

/** Conversational, storytelling blog voice. */
export const MarketingBlog: Story = {
  args: {
    pack: starterPacks[2],
    onClick: fn(),
  },
};

/** Empathetic, solution-focused support voice. */
export const CustomerSupport: Story = {
  args: {
    pack: starterPacks[3],
    onClick: fn(),
  },
};

/** Precise, clear technical documentation voice. */
export const TechnicalDocs: Story = {
  args: {
    pack: starterPacks[4],
    onClick: fn(),
  },
};
