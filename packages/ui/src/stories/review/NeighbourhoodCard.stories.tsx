import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReviewNeighbourhood } from "@neokapi/contract-types";
import { NeighbourhoodCard } from "../../components/review";

const around: ReviewNeighbourhood = {
  key: "greeting",
  before: [
    {
      key: "welcome",
      source: [{ text: "Welcome back." }],
      target: [{ text: "Bon retour." }],
      status: "reviewed",
    },
  ],
  after: [
    {
      key: "credits",
      source: [
        { text: "Your credits reset on " },
        { ph: { id: "1", type: "code:variable", data: "{date}", equiv: "{date}" } },
        { text: "." },
      ],
    },
    {
      key: "farewell",
      source: [{ text: "See you soon." }],
      target: [{ text: "À bientôt." }],
      status: "translated",
    },
  ],
  window: 2,
};

const meta: Meta<typeof NeighbourhoodCard> = {
  title: "Review/NeighbourhoodCard",
  component: NeighbourhoodCard,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "The unit in its document: the blocks before it, the unit, and the blocks after it, in document order. The neighbours travel as run sequences through the declared run projection, so a placeholder reads as a chip rather than disappearing. A neighbour nothing has translated yet draws its source alone.",
      },
    },
  },
  args: {
    unitKey: "greeting",
    unitSource: "Hello {name}, ready to continue?",
    unitTarget: "Bonjour {name}, on continue ?",
    sourceLocale: "en-US",
    locale: "fr-FR",
  },
  render: (args) => (
    <div className="w-[28rem]">
      <NeighbourhoodCard {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof NeighbourhoodCard>;

export const InSequence: Story = {
  name: "Between its neighbours, one untranslated",
  args: { neighbourhood: around },
};

export const Alone: Story = {
  name: "The only unit in its document",
  args: { neighbourhood: { key: "greeting", window: 2 } },
};

export const Loading: Story = {
  args: { loading: true },
};

export const Unreadable: Story = {
  name: "The document could not be read",
  args: {},
};

export const Dark: Story = {
  globals: { theme: "dark" },
  args: { neighbourhood: around },
};
