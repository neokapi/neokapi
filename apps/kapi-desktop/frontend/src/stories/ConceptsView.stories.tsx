import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConceptsView } from "../components/ConceptsView";
import { manyConcepts, storyConceptSource } from "./fixtures/concepts";

/**
 * The concepts of a local terms store as a list beside a detail pane. The
 * story drives the same local concept source the desktop uses, over an
 * in-memory backend, so relating two concepts and removing a relation are live.
 */
const meta: Meta<typeof ConceptsView> = {
  title: "Pages/ConceptsView",
  component: ConceptsView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { handle: "story", sourceLocale: "en-US" },
  decorators: [
    (Story) => (
      <div className="h-[560px] border border-border bg-background">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConceptsView>;

/** A few concepts, nothing selected. */
export const Default: Story = {
  render: (args) => <ConceptsView {...args} source={storyConceptSource()} />,
};

/** A concept selected, with discouraged forms and a relation. */
export const Selected: Story = {
  render: (args) => <ConceptsView {...args} source={storyConceptSource()} defaultSelected="c1" />,
};

/** The same in the dark theme. */
export const SelectedDark: Story = { ...Selected, globals: { theme: "dark" } };

/** The Active Filter narrows a concept's languages to German. */
export const FilteredLanguages: Story = {
  render: (args) => (
    <ConceptsView
      {...args}
      source={storyConceptSource()}
      defaultSelected="c1"
      localeScope={["de-DE"]}
    />
  ),
};

/** Five hundred concepts. */
export const Many: Story = {
  render: (args) => <ConceptsView {...args} source={storyConceptSource(manyConcepts(500))} />,
};

/** A store with no concepts in it yet. */
export const Empty: Story = {
  render: (args) => <ConceptsView {...args} source={storyConceptSource([])} />,
};
