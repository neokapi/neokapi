import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ArrowRight, Check, Languages, Sparkles } from "lucide-react";
import { Button, LocalePill } from "@neokapi/ui-primitives";
import { AdaptiveSidebar, DesktopFrame } from "./_shared";

/**
 * Prototype: the "step up to localization" affordance.
 *
 * A content project carries a tasteful card to add localization. Enabling the
 * feature lights up the l10n surface — shown here as a live before → after of
 * the project sidebar.
 */
const meta = {
  title: "Prototype/StepUpToLocalization",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function StepUp() {
  const [enabled, setEnabled] = useState(false);

  return (
    <div className="flex min-h-screen items-start justify-center bg-background p-8 text-foreground">
      <div className="w-full max-w-5xl">
        <DesktopFrame
          title="Help Center Articles"
          badge={
            enabled ? (
              <span className="flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">
                <Languages size={10} />
                Localization
              </span>
            ) : (
              <span className="rounded-full border border-border px-2 py-0.5 text-[10px] text-muted-foreground">
                Content
              </span>
            )
          }
        >
          <div className="flex h-[460px]">
            <AdaptiveSidebar
              project="Help Center Articles"
              localization={enabled}
              active="project-settings"
            />

            <div className="flex-1 overflow-y-auto bg-background p-6">
              <h2 className="text-lg font-semibold">Project Settings</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Languages, plugins, and processing for this project.
              </p>

              {/* Step-up card. */}
              <div className="mt-6 rounded-2xl border border-primary/25 bg-primary/[0.04] p-5">
                <div className="flex items-start gap-4">
                  <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                    <Languages size={20} />
                  </div>
                  <div className="flex-1">
                    <h3 className="text-sm font-semibold">Add localization</h3>
                    <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                      Turn this content project into a localization project. You get target
                      languages, AI translation, QA, translation memory, and a termbase — your brand
                      voice and checks carry over unchanged.
                    </p>

                    {!enabled ? (
                      <div className="mt-4 flex items-center gap-3">
                        <Button onClick={() => setEnabled(true)}>
                          <Sparkles size={14} />
                          Enable localization
                        </Button>
                        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                          Adds
                          <span className="font-medium text-foreground">Translate</span>,
                          <span className="font-medium text-foreground">TM</span>, and
                          <span className="font-medium text-foreground">Termbases</span>
                          to the sidebar
                        </span>
                      </div>
                    ) : (
                      <div className="mt-4">
                        <div className="flex items-center gap-2 text-sm font-medium text-primary">
                          <Check size={15} />
                          Localization enabled
                        </div>
                        <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <span>Pick target languages to get started:</span>
                          <LocalePill locale="en-US" />
                          <ArrowRight size={12} />
                          <Button variant="outline" size="xs">
                            Add target language
                          </Button>
                        </div>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="mt-3 text-muted-foreground"
                          onClick={() => setEnabled(false)}
                        >
                          Reset prototype
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
              </div>

              <p className="mt-4 px-1 text-xs text-muted-foreground">
                {enabled
                  ? "The Localization group now appears in the sidebar on the left."
                  : "Watch the sidebar on the left — the Localization group appears when you enable the feature."}
              </p>
            </div>
          </div>
        </DesktopFrame>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <StepUp />,
};
