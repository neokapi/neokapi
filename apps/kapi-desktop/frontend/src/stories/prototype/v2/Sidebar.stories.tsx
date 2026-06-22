import type { Meta, StoryObj } from "@storybook/react-vite";
import { DesktopFrame, SourceFirstSidebar } from "../_shared";

/**
 * Prototype v2 (source-first): one project shape; languages stay quiet.
 *
 * There is no "content project" vs "localization project", and nothing
 * announces a move from one language to many. A project simply shows the
 * languages its content uses; once it has target languages, the localization
 * tools are present under a plain group — the same as any other. Adding a
 * language is an ordinary setting (see the Project Languages story).
 */
const meta = {
  title: "Prototype v2/Sidebar",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

/** A neutral content pane so the sidebar is shown in context. */
function PaneStub({ title }: { title: string }) {
  return (
    <div className="flex-1 bg-background p-6">
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="mt-1 text-sm text-muted-foreground">Workspace content for the selected view.</p>
      <div className="mt-5 grid gap-3 sm:grid-cols-2">
        <div className="h-24 rounded-xl border border-dashed border-border" />
        <div className="h-24 rounded-xl border border-dashed border-border" />
      </div>
    </div>
  );
}

function ShellCard({
  project,
  source,
  targets,
}: {
  project: string;
  source: string;
  targets: string[];
}) {
  return (
    <DesktopFrame title={project}>
      <div className="flex h-[560px]">
        <SourceFirstSidebar project={project} source={source} targets={targets} active="content" />
        <PaneStub title="Content" />
      </div>
    </DesktopFrame>
  );
}

export const SideBySide: Story = {
  render: () => (
    <div className="min-h-screen bg-background p-8 text-foreground">
      <div className="mx-auto max-w-5xl">
        <h1 className="text-xl font-semibold">A project shows the languages it has</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          One project shape. The header states the languages as a fact; the localization tools are
          simply present once a project has targets. Adding a language is an ordinary setting — the
          workspace just reflects it, with nothing to announce.
        </p>
        <div className="mt-6 grid gap-6 lg:grid-cols-2">
          <ShellCard project="Help Center Articles" source="en-US" targets={[]} />
          <ShellCard
            project="Acme Marketing Site"
            source="en-US"
            targets={["fr-FR", "de-DE", "ja-JP"]}
          />
        </div>
      </div>
    </div>
  ),
};

export const OneLanguage: Story = {
  render: () => (
    <div className="flex h-screen bg-background text-foreground">
      <SourceFirstSidebar
        project="Help Center Articles"
        source="en-US"
        targets={[]}
        active="rewrite"
      />
      <PaneStub title="Rewrite" />
    </div>
  ),
};

export const SeveralLanguages: Story = {
  render: () => (
    <div className="flex h-screen bg-background text-foreground">
      <SourceFirstSidebar
        project="Acme Marketing Site"
        source="en-US"
        targets={["fr-FR", "de-DE", "ja-JP"]}
        active="translate"
      />
      <PaneStub title="Translate" />
    </div>
  ),
};
