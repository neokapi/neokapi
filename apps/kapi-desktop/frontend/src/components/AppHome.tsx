import { useMemo, useState } from "react";
import { FolderOpen, Lock, Plus } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Badge,
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
import { WorkspaceProjectRow, lastSegment } from "./WorkspaceProjectRow";
import { WorkspaceRules } from "./WorkspaceRules";
import type {
  AIDetectionResult,
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
 * The app's first screen, in the overview shape (packages/ui/docs/page-patterns.md):
 * the context this machine account's projects write by, and what the
 * assistants working in them noticed. A header, a row of labelled counts, then
 * two lists of rows, each row leading to a project.
 *
 * Every project kapi has run in is listed, whichever surface ran it. A
 * repository set up from a terminal appears without anyone opening it in the
 * app, and a folder opened here joins the same list.
 */
export function AppHome({
  workspace,
  workspaceError,
  standings: given,
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
  const [removing, setRemoving] = useState<WorkspaceProject | null>(null);
  const projects = workspace?.projects ?? [];
  const derived = useMemo(() => standingsOf(workspace), [workspace]);
  const standings = given ?? derived;
  // On a first visit nothing is registered, so the three steps come first and
  // the counts and the sections that would only say "nothing yet" wait until
  // there is a project to fill them.
  const empty = !workspaceError && !!workspace && projects.length === 0;
  const startHere = !startHereDismissed && (
    <StartHere
      onDismiss={onDismissStartHere}
      onCreateSampleProject={onCreateSampleProject}
      onConnectAssistant={onConnectAssistant}
      hasOpenProject={hasOpenProject}
    />
  );

  return (
    <div data-slot="app-home" className="mx-auto max-w-4xl space-y-8 p-6">
      <div>
        <PageHeader
          className="mb-4"
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
              {workspace?.location && <WorkspaceLocation location={workspace.location} />}
              {workspace?.read_only && (
                <SimpleTooltip
                  content={t(
                    "This workspace is open for reading only, so nothing new registers here.",
                  )}
                >
                  <Badge
                    variant="outline"
                    data-slot="workspace-read-only"
                    tabIndex={0}
                    className="text-[11px] font-normal text-muted-foreground"
                  >
                    <Lock aria-hidden />
                    Read only
                  </Badge>
                </SimpleTooltip>
              )}
            </>
          }
        />
        {!empty && <WorkspaceNumbers standings={standings} projects={projects.length} />}
      </div>

      {/* A .kpz document whose other version did not land, in a project or in
          none: decided here, so one outside every project has a place. */}
      <div className="empty:hidden">
        <WorkspaceConflicts conflicts={documentConflicts} />
      </div>

      {empty && startHere}

      {!empty && <WhatsNew standings={standings} onOpen={onOpenWhatsNew} />}

      {/* The workspace: every project kapi has run in, from any surface. */}
      <section data-slot="home-projects" aria-labelledby="home-projects-heading">
        <div className="mb-1 flex items-center justify-between gap-3">
          <div id="home-projects-heading">
            <SectionHeading>Projects</SectionHeading>
          </div>
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="sm" onClick={onNewProject}>
              <Plus size={14} />
              New project
            </Button>
            <Button variant="ghost" size="sm" onClick={onOpenProject}>
              <FolderOpen size={14} />
              Open a folder
            </Button>
          </div>
        </div>

        {workspaceError ? (
          <ErrorNotice error={workspaceError} title="Your workspace could not be read" />
        ) : projects.length > 0 ? (
          <div className="divide-y divide-border/50">
            {projects.map((project) => (
              <WorkspaceProjectRow
                key={project.key}
                project={project}
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
      </section>

      {!empty && <WorkspaceRules rules={workspaceRules} />}

      {!empty && startHere}

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
    </div>
  );
}

/**
 * Where the workspace is, as a value: the last segment of its directory in a
 * mono chip, and the whole path in the tooltip.
 */
function WorkspaceLocation({ location }: { location: string }) {
  const shortenHome = useShortenHome();
  return (
    <SimpleTooltip
      content={
        <span className="block max-w-80">
          {t("Where Kapi keeps the context of every project you work on")}
          <span className="mt-0.5 block font-mono break-all" translate="no">
            {shortenHome(location)}
          </span>
        </span>
      }
    >
      <span
        data-slot="workspace-location"
        tabIndex={0}
        translate="no"
        className="max-w-40 truncate rounded-md border border-border/70 px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground"
      >
        {lastSegment(location)}
      </span>
    </SimpleTooltip>
  );
}
