import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BrandProfileList } from "../../brand/BrandProfileList";
import { withProviders } from "../decorators";
import { sampleProfile, casualProfile, technicalProfile } from "./fixtures";

const meta: Meta<typeof BrandProfileList> = {
  title: "Brand/BrandProfileList",
  component: BrandProfileList,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BrandProfileList>;

/** Empty state — no profiles yet. */
export const Empty: Story = {
  args: {
    profiles: [],
    onSelect: fn(),
    onCreate: fn(),
    onCreateFromStarter: fn(),
    onDelete: fn(),
  },
};

/** Single profile. */
export const SingleProfile: Story = {
  args: {
    profiles: [sampleProfile],
    onSelect: fn(),
    onCreate: fn(),
    onCreateFromStarter: fn(),
    onDelete: fn(),
  },
};

/** Multiple profiles in a grid. */
export const MultipleProfiles: Story = {
  args: {
    profiles: [sampleProfile, casualProfile, technicalProfile],
    onSelect: fn(),
    onCreate: fn(),
    onCreateFromStarter: fn(),
    onDelete: fn(),
  },
};

/** Many profiles to test grid scaling and search. */
export const ManyProfiles: Story = {
  args: {
    profiles: [
      sampleProfile,
      casualProfile,
      technicalProfile,
      { ...sampleProfile, id: "vp-4", name: "Support Articles" },
      { ...casualProfile, id: "vp-5", name: "Social Media Posts" },
      { ...technicalProfile, id: "vp-6", name: "Internal Wiki" },
    ],
    onSelect: fn(),
    onCreate: fn(),
    onCreateFromStarter: fn(),
    onDelete: fn(),
  },
};
