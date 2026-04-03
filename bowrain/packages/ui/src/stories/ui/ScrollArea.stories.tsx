import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScrollArea, ScrollBar } from "@neokapi/ui-primitives/components/ui/scroll-area";
import { Separator } from "@neokapi/ui-primitives/components/ui/separator";

const meta: Meta<typeof ScrollArea> = {
  title: "Foundations/ScrollArea",
  component: ScrollArea,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 350, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ScrollArea>;

const locales = [
  "English (en-US)",
  "French (fr-FR)",
  "German (de-DE)",
  "Spanish (es-ES)",
  "Italian (it-IT)",
  "Portuguese (pt-BR)",
  "Japanese (ja-JP)",
  "Korean (ko-KR)",
  "Chinese Simplified (zh-CN)",
  "Chinese Traditional (zh-TW)",
  "Arabic (ar-SA)",
  "Russian (ru-RU)",
  "Hindi (hi-IN)",
  "Turkish (tr-TR)",
  "Dutch (nl-NL)",
];

export const Default: Story = {
  render: () => (
    <ScrollArea className="h-48 w-full rounded-md border">
      <div className="p-4">
        <h4 className="mb-4 text-sm font-medium leading-none">Target Locales</h4>
        {locales.map((locale) => (
          <div key={locale}>
            <div className="text-sm">{locale}</div>
            <Separator className="my-2" />
          </div>
        ))}
      </div>
    </ScrollArea>
  ),
};

export const Horizontal: Story = {
  render: () => (
    <ScrollArea className="w-full whitespace-nowrap rounded-md border">
      <div className="flex w-max space-x-4 p-4">
        {locales.map((locale) => (
          <div key={locale} className="shrink-0 rounded-md border px-3 py-1.5 text-sm">
            {locale}
          </div>
        ))}
      </div>
      <ScrollBar orientation="horizontal" />
    </ScrollArea>
  ),
};
