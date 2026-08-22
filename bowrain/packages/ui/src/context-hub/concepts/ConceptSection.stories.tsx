import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptsSection } from "./ConceptsSection";
import { ConceptStorySection } from "./ConceptStorySection";
import { withContextHub } from "../../stories/voiceHubFixtures";

// The R4 Concepts section, now built on the framework concept UI
// (@neokapi/concept-ui) driven by a RestConceptDataSource over the mock
// ApiAdapter — the same components kapi-desktop renders locally.
const meta: Meta = {
  title: "Context/Concepts/Concept UI",
  parameters: { layout: "fullscreen" },
  decorators: [
    withContextHub,
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;

export const List: StoryObj<typeof ConceptsSection> = {
  render: (args) => <ConceptsSection {...args} />,
  args: { onOpenConcept: fn() },
};

export const Dashboard: StoryObj<typeof ConceptStorySection> = {
  render: (args) => <ConceptStorySection {...args} />,
  args: {
    conceptId: "c-checkout",
    onBack: fn(),
    onOpenConcept: fn(),
    onOpenExperiments: fn(),
  },
};
