import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "../../components/ui/combobox";

/**
 * The shared searchable-select primitive (base-ui). Type to search, keyboard-navigate,
 * pick one. Used as the base for the locale add-picker and other searchable selectors.
 */
const meta: Meta = {
  title: "UI/Combobox",
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 320, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj;

const locales = [
  { value: "en-US", label: "English (US)" },
  { value: "fr-FR", label: "French (France)" },
  { value: "de-DE", label: "German (Germany)" },
  { value: "ja-JP", label: "Japanese (Japan)" },
  { value: "zh-CN", label: "Chinese (Simplified)" },
];

export const Default: Story = {
  render: () => (
    <Combobox>
      <ComboboxInput placeholder="Search locales..." />
      <ComboboxContent>
        <ComboboxList>
          <ComboboxEmpty>No locales found.</ComboboxEmpty>
          {locales.map((locale) => (
            <ComboboxItem key={locale.value} value={locale.value}>
              {locale.label}
            </ComboboxItem>
          ))}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  ),
};

export const WithClear: Story = {
  render: () => (
    <Combobox>
      <ComboboxInput placeholder="Search locales..." showClear />
      <ComboboxContent>
        <ComboboxList>
          <ComboboxEmpty>No locales found.</ComboboxEmpty>
          {locales.map((locale) => (
            <ComboboxItem key={locale.value} value={locale.value}>
              {locale.label}
            </ComboboxItem>
          ))}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  ),
};

/** Controlled single-select with client-side filtering (the add-picker pattern). */
export const Controlled: Story = {
  render: () => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [value, setValue] = useState<string | null>("fr-FR");
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [search, setSearch] = useState("");
    const shown = locales.filter((l) => l.label.toLowerCase().includes(search.toLowerCase()));
    return (
      <div className="flex flex-col gap-2">
        <Combobox
          value={value}
          onValueChange={setValue}
          onInputValueChange={(text: string) => setSearch(text)}
        >
          <ComboboxInput placeholder="Pick a locale..." showClear />
          <ComboboxContent>
            <ComboboxList>
              <ComboboxEmpty>No matching locales.</ComboboxEmpty>
              {shown.map((locale) => (
                <ComboboxItem key={locale.value} value={locale.value}>
                  {locale.label}
                </ComboboxItem>
              ))}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
        <p className="text-xs text-muted-foreground">Selected: {value ?? "(none)"}</p>
      </div>
    );
  },
};
