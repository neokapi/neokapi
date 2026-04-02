import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Sheet,
  SheetTrigger,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
} from "@neokapi/ui-primitives/components/ui/sheet";
import { Button } from "@neokapi/ui-primitives/components/ui/button";

const meta: Meta<typeof Sheet> = {
  title: "Foundations/Sheet",
  component: Sheet,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Sheet>;

export const Default: Story = {
  render: () => (
    <Sheet>
      <SheetTrigger asChild>
        <Button variant="outline">Open Sheet</Button>
      </SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Project Settings</SheetTitle>
          <SheetDescription>
            Configure source and target languages for this project.
          </SheetDescription>
        </SheetHeader>
        <div className="px-4">
          <p className="text-sm text-muted-foreground">Settings form content goes here.</p>
        </div>
        <SheetFooter>
          <Button>Save Changes</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  ),
};

export const LeftSide: Story = {
  render: () => (
    <Sheet>
      <SheetTrigger asChild>
        <Button variant="outline">Open Left Sheet</Button>
      </SheetTrigger>
      <SheetContent side="left">
        <SheetHeader>
          <SheetTitle>Navigation</SheetTitle>
          <SheetDescription>Browse your workspace.</SheetDescription>
        </SheetHeader>
        <div className="px-4">
          <ul className="space-y-2 text-sm">
            <li>Dashboard</li>
            <li>Projects</li>
            <li>Translation Memory</li>
            <li>Terminology</li>
          </ul>
        </div>
      </SheetContent>
    </Sheet>
  ),
};
