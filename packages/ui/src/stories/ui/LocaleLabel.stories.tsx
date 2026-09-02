import { Fragment } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleLabel } from "../../components/ui/locale-label";

/**
 * One rendering for a language: the name in the reader's own language, the tag
 * beside it in muted monospace. Where there is no room, the tag alone with the
 * name in the tooltip.
 */
const meta: Meta<typeof LocaleLabel> = {
  title: "Foundations/LocaleLabel",
  component: LocaleLabel,
  parameters: { layout: "padded" },
  args: { locale: "fr-FR" },
};
export default meta;

type Story = StoryObj<typeof LocaleLabel>;

const TAGS = ["fr-FR", "pt-BR", "zh-Hant", "sr-Latn-RS", "nb-NO", "ar-EG", "qps"];

function Panel({ dark, children }: { dark?: boolean; children: React.ReactNode }) {
  return (
    <div className={dark ? "dark" : undefined}>
      <div className="rounded-lg border bg-background p-4 text-foreground">
        <p className="mb-3 text-xs font-medium text-muted-foreground">{dark ? "Dark" : "Light"}</p>
        {children}
      </div>
    </div>
  );
}

/** Full and compact, in both themes. */
export const NameAndCode: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      {[false, true].map((dark) => (
        <Panel key={String(dark)} dark={dark}>
          <div className="flex flex-col gap-2 text-sm">
            {TAGS.map((tag) => (
              <div key={tag} className="flex items-center gap-6">
                <LocaleLabel locale={tag} className="w-64" />
                <LocaleLabel locale={tag} compact />
              </div>
            ))}
          </div>
        </Panel>
      ))}
    </div>
  ),
};

/** The source language carries a marker, so a list of targets reads as targets. */
export const SourceMarker: Story = {
  render: () => (
    <ul className="flex flex-col gap-2 text-sm">
      <li>
        <LocaleLabel locale="en-US" source />
      </li>
      <li>
        <LocaleLabel locale="fr-FR" />
      </li>
      <li>
        <LocaleLabel locale="ja-JP" />
      </li>
    </ul>
  ),
};

/** `short` drops the region where a column has no room for it. */
export const ShortAndHiddenCode: Story = {
  render: () => (
    <div className="flex flex-col gap-2 text-sm">
      <LocaleLabel locale="fr-FR" variant="short" />
      <LocaleLabel locale="pt-BR" variant="short" />
      <LocaleLabel locale="fr-FR" hideCode />
      <LocaleLabel locale="fr-FR" displayName="French (Canada office)" />
    </div>
  ),
};

/** The name follows the reader. Same tags, named in French and in Norwegian. */
export const InAnotherUILanguage: Story = {
  render: () => (
    <div className="grid grid-cols-3 gap-x-8 gap-y-2 text-sm">
      {TAGS.map((tag) => (
        <Fragment key={tag}>
          <LocaleLabel locale={tag} uiLocale="en" />
          <LocaleLabel locale={tag} uiLocale="fr" />
          <LocaleLabel locale={tag} uiLocale="nb" />
        </Fragment>
      ))}
    </div>
  ),
};
