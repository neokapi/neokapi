import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { ALL_LANGUAGES, ReviewLanguageSelect } from "../../components/review";
import type {
  ReviewLanguageLane,
  ReviewLanguageSelectProps,
} from "../../components/review/ReviewLanguageSelect";

/**
 * Every language the queue can be read in, the project's source among them and
 * marked as the source, each carrying what it is waiting on.
 */
const lanes: ReviewLanguageLane[] = [
  { language: "en-US", source: true, pending: 3 },
  { language: "fr-FR", pending: 24 },
  { language: "de-DE", pending: 11 },
  { language: "ja-JP", pending: 0 },
  { language: "pt-BR", pending: 6 },
];

/** A workspace with a language for every market it sells into. */
const manyLanes: ReviewLanguageLane[] = [
  { language: "en-US", source: true, pending: 12 },
  ...[
    "fr-FR",
    "fr-CA",
    "de-DE",
    "es-ES",
    "es-MX",
    "it-IT",
    "ja-JP",
    "ko-KR",
    "nb-NO",
    "nl-NL",
    "pl-PL",
    "pt-BR",
    "sv-SE",
    "zh-Hans",
    "zh-Hant",
  ].map((language, i) => ({ language, pending: (i * 7) % 19 })),
];

/** Keeps the trigger showing what was chosen, the way a review toolbar does. */
function Controlled(props: Partial<ReviewLanguageSelectProps>) {
  const [value, setValue] = useState(props.value ?? ALL_LANGUAGES);
  return (
    <ReviewLanguageSelect
      lanes={lanes}
      allPending={44}
      {...props}
      value={value}
      onChange={setValue}
    />
  );
}

const meta: Meta<typeof ReviewLanguageSelect> = {
  title: "Review/ReviewLanguageSelect",
  component: ReviewLanguageSelect,
  parameters: {
    layout: "centered",
    docs: {
      description: {
        component:
          "The one control a review surface offers for choosing what it reads. One list holds every language the project has review work in, the source among them, so picking the source opens the author's own wording the same way picking French opens the French review. Both kapi desktop and the platform draw this one control.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ReviewLanguageSelect>;

/**
 * Every language at once, with the total the queue is waiting on. A surface
 * that mixes languages in one list starts here.
 */
export const AllLanguages: Story = {
  render: () => <Controlled value={ALL_LANGUAGES} allowAll />,
};

/** One target language chosen, named rather than shown as a tag in capitals. */
export const ATargetLanguage: Story = {
  render: () => <Controlled value="fr-FR" allowAll />,
};

/**
 * The source language chosen from the same list. Picking it puts the reviewer
 * in front of the author's own wording, so they move between judging a
 * translation and judging the text it was made from without a second control.
 */
export const TheSourceLanguage: Story = {
  render: () => <Controlled value="en-US" allowAll />,
};

/**
 * A surface that offers the languages of one file rather than a queue counts
 * nothing, so no entry carries a number and none reads as zero.
 */
export const WithoutCounts: Story = {
  render: () => (
    <Controlled
      value="fr-FR"
      lanes={[{ language: "en-US", source: true }, { language: "fr-FR" }, { language: "de-DE" }]}
    />
  ),
};

/**
 * A queue with nothing waiting anywhere. The source lane stays selectable, the
 * one lane a reviewer can always open.
 */
export const NothingPending: Story = {
  render: () => (
    <Controlled
      value={ALL_LANGUAGES}
      allowAll
      allPending={0}
      lanes={[
        { language: "en-US", source: true, pending: 0 },
        { language: "fr-FR", pending: 0 },
        { language: "de-DE", pending: 0 },
      ]}
    />
  ),
};

/**
 * The list open: language names in the reader's own UI language, the tag beside
 * each, the source marked, and the pending count per language so the choice is
 * made on the counts.
 */
export const OpenList: Story = {
  render: () => <Controlled value="fr-FR" allowAll />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("combobox"));
    // The popover renders in a portal, so the list is found on the document body.
    const list = within(document.body);
    await expect(await list.findByText(/American English/)).toBeInTheDocument();
  },
};

/**
 * A workspace with sixteen languages. Typing narrows the list by name or by
 * tag, so a long list stays one control rather than a scroll.
 */
export const ManyLanguages: Story = {
  render: () => <Controlled value={ALL_LANGUAGES} allowAll allPending={137} lanes={manyLanes} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("combobox"));
    const list = within(document.body);
    await userEvent.type(await list.findByPlaceholderText("Search languages"), "Chinese");
    await expect(
      document.querySelectorAll("[data-slot='review-language-option']").length,
    ).toBeLessThan(manyLanes.length);
  },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  render: () => <Controlled value="en-US" allowAll />,
};

/** The long list in the dark theme, open on its counts. */
export const DarkOpenList: Story = {
  globals: { theme: "dark" },
  render: () => <Controlled value={ALL_LANGUAGES} allowAll allPending={137} lanes={manyLanes} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("combobox"));
    const list = within(document.body);
    await expect(await list.findByText(/American English/)).toBeInTheDocument();
  },
};
