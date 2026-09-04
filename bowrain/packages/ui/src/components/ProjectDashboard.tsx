import {
  Badge,
  Button,
  Card,
  CardContent,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  PageHeader,
  cn,
} from "@neokapi/ui-primitives";
import { useCallback, useState } from "react";
import type { DragEvent } from "react";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type { ProjectInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { ListCapRow } from "./ListCapRow";
import { LoopStatusRow, type LoopStatusData } from "./LoopStatusRow";
import { ProjectFormDialog } from "./ProjectFormDialog";
import type { ProjectFormData } from "./ProjectFormDialog";
import { StarterPromptCard } from "./StarterPromptCard";
import { TEST_IDS } from "../test-ids";
import {
  FolderOpen,
  ArrowRight,
  Upload,
  Globe,
  FileText,
  Plus,
  Clock,
  Sparkles,
  MoreHorizontal,
  Pencil,
  Trash2,
  UserPlus,
} from "./icons";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Compute relative time string from an ISO timestamp. */
function relativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diff = now - then;
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

/**
 * Word count for a project card: prefer the server-computed aggregate (the
 * projects list is a summary without items[]), fall back to summing embedded
 * items for adapters that still return the full shape (desktop offline path,
 * stories).
 */
function projectWordCount(project: ProjectInfo): number {
  return (
    project.word_count ?? project.items?.reduce((acc, item) => acc + (item.word_count ?? 0), 0) ?? 0
  );
}

/** File count: server aggregate first, embedded items as fallback. */
function projectFileCount(project: ProjectInfo): number {
  return project.item_count ?? project.items?.length ?? 0;
}

/** Stream count: server aggregate first, embedded streams as fallback. */
function projectStreamCount(project: ProjectInfo): number {
  return project.stream_count ?? project.streams?.length ?? 0;
}

/** Hard render cap for the project grid — the page scrolls, the DOM stays bounded. */
const MAX_PROJECT_CARDS = 200;

/** Format a number with compact notation (e.g. 1.2k). */
function compactNumber(n: number): string {
  if (n < 1000) return String(n);
  if (n < 10_000) return `${(n / 1000).toFixed(1)}k`;
  if (n < 1_000_000) return `${Math.round(n / 1000)}k`;
  return `${(n / 1_000_000).toFixed(1)}M`;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ProjectDashboardProps {
  projects: ProjectInfo[];
  /**
   * Create a project. `files`, when present, are files the user dropped on the
   * first-run create card — the caller uploads them into the new project
   * before navigating (the existing upload-after-create flow).
   */
  onCreateProject: (
    name: string,
    sourceLang: string,
    targetLangs: string[],
    files?: File[],
  ) => void;
  onOpenProject: (project: ProjectInfo) => void;
  /** Optional callback to create a sample project for first-time users. */
  onCreateSampleProject?: () => void;
  /** Workspace name shown in the greeting. */
  workspaceName?: string;
  /** Called to update a project's name and target locales. */
  onEditProject?: (projectId: string, data: { name?: string; target_languages?: string[] }) => void;
  /** Called to archive (soft delete) a project. */
  onArchiveProject?: (projectId: string) => void;
  /** Workspace languages for locale pickers. */
  workspaceLanguages?: string[];
  /** Opens the AI context scan (epic 016) from the first-run empty state. */
  onScanVoice?: () => void;
  /** Opens the members settings surface from the first-run invite card. */
  onInviteTeam?: () => void;
  /** Server origin shown in the copyable starter-pack prompt (web shells). */
  serverUrl?: string;
  /**
   * Loop-status layer for the populated state: latest loop activity, the
   * latest convergence run, the caller's open review tasks, the ship-state
   * rollup, and the voice rollup summary. Absent hides the layer (first
   * paint, desktop shells without the data).
   */
  loopStatus?: LoopStatusData;
  /** Opens the workspace activity feed (loop-status card). */
  onOpenActivities?: () => void;
  /** Opens the latest run's runs page (loop-status card). */
  onOpenRuns?: () => void;
  /** Opens the workspace task queue (loop-status card). */
  onOpenTasks?: () => void;
  /** Opens the workspace review inbox (the "Awaiting review" loop-status card). */
  onOpenReview?: () => void;
  /** Opens the delivery/translation dashboard surface (loop-status card). */
  onOpenDelivery?: () => void;
  /** Opens the voice dashboard (loop-status card). */
  onOpenVoiceDashboard?: () => void;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/**
 * Summary statistics strip shown between the loop-status layer and the grid.
 *
 * Each label agrees with its own number. A workspace holding one project read
 * "1 Projects" — and the label is not decoration, it is the sentence the number
 * is the subject of. The count driving the form is the true one even where the
 * displayed value is abbreviated: 1,000 words shows as "1k" and is still plural.
 */
function DashboardStats({ projects }: { projects: ProjectInfo[] }) {
  const totalWords = projects.reduce((acc, p) => acc + projectWordCount(p), 0);
  const uniqueLocales = new Set(projects.flatMap((p) => p.target_languages));
  const totalFiles = projects.reduce((acc, p) => acc + projectFileCount(p), 0);

  const stats = [
    {
      key: "projects",
      value: String(projects.length),
      label: (
        <Plural count={projects.length}>
          <One>Project</One>
          <Other>Projects</Other>
        </Plural>
      ),
    },
    {
      key: "words",
      value: compactNumber(totalWords),
      label: (
        <Plural count={totalWords}>
          <One>Word</One>
          <Other>Words</Other>
        </Plural>
      ),
    },
    {
      key: "languages",
      value: String(uniqueLocales.size),
      label: (
        <Plural count={uniqueLocales.size}>
          <One>Language</One>
          <Other>Languages</Other>
        </Plural>
      ),
    },
    {
      key: "files",
      value: String(totalFiles),
      label: (
        <Plural count={totalFiles}>
          <One>File</One>
          <Other>Files</Other>
        </Plural>
      ),
    },
  ];

  return (
    <div className="mb-8 flex flex-wrap items-center gap-x-10 gap-y-3 rounded-lg border border-border/60 bg-muted/20 px-6 py-4">
      {stats.map((s) => (
        <div key={s.key} className="flex items-baseline gap-2">
          <span className="text-xl font-semibold tabular-nums leading-none">{s.value}</span>
          <span className="text-xs uppercase tracking-wider text-muted-foreground">{s.label}</span>
        </div>
      ))}
    </div>
  );
}

/** Single project card in the grid. */
function ProjectCard({
  project,
  onOpen,
  onRename,
  onArchive,
  getDisplayName,
}: {
  project: ProjectInfo;
  onOpen: () => void;
  onRename?: () => void;
  onArchive?: () => void;
  getDisplayName: (code: string) => string;
}) {
  const wordCount = projectWordCount(project);
  const fileCount = projectFileCount(project);
  const streamCount = projectStreamCount(project);

  return (
    <Card
      onClick={onOpen}
      className="cursor-pointer transition-all group"
      data-testid={`project-card-${project.id}`}
    >
      <CardContent className="pt-5 pb-5">
        {/* Header: name + language count */}
        <div className="flex items-start justify-between mb-3">
          <h3 className="font-semibold text-base leading-snug pr-2">{project.name}</h3>
          <div className="flex items-center gap-1 shrink-0">
            <Badge variant="secondary" className="text-[11px]">
              <Plural count={project.target_languages.length}>
                <One>{project.target_languages.length} lang</One>
                <Other>{project.target_languages.length} langs</Other>
              </Plural>
            </Badge>
            {(onRename || onArchive) && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    onClick={(e) => e.stopPropagation()}
                    className="p-1 rounded text-muted-foreground/50 hover:text-foreground hover:bg-accent/50 transition-colors cursor-pointer bg-transparent border-none opacity-0 group-hover:opacity-100"
                  >
                    <MoreHorizontal className="w-4 h-4" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-[150px]">
                  {onRename && (
                    <DropdownMenuItem
                      onClick={(e: React.MouseEvent) => {
                        e.stopPropagation();
                        onRename();
                      }}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Pencil className="w-3.5 h-3.5" /> Rename
                    </DropdownMenuItem>
                  )}
                  {onArchive && (
                    <>
                      {onRename && <DropdownMenuSeparator />}
                      <DropdownMenuItem
                        onClick={(e: React.MouseEvent) => {
                          e.stopPropagation();
                          onArchive();
                        }}
                        className="flex items-center gap-2 text-sm text-destructive"
                      >
                        <Trash2 className="w-3.5 h-3.5" /> Archive
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </div>

        {/* Locale mapping */}
        <div className="text-[13px] text-muted-foreground mb-4 flex items-center gap-1.5 flex-wrap">
          <span className="font-medium text-foreground/80">
            {getDisplayName(project.default_source_language)}
          </span>
          <ArrowRight className="w-3 h-3 shrink-0 opacity-50" />
          <span className="truncate">
            {project.target_languages.map((l) => getDisplayName(l)).join(", ")}
          </span>
        </div>

        {/* Stats row */}
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <FileText className="w-3 h-3" />
            <Plural count={fileCount}>
              <One>{fileCount} file</One>
              <Other>{fileCount} files</Other>
            </Plural>
          </span>
          <span className="flex items-center gap-1">
            <Globe className="w-3 h-3" />
            {compactNumber(wordCount)} words
          </span>
          {streamCount > 1 && (
            <span className="flex items-center gap-1">
              <Sparkles className="w-3 h-3" />
              {streamCount} streams
            </span>
          )}
        </div>

        {/* Last modified */}
        {project.modified_at && (
          <div className="flex items-center gap-1 mt-3 pt-3 border-t border-border/30 text-[11px] text-muted-foreground">
            <Clock className="w-3 h-3" />
            <span>Updated {relativeTime(project.modified_at)}</span>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// First run: one screen, three distinct ways in
// ---------------------------------------------------------------------------

function OnboardingView({
  workspaceName,
  serverUrl,
  pendingFileCount,
  onStartCreate,
  onDropFiles,
  onCreateSampleProject,
  onScanVoice,
  onInviteTeam,
}: {
  workspaceName?: string;
  serverUrl?: string;
  pendingFileCount: number;
  onStartCreate: () => void;
  onDropFiles: (files: File[]) => void;
  onCreateSampleProject?: () => void;
  onScanVoice?: () => void;
  onInviteTeam?: () => void;
}) {
  const [dragActive, setDragActive] = useState(false);

  const handleDrop = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      e.preventDefault();
      setDragActive(false);
      const files = Array.from(e.dataTransfer?.files ?? []);
      if (files.length > 0) onDropFiles(files);
    },
    [onDropFiles],
  );

  return (
    <div data-testid={TEST_IDS.onboarding.emptyProjects}>
      <PageHeader
        variant="hero"
        eyebrow={workspaceName}
        title="Set up your workspace"
        subtitle="Bowrain keeps translated content converging on your brand. Start with your AI assistant, with your files, or with your team."
      />

      {/* Hero card: assistant-driven starter pack */}
      <StarterPromptCard
        className="mb-5"
        workspaceName={workspaceName}
        serverUrl={serverUrl}
        footer={
          onScanVoice && (
            <p className="mt-4 text-sm text-muted-foreground">
              No assistant at hand?{" "}
              <button
                type="button"
                onClick={onScanVoice}
                data-testid="onboarding-scan-brand"
                className="cursor-pointer border-none bg-transparent p-0 font-medium text-foreground underline underline-offset-2 hover:text-primary"
              >
                Run the hosted context scan
              </button>{" "}
              instead.
            </p>
          )
        }
      />

      {/* Second row: project + team */}
      <div className="grid gap-5 sm:grid-cols-2">
        <Card className="flex min-h-[300px] flex-col">
          <CardContent className="flex flex-1 flex-col px-6 py-6">
            <div className="mb-3 flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10">
                <FolderOpen className="h-5 w-5 text-primary" />
              </div>
              <h3 className="text-base font-semibold">Create or import a project</h3>
            </div>
            <p className="mb-5 text-sm leading-relaxed text-muted-foreground">
              A project holds your files, languages, and translation state. Create one, then add
              JSON, XLIFF, PO, Office documents, or any other supported format.
            </p>
            <div
              data-testid="onboarding-dropzone"
              onDragOver={(e) => {
                e.preventDefault();
                setDragActive(true);
              }}
              onDragLeave={() => setDragActive(false)}
              onDrop={handleDrop}
              className={cn(
                "mb-5 rounded-lg border border-dashed px-4 py-5 text-center text-xs transition-colors",
                dragActive
                  ? "border-primary bg-primary/5 text-foreground"
                  : "border-border text-muted-foreground",
              )}
            >
              <Upload className="mx-auto mb-1.5 h-4 w-4" />
              {pendingFileCount > 0 ? (
                <Plural count={pendingFileCount}>
                  <One>
                    <span>
                      {pendingFileCount} file ready to import, added right after the project is
                      created
                    </span>
                  </One>
                  <Other>
                    <span>
                      {pendingFileCount} files ready to import, added right after the project is
                      created
                    </span>
                  </Other>
                </Plural>
              ) : (
                <span>Drop files here. They are imported right after the project is created</span>
              )}
            </div>
            <div className="mt-auto">
              <Button onClick={onStartCreate} data-testid="onboarding-create-btn">
                <Plus className="mr-1.5 h-4 w-4" />
                Create project
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="flex min-h-[300px] flex-col">
          <CardContent className="flex flex-1 flex-col px-6 py-6">
            <div className="mb-3 flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10">
                <UserPlus className="h-5 w-5 text-primary" />
              </div>
              <h3 className="text-base font-semibold">Invite your team</h3>
            </div>
            <p className="mb-5 text-sm leading-relaxed text-muted-foreground">
              Reviewers and translators join the same workspace: shared projects, one task queue,
              and the same brand context. Invites go out by email from the members page.
            </p>
            {onInviteTeam && (
              <div className="mt-auto">
                <Button
                  variant="outline"
                  onClick={onInviteTeam}
                  data-testid="onboarding-invite-btn"
                >
                  <UserPlus className="mr-1.5 h-4 w-4" />
                  Invite members
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Low-emphasis exit: sample project */}
      {onCreateSampleProject && (
        <div className="mt-8 flex justify-center">
          <button
            type="button"
            onClick={onCreateSampleProject}
            className="flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
          >
            <Sparkles className="w-3 h-3" />
            Or try a sample project to explore Bowrain
          </button>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function ProjectDashboard({
  projects,
  onCreateProject,
  onOpenProject,
  onCreateSampleProject,
  workspaceName,
  onEditProject,
  onArchiveProject,
  workspaceLanguages,
  onScanVoice,
  onInviteTeam,
  serverUrl,
  loopStatus,
  onOpenActivities,
  onOpenRuns,
  onOpenTasks,
  onOpenReview,
  onOpenDelivery,
  onOpenVoiceDashboard,
}: ProjectDashboardProps) {
  const { getDisplayName } = useLocales();
  const [showCreate, setShowCreate] = useState(false);
  const [editingProject, setEditingProject] = useState<ProjectInfo | null>(null);
  // Files dropped on the first-run create card, handed to onCreateProject so
  // the caller can run the existing upload-after-create flow.
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);

  const handleDropFiles = useCallback((files: File[]) => {
    setPendingFiles(files);
    setShowCreate(true);
  }, []);

  const isEmpty = projects.length === 0;

  return (
    <div className="mx-auto w-full max-w-6xl px-2 py-8 md:py-10">
      {isEmpty ? (
        <OnboardingView
          workspaceName={workspaceName}
          serverUrl={serverUrl}
          pendingFileCount={pendingFiles.length}
          onStartCreate={() => setShowCreate(true)}
          onDropFiles={handleDropFiles}
          onCreateSampleProject={onCreateSampleProject}
          onScanVoice={onScanVoice}
          onInviteTeam={onInviteTeam}
        />
      ) : (
        <div>
          <PageHeader
            className="mb-8"
            title={workspaceName || "Projects"}
            subtitle={
              <Plural count={projects.length}>
                <One>{projects.length} project in this workspace</One>
                <Other>{projects.length} projects in this workspace</Other>
              </Plural>
            }
            actions={
              <Button onClick={() => setShowCreate(true)} data-testid="new-project-btn">
                <Plus className="w-4 h-4 mr-1.5" />
                New Project
              </Button>
            }
          />

          {/* Loop status: where the loop stands, above the inventory */}
          {loopStatus && (
            <section aria-label="Loop status" className="mb-8">
              <LoopStatusRow
                status={loopStatus}
                onOpenActivities={onOpenActivities}
                onOpenRuns={onOpenRuns}
                onOpenTasks={onOpenTasks}
                onOpenReview={onOpenReview}
                onOpenDelivery={onOpenDelivery}
                onOpenVoiceDashboard={onOpenVoiceDashboard}
              />
            </section>
          )}

          {/* Stats */}
          <DashboardStats projects={projects} />

          {/* Project grid (hard-capped so huge workspaces never flood the DOM) */}
          <div className="grid grid-cols-[repeat(auto-fill,minmax(min(320px,100%),1fr))] gap-5">
            {projects.slice(0, MAX_PROJECT_CARDS).map((p) => (
              <ProjectCard
                key={p.id}
                project={p}
                onOpen={() => onOpenProject(p)}
                onRename={onEditProject ? () => setEditingProject(p) : undefined}
                onArchive={onArchiveProject ? () => onArchiveProject(p.id) : undefined}
                getDisplayName={getDisplayName}
              />
            ))}
          </div>
          <ListCapRow
            shown={Math.min(projects.length, MAX_PROJECT_CARDS)}
            total={projects.length}
            noun="projects"
            hint="Archive finished projects to shorten this list."
            className="mt-4 border border-border rounded-md"
          />
        </div>
      )}

      {/* Create project dialog */}
      <ProjectFormDialog
        open={showCreate}
        onOpenChange={(v) => {
          setShowCreate(v);
          if (!v) setPendingFiles([]);
        }}
        workspaceLanguages={workspaceLanguages}
        onSubmit={(data: ProjectFormData) => {
          onCreateProject(
            data.name,
            data.default_source_language,
            data.target_languages,
            pendingFiles.length > 0 ? pendingFiles : undefined,
          );
          setPendingFiles([]);
          setShowCreate(false);
        }}
      />

      {/* Edit project dialog */}
      {onEditProject && (
        <ProjectFormDialog
          open={editingProject !== null}
          onOpenChange={(v) => {
            if (!v) setEditingProject(null);
          }}
          editProject={editingProject ?? undefined}
          workspaceLanguages={workspaceLanguages}
          onSubmit={(data: ProjectFormData) => {
            if (editingProject) {
              onEditProject(editingProject.id, {
                name: data.name,
                target_languages: data.target_languages,
              });
              setEditingProject(null);
            }
          }}
        />
      )}
    </div>
  );
}
