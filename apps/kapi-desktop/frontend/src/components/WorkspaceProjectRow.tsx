import {
  Cloud,
  Copy,
  Database,
  Ellipsis,
  FileWarning,
  Folder,
  FolderKanban,
  FolderOpen,
  FolderX,
  GitBranch,
  Laptop,
  Library,
  Terminal,
  Trash2,
  type LucideIcon,
} from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Badge,
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  SimpleTooltip,
  When,
} from "@neokapi/ui-primitives";
import { useShortenHome } from "../hooks/useShortenHome";
import { writeClipboardText } from "../lib/clipboard";
import type {
  ContextFilesNotice,
  ProjectSharing,
  WorkspaceCheckout,
  WorkspaceProject,
} from "../types/api";

interface WorkspaceProjectRowProps {
  project: WorkspaceProject;
  /** Open the project from one of its checkouts, by recipe path. */
  onOpenCheckout: (recipe: string) => void;
  /** Open a project no checkout here carries, on its context alone. */
  onOpenContext: (key: string) => void;
  /** Ask to remove the project from the workspace. */
  onRemove: (project: WorkspaceProject) => void;
}

/**
 * The columns every project row shares, so the rows of the list read as one
 * table: icon, name, voice, rules, reach, last activity, actions. A narrow pane
 * keeps the name and the actions.
 */
const COLUMNS =
  "grid-cols-[16px_minmax(0,1fr)_auto] @2xl:grid-cols-[16px_minmax(0,1fr)_8rem_4.5rem_6rem_7rem_8.5rem]";

/**
 * One project on the workspace home, as one row of the projects list.
 *
 * A project is one row however many copies of it are on this machine, because
 * the checkouts share an identity and one context. Open opens the first
 * checkout that is still there; the others are in the row's menu. With none,
 * the project still opens, on the context the workspace holds for it.
 */
export function WorkspaceProjectRow({
  project,
  onOpenCheckout,
  onOpenContext,
  onRemove,
}: WorkspaceProjectRowProps) {
  const shortenHome = useShortenHome();
  const primary = project.checkouts.find((c) => !c.missing);
  const others = project.checkouts.filter((c) => c !== primary);
  // The checkout the name stands for: the one Open opens, or the first one
  // seen when none is left.
  const named = primary ?? project.checkouts[0];
  const reach = projectReach(project.sharing);

  return (
    <div data-testid="workspace-project" data-project={project.key} className="@container">
      <div className={`grid h-11 items-center gap-x-3 px-2 ${COLUMNS}`}>
        <FolderKanban aria-hidden size={16} className="text-muted-foreground" />

        <div className="flex min-w-0 items-center gap-2">
          <SimpleTooltip
            content={named && <span className="font-mono">{shortenHome(named.path)}</span>}
          >
            <span
              data-slot="workspace-project-name"
              className="truncate text-sm"
              translate="no"
              tabIndex={named ? 0 : undefined}
            >
              {project.name}
            </span>
          </SimpleTooltip>
          {project.context_files && <ContextFilesBadge notice={project.context_files} />}
        </div>

        <div
          data-slot="workspace-project-facts"
          className="hidden text-[11px] text-muted-foreground @2xl:contents"
        >
          <span data-slot="workspace-project-voice" className="truncate" translate="no">
            {project.voice ?? <span className="text-muted-foreground/60">no voice</span>}
          </span>
          <span data-slot="workspace-project-rules" className="tabular-nums">
            {project.context &&
              t("{rules, plural, one {# rule} other {# rules}}", { rules: project.context.rules })}
          </span>
          <span data-slot="workspace-project-reach" className="flex min-w-0 items-center">
            {reach && (
              <SimpleTooltip content={reachDetail(project.sharing, shortenHome)}>
                <span className="inline-flex max-w-full items-center gap-1" tabIndex={0}>
                  <reach.icon aria-hidden size={12} className="shrink-0" />
                  <span className="truncate">{reach.word}</span>
                </span>
              </SimpleTooltip>
            )}
          </span>
          <span data-slot="workspace-project-active" className="truncate">
            {project.last_active && <When iso={project.last_active} relative />}
          </span>
        </div>

        <div className="flex items-center justify-end gap-1">
          {primary ? (
            <Button size="sm" variant="outline" onClick={() => onOpenCheckout(primary.recipe)}>
              Open
            </Button>
          ) : (
            <SimpleTooltip content="Read its terms, voice and content memory without a copy of the files">
              <Button size="sm" variant="outline" onClick={() => onOpenContext(project.key)}>
                <Library size={14} />
                Open context
              </Button>
            </SimpleTooltip>
          )}
          <ProjectMenu
            project={project}
            others={others}
            path={named?.path}
            shortenHome={shortenHome}
            onOpenCheckout={onOpenCheckout}
            onRemove={onRemove}
          />
        </div>
      </div>
    </div>
  );
}

