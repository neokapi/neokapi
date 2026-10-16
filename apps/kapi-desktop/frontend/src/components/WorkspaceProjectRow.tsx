import { FolderKanban, FolderOpen, FolderX, Library } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Badge, Button, SimpleTooltip, When } from "@neokapi/ui-primitives";
import { useShortenHome } from "../hooks/useShortenHome";
import { formatLook } from "./ContextDigest";
import type { ContextNews, ProjectSharing, WorkspaceProject } from "../types/api";

interface WorkspaceProjectRowProps {
  project: WorkspaceProject;
  /** What the project's digest holds that the person has not seen. Absent shows nothing. */
  news?: ContextNews;
  /** Open the project from one of its checkouts, by recipe path. */
  onOpenCheckout: (recipe: string) => void;
  /** Open a project no checkout here carries, on its context alone. */
  onOpenContext: (key: string) => void;
  /** Ask to remove the project from the workspace. */
  onRemove: (project: WorkspaceProject) => void;
}

/**
 * One project on the workspace home.
 *
 * A project is one row however many copies of it are on this machine, because
 * the checkouts share an identity and one context. With a single checkout the
 * row opens it; with several the row lists them and the reader picks. With
 * none, the project still opens, on the context the workspace holds for it.
 */
export function WorkspaceProjectRow({
  project,
  news,
  onOpenCheckout,
  onOpenContext,
  onRemove,
}: WorkspaceProjectRowProps) {
  const shortenHome = useShortenHome();
  const live = project.checkouts.filter((c) => !c.missing);
  const single = live.length === 1 && project.checkouts.length === 1;

  return (
    <div
      data-testid="workspace-project"
      data-project={project.key}
      className="rounded-lg border border-border/60 p-3"
    >
      <div className="flex items-start gap-3">
        <FolderKanban size={16} className="mt-0.5 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className="truncate text-sm font-medium">{project.name}</span>
            <NewsBadge news={news} />
          </div>
          <ProjectFacts project={project} />
          <div className="truncate font-mono text-[11px] text-muted-foreground/70">
            {project.key}
          </div>
          {project.context_files && (
            <p
              data-testid="workspace-context-files"
              className="mt-1 text-[11px] text-amber-700 dark:text-amber-500"
            >
              {t("Context files in the checkout, unread. Run")}{" "}
              <code className="font-mono" translate="no">
                {project.context_files.command}
              </code>
              .
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {single && (
            <Button size="sm" variant="outline" onClick={() => onOpenCheckout(live[0].recipe)}>
              Open
            </Button>
          )}
          {project.checkouts.length === 0 && (
            <SimpleTooltip content="Read its terms, voice and content memory without a copy of the files">
              <Button size="sm" variant="outline" onClick={() => onOpenContext(project.key)}>
                <Library size={14} />
                Open context
              </Button>
            </SimpleTooltip>
          )}
          <Button
            size="sm"
            variant="ghost"
            className="text-muted-foreground"
            onClick={() => onRemove(project)}
            aria-label={t("Remove {name} from your workspace", { name: project.name })}
          >
            Remove
          </Button>
        </div>
      </div>

      {project.checkouts.length > 0 && (
        <ul className="mt-2 space-y-1 pl-7">
          {project.checkouts.map((checkout) => (
            <li key={checkout.path}>
              {checkout.missing ? (
                <div
                  data-testid="checkout-missing"
                  className="flex items-center gap-2 text-xs text-muted-foreground/70"
                >
                  <FolderX size={13} className="shrink-0" />
                  <span className="truncate font-mono">{shortenHome(checkout.path)}</span>
                  <Badge variant="outline" className="shrink-0 text-muted-foreground">
                    Missing
                  </Badge>
                </div>
              ) : single ? (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <FolderOpen size={13} className="shrink-0" />
                  <span className="truncate font-mono">{shortenHome(checkout.path)}</span>
                </div>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  data-testid="checkout-open"
                  onClick={() => onOpenCheckout(checkout.recipe)}
                  className="h-auto w-full justify-start gap-2 px-1 py-1 text-xs font-normal text-muted-foreground hover:bg-accent/30"
                >
                  <FolderOpen size={13} className="shrink-0" />
                  <span className="truncate font-mono">{shortenHome(checkout.path)}</span>
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}

      {project.checkouts.length > 0 && live.length === 0 && (
        <div className="mt-2 pl-7">
          <Button size="sm" variant="outline" onClick={() => onOpenContext(project.key)}>
            <Library size={14} />
            Open context
          </Button>
        </div>
      )}
    </div>
  );
}

/** "4 new since Tuesday": suggestions and rules the person has not seen. */
function NewsBadge({ news }: { news?: ContextNews }) {
  if (!news || news.new <= 0) return null;
  return (
    <Badge variant="secondary" data-slot="context-news">
      {news.since
        ? t("{count} new since {when}", { count: news.new, when: formatLook(news.since) })
        : t("{count} new", { count: news.new })}
    </Badge>
  );
}

/** "voice Fernwell · 23 rules · on this Mac · 2 h ago": the project in one line. */
function ProjectFacts({ project }: { project: WorkspaceProject }) {
  const parts: React.ReactNode[] = [
    project.voice ? (
      <span key="voice">{t("voice {voice}", { voice: project.voice })}</span>
    ) : (
      <span key="voice" className="text-muted-foreground/60">
        no voice bound
      </span>
    ),
  ];
  if (project.context) {
    parts.push(
      <span key="rules">
        {t("{rules, plural, one {# rule} other {# rules}}", { rules: project.context.rules })}
      </span>,
    );
  }
  const reach = sharingWord(project.sharing);
  if (reach) parts.push(<span key="sharing">{reach}</span>);
  if (project.last_active) {
    parts.push(<When key="active" iso={project.last_active} relative />);
  }
  return (
    <p
      data-slot="workspace-project-facts"
      className="mt-0.5 flex flex-wrap items-baseline gap-x-1 text-[11px] text-muted-foreground"
    >
      {parts.map((part, i) => (
        <span key={i} className="inline-flex items-baseline gap-x-1">
          {i > 0 && <span aria-hidden>·</span>}
          {part}
        </span>
      ))}
    </p>
  );
}

/** How far the project's context reaches, in the one word the row has room for. */
export function sharingWord(sharing?: ProjectSharing): string {
  if (!sharing) return "";
  if (sharing.venue_host) return t("Bowrain {host}", { host: sharing.venue_host });
  switch (sharing.kind) {
    case "git":
      return t("shared through git");
    case "file":
      return t("shared through a folder");
    case "s3":
      return t("shared through S3");
    default:
      return t("on this Mac");
  }
}
