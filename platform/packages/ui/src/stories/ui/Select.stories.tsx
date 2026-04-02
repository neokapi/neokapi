import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives/components/ui/select";

const meta: Meta<typeof Select> = {
  title: "Foundations/Select",
  component: Select,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 280, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Select>;

export const Default: Story = {
  render: () => (
    <Select>
      <SelectTrigger>
        <SelectValue placeholder="Select a locale" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>Locales</SelectLabel>
          <SelectItem value="en-US">English (US)</SelectItem>
          <SelectItem value="fr-FR">French (France)</SelectItem>
          <SelectItem value="de-DE">German (Germany)</SelectItem>
          <SelectItem value="ja-JP">Japanese (Japan)</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  ),
};

export const Small: Story = {
  render: () => (
    <Select>
      <SelectTrigger size="sm">
        <SelectValue placeholder="Format" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="json">JSON</SelectItem>
        <SelectItem value="xliff">XLIFF</SelectItem>
        <SelectItem value="po">PO</SelectItem>
        <SelectItem value="properties">Properties</SelectItem>
      </SelectContent>
    </Select>
  ),
};

export const Disabled: Story = {
  render: () => (
    <Select disabled>
      <SelectTrigger>
        <SelectValue placeholder="Disabled" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="a">Option A</SelectItem>
      </SelectContent>
    </Select>
  ),
};
