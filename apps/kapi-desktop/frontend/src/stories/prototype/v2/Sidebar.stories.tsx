import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { DesktopFrame, LanguagesChip, SourceFirstSidebar } from "../_shared";

/**
 * Prototype v2 (source-first): one project shape, a languages dial.
 *
 * There is no "content project" vs "localization project". Every project shows
 * the same workspace; the Localization group is ALWAYS present — an empty-state
 * "Add a language" CTA until a language is added, then the active Translate /
 * Translation Memory / Termbase surface. Nothing about the project "kind"
 * changes; only the dial moves.
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
  languages,
  caption,
}: {
  project: string;
  languages: string[];
  caption: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3">
      <DesktopFrame title={project} badge={<LanguagesChip targets={languages} />}>
        <div className="flex h-[600px]">
          <SourceFirstSidebar project={project} languages={languages} active="content" />
          <PaneStub title="Content" />
        </div>
      </DesktopFrame>
      <p className="px-1 text-xs leading-relaxed text-muted-foreground">{caption}</p>
    </div>
  );
}

export const SideBySide: Story = {
  render: () => (
    <div className="min-h-screen bg-background p-8 text-foreground">
      <div className="mx-auto max-w-5xl">
        <h1 className="text-xl font-semibold">One project shape, a languages dial</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          No content-vs-localization fork. The Localization group is always present — an empty-state
          CTA until you add a language, then the active Translate / TM / Termbase surface. Same
          project; one dial.
        </p>
        <div className="mt-6 grid gap-6 lg:grid-cols-2">
          <ShellCard
            project="Help Center Articles"
            languages={[]}
            caption={
              <>
                No languages yet — a complete, source-only project. The Localization group shows a
                single <span className="font-medium text-foreground">Add a language</span> CTA,
                never hidden, so the dial is always one click away.
              </>
            }
          />
          <ShellCard
            project="Acme Marketing Site"
            languages={["fr-FR", "de-DE", "ja-JP"]}
            caption={
              <>
                Three languages added — the same project, with Translate, Translation Memory, and
                Termbases switched on. Nothing about the project &ldquo;kind&rdquo; changed; only
                the dial moved.
              </>
            }
          />
        </div>
      </div>
    </div>
  ),
};

export const SourceOnly: Story = {
  render: () => (
    <div className="flex h-screen bg-background text-foreground">
      <SourceFirstSidebar project="Help Center Articles" languages={[]} active="rewrite" />
      <PaneStub title="Rewrite" />
    </div>
  ),
};

export const Multilingual: Story = {
  render: () => (
    <div className="flex h-screen bg-background text-foreground">
      <SourceFirstSidebar
        project="Acme Marketing Site"
        languages={["fr-FR", "de-DE", "ja-JP"]}
        active="translate"
      />
      <PaneStub title="Translate" />
    </div>
  ),
};
