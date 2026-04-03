import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TermSidebar } from "../../components/editor/TermSidebar";
import { sampleTermMatches, deprecatedTermMatch } from "../fixtures";

const meta: Meta<typeof TermSidebar> = {
  title: "Editor/Terminology/TermSidebar",
  component: TermSidebar,
  tags: ["autodocs"],
  args: {
    onInsertTerm: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ height: 500, display: "flex", justifyContent: "flex-end" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TermSidebar>;

/** Sidebar with multiple term matches */
export const WithMatches: Story = {
  args: {
    termMatches: sampleTermMatches,
  },
};

/** Empty state — no terminology matches */
export const Empty: Story = {
  args: {
    termMatches: [],
  },
};

/** Loading state */
export const Loading: Story = {
  args: {
    termMatches: [],
    loading: true,
  },
};

/** Enrich mode — shows add term button */
export const EnrichMode: Story = {
  args: {
    termMatches: sampleTermMatches,
    editorMode: "enrich",
    onAddTerm: fn(),
  },
};

/** Term with no target translations defined */
export const NoTargetTerms: Story = {
  args: {
    termMatches: [deprecatedTermMatch],
  },
};

/** Single match with domain badge */
export const SingleMatch: Story = {
  args: {
    termMatches: [sampleTermMatches[0]],
  },
};

/** All term statuses displayed */
export const AllStatuses: Story = {
  args: {
    termMatches: [...sampleTermMatches, deprecatedTermMatch],
  },
};
