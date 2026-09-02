import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { ALL_LANGUAGES, LanguageScopeSelect } from "../../components/review/LanguageScopeSelect";
import type { LanguageScopeOption } from "../../components/review/LanguageScopeSelect";

/**
 * Every language the queue can be read in, the project's source among them and
 * marked as the source, each carrying what it is waiting on.
 */
const options: LanguageScopeOption[] = [
  { locale: "en-US", source: true, pending: 3 },
  { locale: "fr-FR", pending: 24 },
  { locale: "de-DE", pending: 11 },
  { locale: "ja-JP", pending: 0 },
  { locale: "pt-BR", pending: 6 },
];

/** Keeps the trigger showing what was chosen, the way a review toolbar does. */
function Controlled({ initial, allowAll }: { initial: string; allowAll?: boolean }) {
  const [value, setValue] = useState(initial);
  return (
    <LanguageScopeSelect
      value={value}
      options={options}
      onChange={setValue}
      allowAll={allowAll}
      allPending={44}
      label="Language to review"
    />
  );
}

const meta: Meta<typeof LanguageScopeSelect> = {
  title: "Review/LanguageScopeSelect",
  component: LanguageScopeSelect,
  parameters: { layout: "centered" },
};
export default meta;
type Story = StoryObj<typeof LanguageScopeSelect>;

/**
 * The workspace-wide reading: every language at once, with the total the queue
 * is waiting on. A surface that mixes languages in one list starts here.
 */
export const AllLanguages: Story = {
  render: () => <Controlled initial={ALL_LANGUAGES} allowAll />,
};

/** One target language chosen, named rather than shown as a tag in capitals. */
export const ATargetLanguage: Story = {
  render: () => <Controlled initial="fr-FR" allowAll />,
};

/**
 * The source language chosen from the same list. Picking it opens the source
 * review, so a reviewer moves between judging a translation and judging the
 * text it was made from without a second control.
 */
export const TheSourceLanguage: Story = {
  render: () => <Controlled initial="en-US" allowAll />,
};

/**
 * The list open: language names in the reader's own UI language, the tag beside
 * each, the source marked, and the pending count per language so the choice is
 * made on the counts.
 */
export const OpenList: Story = {
  render: () => <Controlled initial="fr-FR" allowAll />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByTestId("language-scope"));
    // Radix renders the list in a portal, so it is found on the document body.
    const list = within(document.body);
    await expect(await list.findByTestId("language-scope-en-US")).toBeInTheDocument();
  },
};