/** The row's `⋯` menu: the other checkouts, Copy path, and Remove. */
function ProjectMenu({
  project,
  others,
  path,
  shortenHome,
  onOpenCheckout,
  onRemove,
}: {
  project: WorkspaceProject;
  others: WorkspaceCheckout[];
  path?: string;
  shortenHome: (path: string) => string;
  onOpenCheckout: (recipe: string) => void;
  onRemove: (project: WorkspaceProject) => void;
}) {
  const command = project.context_files?.command;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="icon-sm"
          variant="ghost"
          data-slot="workspace-project-menu"
          className="text-muted-foreground"
          aria-label={t("More for {name}", "the menu on a project's row", { name: project.name })}
        >
          <Ellipsis size={14} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        {others.length > 0 && (
          <>
            <DropdownMenuLabel className="text-[11px] font-normal text-muted-foreground">
              {t("Other checkouts")}
            </DropdownMenuLabel>
            {others.map((checkout) =>
              checkout.missing ? (
                <DropdownMenuItem key={checkout.path} disabled data-testid="checkout-missing">
                  <FolderX />
                  <CheckoutPath path={checkout.path} shortenHome={shortenHome} />
                  <Badge variant="outline" className="text-[11px] font-normal">
                    {t("Missing")}
                  </Badge>
                </DropdownMenuItem>
              ) : (
                <DropdownMenuItem
                  key={checkout.path}
                  data-testid="checkout-open"
                  onSelect={() => onOpenCheckout(checkout.recipe)}
                >
                  <FolderOpen />
                  <CheckoutPath path={checkout.path} shortenHome={shortenHome} />
                </DropdownMenuItem>
              ),
            )}
            <DropdownMenuSeparator />
          </>
        )}
        {path && (
          <DropdownMenuItem onSelect={() => void writeClipboardText(path).catch(() => {})}>
            <Copy />
            {t("Copy path")}
          </DropdownMenuItem>
        )}
        {command && (
          <DropdownMenuItem onSelect={() => void writeClipboardText(command).catch(() => {})}>
            <Terminal />
            {t("Copy the command that reads the context files")}
          </DropdownMenuItem>
        )}
        {(path || command) && <DropdownMenuSeparator />}
        <DropdownMenuItem variant="destructive" onSelect={() => onRemove(project)}>
          <Trash2 />
          {t("Remove")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * A checkout in the menu: the folder's own name, which is what tells two
 * worktrees apart, over the folder it sits in.
 */
function CheckoutPath({
  path,
  shortenHome,
}: {
  path: string;
  shortenHome: (path: string) => string;
}) {
  return (
    <span className="flex min-w-0 flex-1 flex-col" translate="no">
      <span className="truncate font-mono text-xs">{lastSegment(path)}</span>
      <span className="truncate font-mono text-[11px] text-muted-foreground">
        {shortenHome(parentOf(path))}
      </span>
    </span>
  );
}

/** The last segment of a path, on either separator. */
export function lastSegment(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean);
  return parts[parts.length - 1] ?? path;
}

/** The path without its last segment. */
export function parentOf(path: string): string {
  const trimmed = path.replace(/[\\/]+$/, "");
  const at = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return at > 0 ? trimmed.slice(0, at) : trimmed.slice(0, at + 1);
}

/** Context files in a checkout that the project's store has never read. */
function ContextFilesBadge({ notice }: { notice: ContextFilesNotice }) {
  return (
    <SimpleTooltip
      content={
        <span>
          {t("Context files in the checkout, unread. Run")}{" "}
          <code className="font-mono" translate="no">
            {notice.command}
          </code>
          .
        </span>
      }
    >
      <Badge
        variant="outline"
        data-testid="workspace-context-files"
        tabIndex={0}
        className="border-warning/60 text-[11px] font-normal text-[color-mix(in_oklch,var(--warning)_60%,var(--warning-foreground))] dark:text-warning"
      >
        <FileWarning aria-hidden />
        {t("Unread files")}
      </Badge>
    </SimpleTooltip>
  );
}

/** How far a project's context reaches, as an icon and the one word a column has room for. */
export interface ProjectReach {
  icon: LucideIcon;
  word: string;
}

/** The icon and word for where a project's context is kept. Null when unknown. */
export function projectReach(sharing?: ProjectSharing): ProjectReach | null {
  if (!sharing) return null;
  if (sharing.venue_host) return { icon: Cloud, word: "Bowrain" };
  switch (sharing.kind) {
    case "git":
      return { icon: GitBranch, word: "git" };
    case "file":
      return { icon: Folder, word: t("folder", "where a project's context is shared") };
    case "s3":
      return { icon: Database, word: "S3" };
    default:
      return { icon: Laptop, word: t("this Mac", "where a project's context is kept") };
  }
}

/** The word alone, for a reader that has no room for the icon. Empty when unknown. */
export function sharingWord(sharing?: ProjectSharing): string {
  return projectReach(sharing)?.word ?? "";
}

/** The reach spelt out, with where the shared copy is: the reach column's tooltip. */
export function reachDetail(
  sharing: ProjectSharing | undefined,
  shortenHome: (path: string) => string = (p) => p,
): string {
  if (!sharing) return "";
  if (sharing.venue_host) return t("Connected to Bowrain at {host}", { host: sharing.venue_host });
  const location = sharing.location ? shortenHome(sharing.location) : "";
  switch (sharing.kind) {
    case "git":
      return location
        ? t("Shared through git, at {location}", { location })
        : t("Shared through git");
    case "file":
      return location
        ? t("Shared through a folder, at {location}", { location })
        : t("Shared through a folder");
    case "s3":
      return location
        ? t("Shared through S3, at {location}", { location })
        : t("Shared through S3");
    default:
      return t("On this Mac");
  }
}
