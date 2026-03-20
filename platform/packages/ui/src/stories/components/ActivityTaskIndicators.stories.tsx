import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ActivityIndicator } from "../../components/ActivityTaskIndicators";
import { sampleActivities } from "./fixtures";

const activityMeta: Meta<typeof ActivityIndicator> = {
  title: "Components/ActivityIndicator",
  component: ActivityIndicator,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default activityMeta;
type ActivityStory = StoryObj<typeof ActivityIndicator>;

export const WithActivities: ActivityStory = {
  args: { activities: sampleActivities, onActivityClick: fn(), onViewAll: fn() },
};

export const Empty: ActivityStory = {
  args: { activities: [], onActivityClick: fn() },
};

// TaskIndicator stories are in a separate file due to default export constraint.
