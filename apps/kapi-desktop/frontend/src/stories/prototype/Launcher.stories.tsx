import type { Meta, StoryObj } from "@storybook/react-vite";
import { ArrowRight, FolderKanban, FolderOpen, Languages, ShieldCheck, Wrench } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import { Chip, JourneyCard, LocaleRoute, ProjectKindBadge, recentProjects } from "./_shared";

/**
 * Prototype: the front-page launcher shown when no project is open.
 *
 * Two co-equal journeys are the primary entry — "Keep content on brand" and
 * "Localize content" — with recent projects and an ad-hoc Quick Tools entry
 * below. Content-first, but localization is unmistakably present.
 */
const meta = {
  title: "Prototype/Launcher",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function Launcher() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* Soft hero wash to lift the welcome without marketing flourish. */}
      <div className="bg-gradient-to-b from-primary/[0.05] to-transparent">
        <div className="mx-auto max-w-4xl px-8 pb-4 pt-12">
          <div className="flex items-center gap-4">
            <img src="/neokapi-logo.png" alt="neokapi" className="h-12 w-12 drop-shadow-lg" />
            <div>
              <h1 className="text-2xl font-semibold tracking-tight">Welcome to Kapi</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                Start with a journey, or pick up where you left off.
              </p>
            </div>
          </div>
        </div>
      </div>

      <div className="mx-auto max-w-4xl px-8 pb-16">
        {/* Primary: two co-equal journeys. */}
        <section className="mt-2">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <JourneyCard
              icon={<ShieldCheck size={22} />}
              eyebrow="Content"
              title="Keep content on brand"
              description="Brand voice, terminology, and quality checks. Keep every draft on-message — no translation required."
              chips={
                <>
                  <Chip>Brand voice</Chip>
                  <Chip>Checks</Chip>
                  <Chip>Rewrite</Chip>
                </>
              }
              footer={
                <span className="flex items-center gap-1.5 text-sm font-medium text-primary">
                  Start a content project
                  <ArrowRight
                    size={15}
                    className="transition-transform group-hover:translate-x-0.5"
                  />
                </span>
              }
            />
            <JourneyCard
              icon={<Languages size={22} />}
              eyebrow="Localization"
              title="Localize content"
              description="Source and target languages, AI translation, QA, and translation memory — on top of the same brand tooling."
              chips={
                <>
                  <Chip>Translate</Chip>
                  <Chip>QA</Chip>
                  <Chip>Translation memory</Chip>
                </>
              }
              footer={
                <span className="flex items-center gap-1.5 text-sm font-medium text-primary">
                  Start a localization project
                  <ArrowRight
                    size={15}
                    className="transition-transform group-hover:translate-x-0.5"
                  />
                </span>
              }
            />
          </div>
          <div className="mt-3 flex items-center justify-center gap-2 text-xs text-muted-foreground">
            <FolderOpen size={13} />
            <span>Already have a project?</span>
            <button type="button" className="font-medium text-primary hover:underline">
              Open from disk
            </button>
          </div>
        </section>

        {/* Recent projects. */}
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
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">{p.name}</span>
                    <ProjectKindBadge kind={p.kind} />
                  </div>
                  <div className="mt-0.5 flex items-center gap-2 truncate text-xs text-muted-foreground">
                    <span className="truncate">{p.path}</span>
                  </div>
                </div>
                {p.langs && <LocaleRoute source={p.langs.source} targets={p.langs.targets} />}
              </button>
            ))}
          </div>
        </section>

        {/* Ad-hoc Quick Tools — one-off, no project. */}
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
