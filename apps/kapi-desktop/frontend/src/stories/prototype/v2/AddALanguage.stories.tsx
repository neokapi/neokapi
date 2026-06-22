import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ArrowRight, Check, Languages, Plus } from "lucide-react";
import { Button, LocalePill } from "@neokapi/ui-primitives";
import { DesktopFrame, LanguagesChip, SourceFirstSidebar } from "../_shared";

/**
 * Prototype v2 (source-first): the "Add a language" affordance.
 *
 * Not "enable localization" — there is no separate localization mode to switch
 * on. Adding a target language is the ordinary dial that turns on the Translate /
 * TM / Termbase surface; brand voice, terminology, and checks carry into every
 * language unchanged. Shown as a live before → after of the project sidebar.
 */
const meta = {
  title: "Prototype v2/AddALanguage",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function AddALanguage() {
  const [langs, setLangs] = useState<string[]>([]);
  const has = langs.length > 0;

  return (
    <div className="flex min-h-screen items-start justify-center bg-background p-8 text-foreground">
      <div className="w-full max-w-5xl">
        <DesktopFrame title="Help Center Articles" badge={<LanguagesChip targets={langs} />}>
          <div className="flex h-[560px]">
            <SourceFirstSidebar
              project="Help Center Articles"
              languages={langs}
              active="project-settings"
              onAddLanguage={() => setLangs(["fr-FR"])}
            />

            <div className="flex-1 overflow-y-auto bg-background p-6">
              <h2 className="text-lg font-semibold">Project Settings</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Languages, plugins, and processing for this project.
              </p>

              <div className="mt-6 rounded-2xl border border-primary/25 bg-primary/[0.04] p-5">
                <div className="flex items-start gap-4">
                  <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                    <Languages size={20} />
                  </div>
                  <div className="flex-1">
                    <h3 className="text-sm font-semibold">Languages</h3>
                    <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                      Add a target language to turn on Translate, translation memory, and termbases.
                      Your brand voice, terminology, and checks carry into every language unchanged.
                    </p>

                    {!has ? (
                      <div className="mt-4 flex items-center gap-3">
                        <Button onClick={() => setLangs(["fr-FR"])}>
                          <Plus size={14} />
                          Add a language
                        </Button>
                        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                          Turns on
                          <span className="font-medium text-foreground">Translate</span>,
                          <span className="font-medium text-foreground">TM</span>, and
                          <span className="font-medium text-foreground">Termbases</span>
                        </span>
                      </div>
                    ) : (
                      <div className="mt-4">
                        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <span>Source</span>
                          <LocalePill locale="en-US" />
                          <ArrowRight size={12} />
                          {langs.map((l) => (
                            <LocalePill key={l} locale={l} />
                          ))}
                          <Button
                            variant="outline"
                            size="xs"
                            onClick={() =>
                              setLangs((s) => (s.includes("de-DE") ? s : [...s, "de-DE"]))
                            }
                          >
                            <Plus size={12} /> Add another
                          </Button>
                        </div>
                        <div className="mt-3 flex items-center gap-2 text-sm font-medium text-primary">
                          <Check size={15} />
                          Translate, TM &amp; termbases are on
                        </div>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="mt-2 text-muted-foreground"
                          onClick={() => setLangs([])}
                        >
                          Reset prototype
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
              </div>

              <p className="mt-4 px-1 text-xs text-muted-foreground">
                {has
                  ? "The Localization group in the sidebar now lists Translate, Translation Memory, and Termbases."
                  : "Watch the sidebar on the left — the Localization group's Add-a-language CTA becomes the active surface."}
              </p>
            </div>
          </div>
        </DesktopFrame>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <AddALanguage />,
};
