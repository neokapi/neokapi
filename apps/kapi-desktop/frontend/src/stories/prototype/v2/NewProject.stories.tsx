import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ChangeEvent, useState } from "react";
import { ChevronDown, Folder } from "lucide-react";
import { Button, Input, Label, LocalePill } from "@neokapi/ui-primitives";

/**
 * Prototype v2 (source-first): new project — mirrors the shipped NewProjectDialog.
 *
 * Creation is name-based, matching the existing flow rather than reinventing it
 * as a drag/drop: a project is created at ~/KapiProjects/{name} (or a location
 * you browse to), and content is added afterward. Dropping a file is the ad-hoc
 * Quick Tools gesture, not project creation. The one source-first addition over
 * today's dialog is a quiet source-language default (en-US); target languages
 * stay empty and are added later as an ordinary project setting.
 */
const meta = {
  title: "Prototype v2/NewProject",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function NewProjectDialog() {
  const [name, setName] = useState("Acme Marketing Site");
  const trimmed = name.trim();
  const saveDir = trimmed ? `~/KapiProjects/${trimmed}` : " ";

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-8 text-foreground">
      {/* The modal card mirrors the shipped NewProjectDialog. */}
      <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
        <h2 className="mb-4 text-lg font-semibold">New Project</h2>
        <div className="space-y-3">
          {/* Name (or a browsed Location) — the durable project, in a known place. */}
          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">Name</Label>
            <div className="flex items-center gap-1.5">
              <Input
                value={name}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
                placeholder="My App"
                className="flex-1"
              />
              <Button
                variant="outline"
                size="icon-sm"
                aria-label="Choose location"
                className="shrink-0"
              >
                <Folder size={16} strokeWidth={1.5} />
              </Button>
            </div>
            <p className="mt-1 truncate text-xs text-muted-foreground">{saveDir}</p>
          </div>

          {/* Source language — the one source-first addition; a quiet default. */}
          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">Source language</Label>
            <button
              type="button"
              className="flex w-full items-center justify-between rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            >
              <span className="flex items-center gap-2">
                <LocalePill locale="en-US" />
                <span className="text-muted-foreground">English (United States)</span>
              </span>
              <ChevronDown size={15} className="text-muted-foreground" />
            </button>
          </div>

          <div className="flex gap-2 pt-1">
            <Button className="flex-1">Create Project</Button>
            <Button variant="outline">Cancel</Button>
          </div>
        </div>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <NewProjectDialog />,
};
