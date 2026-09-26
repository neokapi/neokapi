import { useState } from "react";
import { FolderKanban, FolderOpen, Sparkles, Workflow, X } from "lucide-react";
import { Button, ErrorNotice, SimpleTooltip } from "@neokapi/ui-primitives";
import { useShortenHome } from "../hooks/useShortenHome";
import { ConnectAICard } from "./ConnectAICard";
import { ContextFeedHint } from "./ContextFeed";
import { ContextFeedPanel } from "./ContextFeedPanel";
import { RemoveProjectDialog } from "./RemoveProjectDialog";
import { WorkspaceProjectRow } from "./WorkspaceProjectRow";
import type {
  AIDetectionResult,
  ContextNews,
  ContextFeed,
  WorkspaceHome,
  WorkspaceProject,
} from "../types/api";

interface AppHomeProps {
  /** The workspace and the projects it holds, null while it is being read. */
  workspace: WorkspaceHome | null;
  /** Why the workspace could not be read, when it could not. */
  workspaceError?: unknown;
  /** What each project's digest holds that the person has not seen. */
  news?: ContextNews[] | null;
  /** Pre-loaded feed for Storybook and tests, which reach no backend. */
  feed?: ContextFeed;
  samplesDismissed: boolean;
  /** Open a project from one of its checkouts, by recipe path. */
  onOpenCheckout: (recipe: string) => void;
  /** Open a project with no checkout here, on its context alone. */
  onOpenContext: (key: string) => void;
  /** Remove a project and the context the workspace holds for it. */
  onForgetProject: (key: string) => Promise<void> | void;
  onNewProject: () => void;
  onOpenProject: () => void;
  onNavigate: (view: string) => void;
  onCreateSampleProject: (name: string) => void;
  onDismissSamples: () => void;
  /** Pre-loaded AI detection for Storybook/tests — forwarded to ConnectAICard. */
  aiDetection?: AIDetectionResult;
}

/**
 * The app's first screen: this machine account's workspace.
 *
 * Every project kapi has run in is here, whichever surface ran it. A repository
 * set up from a terminal appears without anyone opening it in the app, and a
 * project opened here with "Open a Project" joins the same list.
 */
