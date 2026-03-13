import type { Meta, StoryObj } from "@storybook/react-vite";
import { HighlightedSource } from "../../components/editor/HighlightedSource";
import { sampleTermMatches, deprecatedTermMatch } from "../fixtures";

const sampleText = "localization is key in translation memory work and each term matters";

const meta: Meta<typeof HighlightedSource> = {
  title: "Editor/HighlightedSource",
  component: HighlightedSource,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 600, padding: 16, fontSize: 14, lineHeight: 1.6 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof HighlightedSource>;

/** Multiple term matches highlighted in source text */
export const MultipleMatches: Story = {
  args: {
    text: sampleText,
    termMatches: sampleTermMatches,
  },
};

/** Single term match */
export const SingleMatch: Story = {
  args: {
    text: "The localization process requires careful attention.",
    termMatches: [
      {
        source_term: "localization",
        target_terms: ["localisation"],
        domain: "i18n",
        status: "preferred",
        start: 4,
        end: 16,
      },
    ],
  },
};

/** No matches — renders plain text */
export const NoMatches: Story = {
  args: {
    text: "This sentence has no terminology matches.",
    termMatches: [],
  },
};

/** Deprecated term highlighted */
export const DeprecatedTerm: Story = {
  args: {
    text: "internationalization is a complex process.",
    termMatches: [deprecatedTermMatch],
  },
};

/** Adjacent matches close together */
export const AdjacentMatches: Story = {
  args: {
    text: "Use localization and translation tools.",
    termMatches: [
      {
        source_term: "localization",
        target_terms: ["localisation"],
        domain: "i18n",
        status: "preferred",
        start: 4,
        end: 16,
      },
      {
        source_term: "translation",
        target_terms: ["traduction"],
        domain: "i18n",
        status: "approved",
        start: 21,
        end: 32,
      },
    ],
  },
};
