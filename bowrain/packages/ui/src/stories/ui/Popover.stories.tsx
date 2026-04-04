import {
  Button,
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta: Meta<typeof Popover> = {
  title: "Foundations/Popover",
  component: Popover,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Popover>;

export const Default: Story = {
  render: () => (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline">Open Popover</Button>
      </PopoverTrigger>
      <PopoverContent>
        <PopoverHeader>
          <PopoverTitle>Translation Stats</PopoverTitle>
          <PopoverDescription>Current project translation progress.</PopoverDescription>
        </PopoverHeader>
        <p className="text-sm text-muted-foreground">142 of 200 strings translated (71%)</p>
      </PopoverContent>
    </Popover>
  ),
};

export const Simple: Story = {
  render: () => (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline">Info</Button>
      </PopoverTrigger>
      <PopoverContent>
        <p className="text-sm">Locale codes follow BCP-47 format (e.g. en-US, fr-FR).</p>
      </PopoverContent>
    </Popover>
  ),
};
