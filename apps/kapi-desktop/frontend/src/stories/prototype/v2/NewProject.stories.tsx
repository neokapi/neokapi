import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ArrowRight, FileText, Plus, Upload } from "lucide-react";
import { Button, LocalePill } from "@neokapi/ui-primitives";

/**
 * Prototype v2 (source-first): one-door new project.
 *
 * No journey fork, no "content vs localization" self-classification. You point
 * Kapi at content and get parse / check / rewrite / brand / stats out of the
 * box. Languages are an optional dial — pre-add one here, or skip and add later
 * from the workspace. A source-only project is a complete setup, not a lesser one.
 */
const meta = {
  title: "Prototype v2/NewProject",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function NewProject() {
  const [langs, setLangs] = useState<string[]>([]);
  const has = langs.length > 0;

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
              <span className="text-muted-foreground"> — or </span>
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
        </div>

        {/* Languages — an optional dial, default none. */}
        <div className="mt-4 rounded-2xl border border-border bg-card p-5">
          <div className="text-sm font-semibold">
            Languages <span className="font-normal text-muted-foreground">— optional</span>
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Add a target language to turn on Translate, TM, and termbases. You can always add them
            later.
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">Source</span>
            <LocalePill locale="en-US" />
            {has && (
              <>
                <span className="text-muted-foreground">&rarr;</span>
                {langs.map((l) => (
                  <LocalePill key={l} locale={l} />
                ))}
              </>
            )}
            <Button
              variant="outline"
              size="xs"
              onClick={() => setLangs((s) => (s.includes("fr-FR") ? [...s, "de-DE"] : ["fr-FR"]))}
            >
              <Plus size={12} /> Add a language
            </Button>
          </div>
          {!has && (
            <p className="mt-2 text-xs text-muted-foreground/70">
              No languages yet — a source-only (monolingual) project. That&rsquo;s a complete setup;
              translation is a dial you turn up when ready.
            </p>
          )}
        </div>

        <div className="mt-6 flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            {has
              ? `${langs.length} language${langs.length > 1 ? "s" : ""} · Translate, TM & termbases on`
              : "Source-only project · add languages anytime"}
          </span>
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
