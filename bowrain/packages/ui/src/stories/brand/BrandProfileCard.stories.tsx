import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BrandProfileCard } from "../../brand/BrandProfileCard";
import { sampleProfile, casualProfile, technicalProfile } from "./fixtures";

const meta: Meta<typeof BrandProfileCard> = {
  title: "Brand/BrandProfileCard",
  component: BrandProfileCard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 360, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BrandProfileCard>;

/** Formal enterprise profile with delete action. */
export const Default: Story = {
  args: {
    profile: sampleProfile,
    onClick: fn(),
    onDelete: fn(),
  },
};

/** Card without delete button (read-only). */
export const WithoutDelete: Story = {
  args: {
    profile: casualProfile,
    onClick: fn(),
  },
};

/** Technical profile with fewer personality tags. */
export const Technical: Story = {
  args: {
    profile: technicalProfile,
    onClick: fn(),
    onDelete: fn(),
  },
};

/** Profile with a long name and description to test truncation. */
export const LongContent: Story = {
  args: {
    profile: {
      ...sampleProfile,
      name: "Enterprise Documentation Style Guide for All Customer-Facing Content",
      description:
        "This profile governs all customer-facing documentation including API references, getting started guides, tutorials, and troubleshooting articles. It enforces formal tone, active voice, and approved terminology across all locales.",
    },
    onClick: fn(),
    onDelete: fn(),
  },
};
