import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { HistoryCard } from "../../components/review";

const meta: Meta<typeof HistoryCard> = {
  title: "Review/HistoryCard",
  component: HistoryCard,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "What this unit said before, and the wording the content memory already holds for it. Both halves carry their text; a prior approval whose governing context has moved is marked in muted ink, because the mark describes the context and the severities belong to the Checks card. A surface with a write offers the match's wording.",
      },
    },
  },
  args: { sourceLocale: "en-US", locale: "fr-FR" },
  render: (args) => (
    <div className="w-[28rem]">
      <HistoryCard {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof HistoryCard>;

export const StillGoverned: Story = {
  name: "Approved before, still governed",
  args: {
    history: {
      prior: { source: "Hello {name}", target: "Bonjour {name}", governed: true },
      match: { score: 100, source: "Hello {name}", target: "Bonjour {name}", kind: "exact" },
    },
  },
};

export const ContextMoved: Story = {
  name: "Approved before, under a context that has moved",
  args: {
    history: {
      prior: { source: "Hi {name}", target: "Salut {name}", governed: false },
      match: { score: 88, source: "Hello {name}!", target: "Bonjour {name} !", kind: "fuzzy" },
    },
  },
};

export const MatchWithWrite: Story = {
  name: "Match only, on a surface that can take it (platform)",
  args: {
    history: {
      match: {
        score: 92,
        source: "Reset your password",
        target: "Réinitialisez votre mot de passe",
        kind: "fuzzy",
      },
    },
    onUseMatch: fn(),
  },
};

export const Unseeded: Story = {
  name: "The content memory has not been read into this copy",
  args: { history: { unseeded: true } },
};

export const Empty: Story = {
  name: "Nothing approved, no close match",
  args: {
    history: {},
    emptyText:
      "Nothing has been approved for this unit yet, and the content memory holds no close match.",
  },
};

export const Loading: Story = {
  args: { loading: true },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  args: {
    history: {
      prior: { source: "Hi {name}", target: "Salut {name}", governed: false },
      match: { score: 88, source: "Hello {name}!", target: "Bonjour {name} !", kind: "fuzzy" },
    },
    onUseMatch: fn(),
  },
};
