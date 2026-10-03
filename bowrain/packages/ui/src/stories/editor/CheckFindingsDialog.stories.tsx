import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { CheckFindingsDialog } from "../../components/editor/CheckFindingsDialog";
import { withProviders } from "../decorators";

// What a person sees when the project's checks refuse a translation they
// saved: nothing was written. The dialog lists what the checks found and the
// person's version, and asks whether to go back to the wording or save it
// anyway, which records the findings with the change.

const meta: Meta<typeof CheckFindingsDialog> = {
  title: "Editor/CheckFindingsDialog",
  component: CheckFindingsDialog,
  decorators: [withProviders],
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof CheckFindingsDialog>;

const handlers = () => ({ onOverride: fn(), onRevise: fn() });

/** A save that uses a word the terms rule out. */
export const SaveUsesARuledOutTerm: Story = {
  args: {
    state: {
      locale: "fr-FR",
      mine: "Ouvrez les paramètres pour changer de langue.",
      findings: [
        {
          rule: "terms.vocabulary",
          message: 'Use "réglages", not "paramètres"',
          fails: true,
        },
      ],
      ...handlers(),
    },
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByTestId("check-finding")).toHaveTextContent("réglages");
    await expect(body.getByTestId("check-findings-mine")).toHaveTextContent("paramètres");
    await expect(body.getByTestId("findings-override")).toHaveTextContent("Save anyway");
  },
};

/** A save that drops a variable and repeats a link: two findings, codes as chips. */
export const SaveBreaksInlineCodes: Story = {
  args: {
    state: {
      locale: "de-DE",
      mine: 'Lesen Sie den <x id="1"/>Leitfaden<x id="/1"/> und den <x id="1"/>Leitfaden<x id="/1"/>.',
      findings: [
        {
          rule: "placeholders.integrity",
          message:
            'Non-deletable Placeholder span "code:variable" is missing from target (1 missing)',
          fails: true,
        },
        {
          rule: "placeholders.integrity",
          message: 'Non-cloneable Opening span "link:hyperlink" was duplicated in target (1 extra)',
          fails: true,
        },
      ],
      ...handlers(),
    },
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findAllByTestId("check-finding")).toHaveLength(2);
  },
};
