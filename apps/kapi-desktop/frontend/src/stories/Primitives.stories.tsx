import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Button,
  Badge,
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  Input,
  Label,
  Separator,
} from "@neokapi/ui-primitives";

const meta: Meta = {
  title: "Foundations/UI Primitives",
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj;

export const Buttons: Story = {
  render: () => (
    <div className="flex flex-wrap gap-3 p-4">
      <Button>Primary</Button>
      <Button variant="secondary">Secondary</Button>
      <Button variant="destructive">Destructive</Button>
      <Button variant="ghost">Ghost</Button>
      <Button disabled>Disabled</Button>
    </div>
  ),
};

export const Cards: Story = {
  render: () => (
    <div className="grid grid-cols-2 gap-4 p-4" style={{ maxWidth: 600 }}>
      <Card>
        <CardHeader>
          <CardTitle>Card Title</CardTitle>
          <CardDescription>Card description with some content.</CardDescription>
        </CardHeader>
      </Card>
      <Card className="bg-accent/30">
        <CardHeader>
          <CardTitle>Accent Card</CardTitle>
          <CardDescription>With background accent.</CardDescription>
        </CardHeader>
      </Card>
    </div>
  ),
};

export const Inputs: Story = {
  render: () => (
    <div className="max-w-sm space-y-3 p-4">
      <div>
        <Label>Text Input</Label>
        <Input type="text" placeholder="Enter text..." />
      </div>
      <div>
        <Label>Disabled</Label>
        <Input type="text" value="Read-only value" disabled />
      </div>
    </div>
  ),
};

export const Badges: Story = {
  render: () => (
    <div className="flex flex-wrap gap-2 p-4">
      <Badge>Primary</Badge>
      <Badge variant="secondary">Accent</Badge>
      <Badge variant="destructive">Destructive</Badge>
      <Badge variant="outline">Success</Badge>
    </div>
  ),
};

export const Typography: Story = {
  render: () => (
    <div className="space-y-3 p-4">
      <h1 className="text-3xl font-bold tracking-tight">Heading 1</h1>
      <h2 className="text-xl font-semibold">Heading 2</h2>
      <h3 className="text-sm font-medium">Heading 3</h3>
      <p className="text-sm text-foreground">Body text in foreground color.</p>
      <p className="text-sm text-muted-foreground">Muted text for secondary content.</p>
      <p className="text-xs text-muted-foreground">Small caption text.</p>
    </div>
  ),
};

export const Colors: Story = {
  render: () => (
    <div className="space-y-4 p-4">
      <h3 className="text-sm font-medium">Theme Colors</h3>
      <div className="grid grid-cols-4 gap-2">
        {[
          { name: "Background", cls: "bg-background border border-border" },
          { name: "Foreground", cls: "bg-foreground" },
          { name: "Primary", cls: "bg-primary" },
          { name: "Secondary", cls: "bg-secondary" },
          { name: "Accent", cls: "bg-accent" },
          { name: "Muted", cls: "bg-muted" },
          { name: "Destructive", cls: "bg-destructive" },
          { name: "Border", cls: "bg-border" },
        ].map(({ name, cls }) => (
          <div key={name} className="text-center">
            <div className={`h-10 rounded ${cls}`} />
            <span className="mt-1 text-xs text-muted-foreground">{name}</span>
          </div>
        ))}
      </div>
      <Separator />
    </div>
  ),
};
