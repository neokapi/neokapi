import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocalePill } from "../../components/resource-browser/LocalePill";

const meta: Meta<typeof LocalePill> = {
  title: "Resource Browser/LocalePill",
  component: LocalePill,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Compact locale badge with a deterministic color derived from the locale code. Uses OKLCH color space with CSS custom properties for dark mode support.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof LocalePill>;

export const EnglishUS: Story = {
  args: { locale: "en-US" },
};

export const FrenchFrance: Story = {
  args: { locale: "fr-FR" },
};

export const ChineseSimplified: Story = {
  args: { locale: "zh-CN" },
};

export const ArabicSaudiArabia: Story = {
  args: { locale: "ar-SA" },
};

export const GermanGermany: Story = {
  args: { locale: "de-DE" },
};

/** Multiple locale pills displayed together as they would appear in a TM entry row. */
export const AllLocales: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <LocalePill locale="en-US" />
      <LocalePill locale="fr-FR" />
      <LocalePill locale="zh-CN" />
      <LocalePill locale="ar-SA" />
      <LocalePill locale="de-DE" />
      <LocalePill locale="ja-JP" />
      <LocalePill locale="ko-KR" />
      <LocalePill locale="pt-BR" />
      <LocalePill locale="es-MX" />
    </div>
  ),
};

/**
 * When an active language filter is applied, filtered-in locales keep their
 * colour and the rest render grey (`muted`) — used on the project header and
 * collection cards so the language scope reads at a glance. Here fr-FR and
 * ja-JP are the active languages.
 */
export const MutedOutsideFilter: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <LocalePill locale="de-DE" muted />
      <LocalePill locale="fr-FR" />
      <LocalePill locale="ja-JP" />
      <LocalePill locale="nb-NO" muted />
      <LocalePill locale="ar-SA" muted />
    </div>
  ),
};
