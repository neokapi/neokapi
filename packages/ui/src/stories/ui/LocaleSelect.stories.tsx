import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  LocaleSelect,
  MultiLocaleSelect,
  type LocaleInfo,
} from "../../components/ui/locale-select";

// Sample locale data matching core/locale.WellKnownLocales() output.
const sampleLocales: LocaleInfo[] = [
  { code: "ar", displayName: "Arabic" },
  { code: "zh", displayName: "Chinese" },
  { code: "cs", displayName: "Czech" },
  { code: "da", displayName: "Danish" },
  { code: "nl", displayName: "Dutch" },
  { code: "en", displayName: "English" },
  { code: "fi", displayName: "Finnish" },
  { code: "fr", displayName: "French" },
  { code: "de", displayName: "German" },
  { code: "el", displayName: "Greek" },
  { code: "it", displayName: "Italian" },
  { code: "ja", displayName: "Japanese" },
  { code: "ko", displayName: "Korean" },
  { code: "nb", displayName: "Norwegian Bokmål" },
  { code: "pl", displayName: "Polish" },
  { code: "pt", displayName: "Portuguese" },
  { code: "pt-BR", displayName: "Brazilian Portuguese" },
  { code: "ru", displayName: "Russian" },
  { code: "zh-Hans", displayName: "Simplified Chinese" },
  { code: "es", displayName: "Spanish" },
  { code: "sv", displayName: "Swedish" },
  { code: "th", displayName: "Thai" },
  { code: "zh-Hant", displayName: "Traditional Chinese" },
  { code: "tr", displayName: "Turkish" },
];

function SingleWrapper({
  initial = "",
  locales = sampleLocales,
}: {
  initial?: string;
  locales?: LocaleInfo[];
}) {
  const [value, setValue] = useState(initial);
  return (
    <div className="max-w-sm space-y-2">
      <LocaleSelect value={value} onChange={setValue} locales={locales} />
      <pre className="rounded bg-muted p-2 font-mono text-xs">value: {JSON.stringify(value)}</pre>
    </div>
  );
}

function MultiWrapper({
  initial = [] as string[],
  locales = sampleLocales,
}: {
  initial?: string[];
  locales?: LocaleInfo[];
}) {
  const [value, setValue] = useState(initial);
  return (
    <div className="max-w-lg space-y-2">
      <MultiLocaleSelect value={value} onChange={setValue} locales={locales} />
      <pre className="rounded bg-muted p-2 font-mono text-xs">value: {JSON.stringify(value)}</pre>
    </div>
  );
}

const meta: Meta<typeof LocaleSelect> = {
  title: "Foundations/LocaleSelect",
  component: LocaleSelect,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Locale selector with autocomplete. Single-select for source language, multi-select with chips for target languages. Pure component — locales passed as props.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof LocaleSelect>;

export const SingleSelect: Story = {
  name: "Single Select",
  render: () => <SingleWrapper initial="en" />,
};

export const SingleSelectEmpty: Story = {
  name: "Single Select — Empty",
  render: () => <SingleWrapper />,
};

export const MultiSelect: Story = {
  name: "Multi Select",
  render: () => <MultiWrapper initial={["fr", "de", "ja"]} />,
};

export const MultiSelectEmpty: Story = {
  name: "Multi Select — Empty",
  render: () => <MultiWrapper />,
};

export const SideBySide: Story = {
  name: "Source + Target (Side by Side)",
  render: () => (
    <div className="max-w-2xl space-y-4">
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Source Language</label>
        <SingleWrapper initial="en" />
      </div>
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Target Languages</label>
        <MultiWrapper initial={["fr", "de", "pt-BR"]} />
      </div>
    </div>
  ),
};

export const POSIXCodes: Story = {
  name: "POSIX-style Codes",
  render: () => {
    const posixLocales: LocaleInfo[] = sampleLocales.map((l) => ({
      ...l,
      code: l.code.replace(/-/g, "_"),
    }));
    return <MultiWrapper initial={["pt_BR", "zh_Hans"]} locales={posixLocales} />;
  },
};
