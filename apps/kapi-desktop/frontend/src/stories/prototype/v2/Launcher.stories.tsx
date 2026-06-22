import type { Meta, StoryObj } from "@storybook/react-vite";
import { ArrowRight, FilePlus2, FolderKanban, FolderOpen, Wrench } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import { LocaleRoute, recentProjects } from "../_shared";

/**
 * Prototype v2 (source-first): the launcher with ONE door.
 *
 * No "choose a journey" fork. You point Kapi at your content and land in the
 * workspace. Recents simply show the languages a project uses when it has any —
 * no badge naming a project as "source only" or counting languages as a status.
 */
const meta = {
  title: "Prototype v2/Launcher",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function Launcher() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* Soft hero wash to lift the welcome without marketing flourish. */}
      <div className="bg-gradient-to-b from-primary/[0.05] to-transparent">
        <div className="mx-auto max-w-3xl px-8 pb-4 pt-12">
          <div className="flex items-center gap-4">
            <img src="/neokapi-logo.png" alt="neokapi" className="h-12 w-12 drop-shadow-lg" />
            <div>
              <h1 className="text-2xl font-semibold tracking-tight">Welcome to Kapi</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                Get your content right — check, rewrite, brand, and translate.
              </p>
            </div>
          </div>
        </div>
      </div>

      <div className="mx-auto max-w-3xl px-8 pb-16">
        {/* Primary: ONE door — point at content. */}
        <section className="mt-2">
          <button
            type="button"
            className="group flex w-full items-center gap-4 rounded-2xl border border-primary/40 bg-primary/[0.04] p-6 text-left transition-colors hover:border-primary/60 hover:bg-primary/[0.07]"
          >
            <div className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <FilePlus2 size={22} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-lg font-semibold">New project</div>
              <p className="mt-1 text-sm text-muted-foreground">
                Point Kapi at your content — a file, a folder, a glob, or a{" "}
                <span className="font-mono text-xs">.kapi</span> recipe.
              </p>
            </div>
            <ArrowRight
              size={18}
              className="ml-auto shrink-0 text-primary transition-transform group-hover:translate-x-0.5"
            />
          </button>
          <div className="mt-3 flex items-center justify-center gap-2 text-xs text-muted-foreground">
            <FolderOpen size={13} />
            <span>Already have a project?</span>
            <button type="button" className="font-medium text-primary hover:underline">
              Open from disk
            </button>
          </div>
        </section>

        {/* Recent projects — show languages only when a project has them. */}
        <section className="mt-10">
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Recent projects
          </h2>
          <div className="space-y-1.5">
            {recentProjects.map((p) => (
              <button
                key={p.path}
                type="button"
                className="flex w-full items-center gap-3 rounded-lg border border-border p-3 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
              >
                <FolderKanban size={16} className="shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{p.name}</div>
                  <div className="mt-0.5 flex items-center gap-2 truncate text-xs text-muted-foreground">
                    <span className="truncate">{p.path}</span>
                  </div>
                </div>
                {p.langs && <LocaleRoute source={p.langs.source} targets={p.langs.targets} />}
              </button>
            ))}
          </div>
        </section>

        {/* Ad-hoc Quick Tools — one-off, no project (unchanged from v1). */}
        <section className="mt-10">
          <h2 className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground/70">
            Quick tools
          </h2>
          <p className="mb-3 text-xs text-muted-foreground/70">
            One-off actions on a file. Nothing is saved to a project.
          </p>
          <Button
            variant="ghost"
            className="flex h-auto w-full flex-row items-center gap-3 rounded-lg border border-border/60 p-3 text-left hover:bg-accent/30"
          >
            <Wrench size={16} className="shrink-0 text-muted-foreground" />
            <div>
              <div className="text-sm font-medium">Open the toolbox</div>
              <div className="text-xs font-normal text-muted-foreground">
                Drop a file and run a single operation — check, rewrite, inspect, convert,
                translate.
              </div>
            </div>
            <ArrowRight size={15} className="ml-auto text-muted-foreground" />
          </Button>
        </section>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <Launcher />,
};
