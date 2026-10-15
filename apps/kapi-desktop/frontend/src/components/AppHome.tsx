import { useMemo, useState } from "react";
import { FolderOpen, Plus, Workflow } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  ErrorNotice,
  PageHeader,
  SectionHeading,
  SimpleTooltip,
} from "@neokapi/ui-primitives";
import { useShortenHome } from "../hooks/useShortenHome";
import { standingsOf } from "../hooks/useAppInit";
import { ConnectAICard } from "./ConnectAICard";
import { RemoveProjectDialog } from "./RemoveProjectDialog";
import { StartHere } from "./StartHere";
import { WhatsNew, WorkspaceNumbers } from "./WhatsNew";
import { WorkspaceConflicts } from "./WorkspaceConflicts";
import { WorkspaceProjectRow } from "./WorkspaceProjectRow";
import { WorkspaceRules } from "./WorkspaceRules";
import type {
  AIDetectionResult,
  ContextNews,
  ContextProjectStanding,
  KeptConflict,
  WorkspaceHome,
  WorkspaceProject,
  WorkspaceRule,
} from "../types/api";

interface AppHomeProps {
  /** The workspace and the projects it holds, null while it is being read. */
  workspace: WorkspaceHome | null;
  /** Why the workspace could not be read, when it could not. */
  workspaceError?: unknown;
  /** Every project's context in numbers. Absent reads them off the workspace's rows. */
  standings?: ContextProjectStanding[] | null;
  /** What each project's digest holds that the person has not seen. */
  news?: ContextNews[] | null;
  /** Pre-loaded workspace rules for Storybook and tests, which reach no backend. */
  workspaceRules?: WorkspaceRule[];
  /** The Start here steps were dismissed. */
  startHereDismissed: boolean;
  onDismissStartHere: () => void;
  /** A project tab is open, so Start here can connect an assistant to it. */
  hasOpenProject: boolean;
  /** Open the most recent project tab on its Assistant section. */
  onConnectAssistant: () => void;
  /** Open a project on its What's new, by workspace key. */
  onOpenWhatsNew: (key: string) => void;
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
  /** Pre-loaded AI detection for Storybook/tests — forwarded to ConnectAICard. */
  aiDetection?: AIDetectionResult;
  /** Pre-loaded .kpz document conflicts for Storybook and tests. */
  documentConflicts?: KeptConflict[];
}

/**
 * The app's first screen: the context this machine account's projects write
 * by, and what the assistants working in them noticed.
 *
 * Every project kapi has run in is listed, whichever surface ran it. A
 * repository set up from a terminal appears without anyone opening it in the
 * app, and a folder opened here joins the same list.
 */
export function AppHome({
  workspace,
  workspaceError,
  standings: given,
  news,
  workspaceRules,
  startHereDismissed,
  onDismissStartHere,
  hasOpenProject,
  onConnectAssistant,
  onOpenWhatsNew,
  onOpenCheckout,
  onOpenContext,
  onForgetProject,
  onNewProject,
  onOpenProject,
  onNavigate,
  onCreateSampleProject,
  aiDetection,
  documentConflicts,
}: AppHomeProps) {
  const shortenHome = useShortenHome();
  const [removing, setRemoving] = useState<WorkspaceProject | null>(null);
  const projects = workspace?.projects ?? [];
  const newsByProject = new Map((news ?? []).map((row) => [row.project, row]));
  const derived = useMemo(() => standingsOf(workspace), [workspace]);
  const standings = given ?? derived;

  return (
    <div data-slot="app-home" className="mx-auto max-w-3xl space-y-8 p-6">
      <div>
        <PageHeader
          className="mb-3"
          title="Your context"
          subtitle="What your projects write by, and what your assistants noticed."
          actions={
            <>
              <Button
                variant="link"
                size="sm"
                data-slot="home-history"
                className="h-auto px-0"
                onClick={() => onNavigate("history")}
              >
                History
              </Button>
              {workspace?.location && (
                <SimpleTooltip
                  content={t("Where Kapi keeps the context of every project you work on")}
                >
                  <span
                    className="max-w-64 truncate font-mono text-[11px] text-muted-foreground/70"
                    translate="no"
                  >
                    {shortenHome(workspace.location)}
                  </span>
                </SimpleTooltip>
              )}
            </>
          }
        />
        <WorkspaceNumbers standings={standings} />
      </div>

      {/* A .kpz document whose other version did not land, in a project or in
          none: decided here, so one outside every project has a place. */}
      <div className="empty:hidden">
        <WorkspaceConflicts conflicts={documentConflicts} />
      </div>

      <WhatsNew standings={standings} onOpen={onOpenWhatsNew} />

      {/* The workspace: every project kapi has run in, from any surface. */}
      <section data-slot="home-projects" aria-labelledby="home-projects-heading">
        <div className="mb-2 flex items-center justify-between gap-3">
          <div id="home-projects-heading">
            <SectionHeading>Projects</SectionHeading>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={onNewProject}>
              <Plus size={14} />
              New project
            </Button>
            <Button variant="outline" size="sm" onClick={onOpenProject}>
              <FolderOpen size={14} />
              Open a folder
            </Button>
          </div>
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
              Nothing here yet. Projects appear as soon as kapi runs in them, whether you open one
              here or run{" "}
              <code className="font-mono text-xs" translate="no">
                kapi
              </code>{" "}
              in a folder.
            </p>
          )
        )}
        {workspace?.read_only && (
          <p className="mt-2 text-[11px] text-muted-foreground">
            This workspace is open for reading only, so nothing new registers here.
          </p>
        )}
      </section>

      <WorkspaceRules rules={workspaceRules} />

      {!startHereDismissed && (
        <StartHere
          onDismiss={onDismissStartHere}
          onCreateSampleProject={onCreateSampleProject}
          onConnectAssistant={onConnectAssistant}
          hasOpenProject={hasOpenProject}
        />
      )}

      <RemoveProjectDialog
        project={removing}
        onClose={() => setRemoving(null)}
        onConfirm={onForgetProject}
      />

      {/* First-run: connect an AI provider. The card renders only when no
          provider is configured anywhere, and disappears once one is. */}
      <div className="empty:hidden">
        <ConnectAICard detection={aiDetection} />
      </div>

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
