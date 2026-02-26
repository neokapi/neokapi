import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { LocaleSelect, MultiLocaleSelect } from "../../components/LocaleSelect";
import { withProviders } from "../decorators";

const meta: Meta<typeof LocaleSelect> = {
  title: "Layout/LocaleSelect",
  component: LocaleSelect,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 320, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof LocaleSelect>;

export const Default: Story = {
  args: {
    value: "en-US",
    onChange: () => {},
  },
};

export const WithPlaceholder: Story = {
  args: {
    value: "",
    onChange: () => {},
    placeholder: "Choose a locale...",
  },
};

export const MultiSelect: StoryObj<typeof MultiLocaleSelect> = {
  render: () => {
    const [value, setValue] = useState(["fr-FR", "de-DE"]);
    return <MultiLocaleSelect value={value} onChange={setValue} />;
  },
};
