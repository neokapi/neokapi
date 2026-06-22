import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Languages, Sparkles } from "lucide-react";
import { AdaptiveSidebar, DesktopFrame, ProjectKindBadge } from "./_shared";

/**
 * Prototype: the adaptive, project-type-aware sidebar.
 *
 * A content project shows only the content workspace. When the project enables
 * the localization feature, a clearly-grouped Localization set (Translate,
 * Translation Memories, Termbases) lights up — same content items, plus the
 * l10n surface.
 */
const meta = {
  title: "Prototype/AdaptiveSidebar",
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
  localization,
  badge,
  caption,
}: {
  project: string;
  localization: boolean;
  badge: ReactNode;
  caption: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3">
      <DesktopFrame title={project} badge={badge}>
        <div className="flex h-[420px]">
          <AdaptiveSidebar project={project} localization={localization} active="content" />
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
        <h1 className="text-xl font-semibold">One workspace, two shapes</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          The desktop is a content workspace that becomes a localization workspace when the open
          project enables the localization feature.
        </p>
        <div className="mt-6 grid gap-6 lg:grid-cols-2">
          <ShellCard
            project="Help Center Articles"
            localization={false}
            badge={<ProjectKindBadge kind="content" />}
            caption={
              <>
                A content project: Home, Content, Check, Rewrite, Stats, Brand. No Translate, TM, or
                Termbase — and flows are not a sidebar pillar.
              </>
            }
          />
          <ShellCard
            project="Acme Marketing Site"
            localization
            badge={<ProjectKindBadge kind="localization" />}
            caption={
              <span className="flex items-start gap-1.5">
                <Sparkles size={13} className="mt-0.5 shrink-0 text-primary" />
                <span>
                  The same content items, plus a grouped{" "}
                  <span className="font-medium text-foreground">Localization</span> set. These
                  appear only because the project enabled the localization feature.
                </span>
              </span>
            }
          />
        </div>
      </div>
    </div>
  ),
};

export const ContentProject: Story = {
  render: () => (
    <div className="flex h-screen bg-background text-foreground">
      <AdaptiveSidebar project="Help Center Articles" localization={false} active="content" />
      <PaneStub title="Content" />
    </div>
  ),
};

export const LocalizationProject: Story = {
  render: () => (
    <div className="flex h-screen bg-background text-foreground">
      <AdaptiveSidebar project="Acme Marketing Site" localization active="translate" />
      <div className="flex-1 bg-background p-6">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Languages size={18} className="text-primary" />
          Translate
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          The localization surface is available because this project enabled the feature.
        </p>
      </div>
    </div>
  ),
};
