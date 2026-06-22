import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Plus, X } from "lucide-react";
import { Button, LocalePill } from "@neokapi/ui-primitives";
import { DesktopFrame, SourceFirstSidebar } from "../_shared";

/**
 * Prototype v2 (source-first): managing a project's languages.
 *
 * Adding a language is an ordinary setting — no "enable localization", no
 * "turns on Translate / TM / Termbases", no before/after drama. You add or
 * remove target languages like any other project property, and the sidebar
 * quietly reflects the result. The transition from one language to many is
 * transparent precisely because nothing marks it as an event.
 */
const meta = {
  title: "Prototype v2/Project Languages",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

const CANDIDATES = ["ja-JP", "es-ES", "pt-BR"];

function ProjectLanguages() {
  const [targets, setTargets] = useState<string[]>(["fr-FR", "de-DE"]);
  const next = CANDIDATES.find((c) => !targets.includes(c));

  return (
    <div className="flex min-h-screen items-start justify-center bg-background p-8 text-foreground">
      <div className="w-full max-w-5xl">
        <DesktopFrame title="Acme Marketing Site">
          <div className="flex h-[520px]">
            <SourceFirstSidebar
              project="Acme Marketing Site"
              source="en-US"
              targets={targets}
              active="project-settings"
            />

            <div className="flex-1 overflow-y-auto bg-background p-6">
              <h2 className="text-lg font-semibold">Project Settings</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Languages, plugins, and processing for this project.
              </p>

              <div className="mt-6 max-w-md">
                <h3 className="text-sm font-semibold">Languages</h3>

                <div className="mt-3 flex items-center gap-2 text-sm">
                  <span className="w-28 text-muted-foreground">Source</span>
                  <LocalePill locale="en-US" />
                  <span className="text-xs text-muted-foreground/70">· detected</span>
                </div>

                <div className="mt-3 flex items-start gap-2 text-sm">
                  <span className="w-28 pt-1 text-muted-foreground">Target languages</span>
                  <div className="flex flex-1 flex-wrap items-center gap-1.5">
                    {targets.map((l) => (
                      <span
                        key={l}
                        className="inline-flex items-center gap-1 rounded-full border border-border bg-muted/40 py-0.5 pl-2 pr-1 text-xs"
                      >
                        {l}
                        <button
                          type="button"
                          onClick={() => setTargets((s) => s.filter((x) => x !== l))}
                          className="rounded-full p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                          aria-label={`Remove ${l}`}
                        >
                          <X size={11} />
                        </button>
                      </span>
                    ))}
                    {next && (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() => setTargets((s) => [...s, next])}
                      >
                        <Plus size={12} /> Add language
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </DesktopFrame>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <ProjectLanguages />,
};
