import type { Meta, StoryObj } from "@storybook/react-vite";

// Import primitives from the shared package to demonstrate they work
// in the framework context (no platform dependency).

const meta: Meta = {
  title: "Foundations/UI Primitives",
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj;

export const Buttons: Story = {
  render: () => (
    <div className="flex flex-wrap gap-3 p-4">
      <button className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
        Primary
      </button>
      <button className="rounded-md border border-border bg-background px-4 py-2 text-sm font-medium hover:bg-accent">
        Secondary
      </button>
      <button className="rounded-md bg-destructive px-4 py-2 text-sm font-medium text-destructive-foreground hover:bg-destructive/90">
        Destructive
      </button>
      <button className="rounded-md px-4 py-2 text-sm font-medium text-muted-foreground hover:bg-accent">
        Ghost
      </button>
      <button className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground opacity-50" disabled>
        Disabled
      </button>
    </div>
  ),
};

export const Cards: Story = {
  render: () => (
    <div className="grid grid-cols-2 gap-4 p-4" style={{ maxWidth: 600 }}>
      <div className="rounded-lg border border-border p-4">
        <h3 className="text-sm font-medium">Card Title</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          Card description with some content.
        </p>
      </div>
      <div className="rounded-lg border border-border bg-accent/30 p-4">
        <h3 className="text-sm font-medium">Accent Card</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          With background accent.
        </p>
      </div>
    </div>
  ),
};

export const Inputs: Story = {
  render: () => (
    <div className="max-w-sm space-y-3 p-4">
      <div>
        <label className="mb-1 block text-sm font-medium">Text Input</label>
        <input
          type="text"
          placeholder="Enter text..."
          className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
        />
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium">Disabled</label>
        <input
          type="text"
          value="Read-only value"
          disabled
          className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm opacity-50 outline-none"
        />
      </div>
    </div>
  ),
};

export const Badges: Story = {
  render: () => (
    <div className="flex flex-wrap gap-2 p-4">
      <span className="rounded bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
        Primary
      </span>
      <span className="rounded bg-accent px-2 py-0.5 text-xs">
        Accent
      </span>
      <span className="rounded bg-destructive/10 px-2 py-0.5 text-xs text-destructive">
        Destructive
      </span>
      <span className="rounded bg-green-500/10 px-2 py-0.5 text-xs text-green-500">
        Success
      </span>
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
    </div>
  ),
};