export function AppHome({
  workspace,
  workspaceError,
  news,
  feed,
  samplesDismissed,
  onOpenCheckout,
  onOpenContext,
  onForgetProject,
  onNewProject,
  onOpenProject,
  onNavigate,
  onCreateSampleProject,
  onDismissSamples,
  aiDetection,
}: AppHomeProps) {
  const shortenHome = useShortenHome();
  const [removing, setRemoving] = useState<WorkspaceProject | null>(null);
  const projects = workspace?.projects ?? [];
  const newsByProject = new Map((news ?? []).map((row) => [row.project, row]));
  return (
    <div className="mx-auto max-w-3xl p-6">
      <div className="mb-8 flex items-center gap-4">
        <img src="/neokapi-logo.png" alt="neokapi" className="h-12 w-12 drop-shadow-lg" />
        <div>
          <h1 className="text-xl font-semibold">Welcome to Kapi</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Your project&apos;s context, applied to real files, for people and agents.
          </p>
        </div>
      </div>

      {/* Primary: start or open a project. This is the day-to-day model. */}
      <section className="mb-8">
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Projects
        </h2>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Button
            variant="outline"
            onClick={onNewProject}
            className="h-auto whitespace-normal rounded-lg border-primary/30 bg-primary/5 p-5 text-left flex-col items-start hover:border-primary/50 hover:bg-primary/10"
          >
            <FolderKanban size={22} className="mb-2 text-primary" />
            <div className="text-base font-semibold">New Project</div>
            <div className="text-xs text-muted-foreground font-normal">
              Create a Kapi project with content, flows, and languages
            </div>
          </Button>
          <Button
            variant="outline"
            onClick={onOpenProject}
            className="h-auto whitespace-normal rounded-lg border-primary/30 bg-primary/5 p-5 text-left flex-col items-start hover:border-primary/50 hover:bg-primary/10"
          >
            <FolderOpen size={22} className="mb-2 text-primary" />
            <div className="text-base font-semibold">Open a Project</div>
            <div className="text-xs text-muted-foreground font-normal">
              Open an existing kapi project folder from disk
            </div>
          </Button>
        </div>
      </section>

      {/* The workspace: every project kapi has run in, from any surface. */}
      <section className="mb-8">
        <div className="mb-3 flex items-baseline justify-between gap-3">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Your Projects
          </h2>
          {workspace?.location && (
            <SimpleTooltip content="Where Kapi keeps the context of every project you work on">
              <span className="truncate font-mono text-[11px] text-muted-foreground/70">
                {shortenHome(workspace.location)}
              </span>
            </SimpleTooltip>
          )}
        </div>

        {workspaceError ? (
          <ErrorNotice error={workspaceError} title="Your workspace could not be read" />
        ) : projects.length > 0 ? (
          <div className="space-y-2">
            {projects.map((project) => (
              <WorkspaceProjectRow
                key={project.key}
                project={project}
                news={newsByProject.get(project.key)}
                onOpenCheckout={onOpenCheckout}
                onOpenContext={onOpenContext}
                onRemove={setRemoving}
              />
            ))}
          </div>
        ) : (
          workspace && (
            <p className="text-sm text-muted-foreground">
              Nothing here yet. Projects appear as soon as Kapi runs in them, whether you open one
              here or run <code className="font-mono text-xs">kapi</code> in a folder.
            </p>
          )
        )}
        {workspace?.read_only && (
          <p className="mt-2 text-xs text-muted-foreground">
            This workspace is open for reading only, so nothing new registers here.
          </p>
        )}
      </section>

      {/* What has been recorded about context, across every project. An agent
          works in another process; this is where its proposals arrive and
          where they are decided. */}
      <section className="mb-8">
        <div className="mb-3 flex items-baseline justify-between gap-3">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Recorded
          </h2>
          <ContextFeedHint />
        </div>
        <ContextFeedPanel feed={feed} />
      </section>

      <RemoveProjectDialog
        project={removing}
        onClose={() => setRemoving(null)}
        onConfirm={onForgetProject}
      />

      {/* First-run: connect an AI provider. The card renders only when no
          provider is configured anywhere, and disappears once one is. */}
      <ConnectAICard detection={aiDetection} />

      {/* Sample projects — shown until explicitly dismissed */}
      {!samplesDismissed && (
        <section className="mb-8">
          <div className="mb-3 flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Sparkles size={14} />
            <span className="flex-1">New to Kapi? Try a sample project</span>
            <SimpleTooltip content="Dismiss">
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={onDismissSamples}
                className="text-muted-foreground/60"
                aria-label="Dismiss"
              >
                <X size={14} />
              </Button>
            </SimpleTooltip>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Button
              variant="outline"
              data-testid="sample-kapimart"
              onClick={() => onCreateSampleProject("kapimart")}
              className="h-auto whitespace-normal rounded-lg border-primary/20 bg-primary/5 p-4 text-left flex-col items-start hover:border-primary/40 hover:bg-primary/10"
            >
              <div className="text-sm font-medium">KapiMart</div>
              <p className="mt-1 text-xs text-muted-foreground font-normal">
                A realistic multilingual project with docs, store UI, Office documents, and
                templates: 4 content collections, 5 target languages, 1000+ memory entries. No
                plugins needed.
              </p>
            </Button>
          </div>
        </section>
      )}

      {/* Secondary: ad-hoc quick tools — one-off, no project required. */}
      <section>
        <h2 className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground/70">
          Quick tools
        </h2>
        <p className="mb-3 text-xs text-muted-foreground/70">
          One-off actions that don&apos;t need a project. Results aren&apos;t saved to a project.
        </p>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Button
            variant="ghost"
            onClick={() => onNavigate("flows")}
            className="h-auto whitespace-normal rounded-lg border border-border/60 p-3 text-left flex-row items-center gap-3 hover:bg-accent/30"
          >
            <Workflow size={16} className="shrink-0 text-muted-foreground" />
            <div>
              <div className="text-sm font-medium">Design a Flow</div>
              <div className="text-xs text-muted-foreground font-normal">Build tool pipelines</div>
            </div>
          </Button>
        </div>
      </section>
    </div>
  );
}
