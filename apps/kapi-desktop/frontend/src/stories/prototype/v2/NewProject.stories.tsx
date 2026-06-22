import type { Meta, StoryObj } from "@storybook/react-vite";
import { ArrowRight, FileText, Upload } from "lucide-react";
import { Button, LocalePill } from "@neokapi/ui-primitives";

/**
 * Prototype v2 (source-first): one-door new project.
 *
 * No journey fork, no "content vs localization" self-classification, and no
 * languages dial to narrate. You point Kapi at content and get the workspace.
 * Kapi states the detected source language as a fact; target languages are an
 * ordinary setting added later, from the project — not an onboarding decision.
 */
const meta = {
  title: "Prototype v2/NewProject",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function NewProject() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-8 text-foreground">
      <div className="w-full max-w-xl">
        <div className="mb-6">
          <h1 className="text-2xl font-semibold tracking-tight">New project</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Point Kapi at your content. You&rsquo;ll get parse, check, rewrite, brand, and stats out
            of the box.
          </p>
        </div>

        {/* The one door — a content source. */}
        <div className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border bg-muted/20 px-6 py-8 text-center">
            <Upload size={22} className="text-muted-foreground" />
            <div className="text-sm">
              <span className="font-medium">Drop a file or folder</span>
              <span className="text-muted-foreground">{" — or "}</span>
              <button type="button" className="font-medium text-primary hover:underline">
                browse
              </button>
            </div>
            <div className="text-xs text-muted-foreground">
              A file, a folder, a glob, or a <span className="font-mono">.kapi</span> recipe
            </div>
          </div>
          {/* Selected preview. */}
          <div className="mt-3 flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
            <FileText size={15} className="text-muted-foreground" />
            <span className="font-mono text-xs">src/locales/en-US.json</span>
            <span className="ml-auto text-xs text-muted-foreground">+ 42 files</span>
          </div>
          {/* Detected source language — a fact about the content, not a choice. */}
          <div className="mt-3 flex items-center gap-2 px-1 text-xs text-muted-foreground">
            <span>Source language</span>
            <LocalePill locale="en-US" />
            <span className="text-muted-foreground/70">· detected</span>
          </div>
        </div>

        <div className="mt-6 flex items-center justify-end">
          <Button>
            Create project
            <ArrowRight size={15} />
          </Button>
        </div>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <NewProject />,
};
