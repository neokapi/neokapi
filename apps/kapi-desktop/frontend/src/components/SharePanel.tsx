// Share: how far one project's rules reach, as three rungs in one list.
// On this Mac, the workspace every clone and worktree here reads. With your
// team, the place kapi.yaml names under `context:`, which every machine that
// syncs the project reads. Managed on Bowrain, the server a recipe's
// `bowrain:` block connects to.
//
// The backend's ShareView names the rung the way `kapi share --status` does,
// and the list marks it. The facts and actions sit under the rung they
// belong to: the workspace under this Mac's while it is current, the place,
// its counts, Sync now and Change under the team rung once the recipe names a
// place, and the server under Bowrain's once the recipe connects to one.
// Sync now is the sync `kapi share` runs. Its result, or its error word for
// word, stays under the rung until the next sync. Change opens the dialog
// that writes the `context:` block, and the recipe with it in a folder that
// has none.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ProjectScreensPort } from "@neokapi/project-screens";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Cloud,
  ExternalLink,
  Eye,
  Folder,
  GitBranch,
  Laptop,
  Server,
  Share2,
  TriangleAlert,
  Users,
} from "lucide-react";
import {
  Badge,
  Button,
  ErrorNotice,
  LoadingSpinner,
  PageHeader,
  SimpleTooltip,
  When,
  parseAppError,
} from "@neokapi/ui-primitives";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useShortenHome } from "../hooks/useShortenHome";
import { ShareSetupDialog } from "./ShareSetupDialog";
import { BowrainPreviewSheet } from "./BowrainPreviewSheet";
import { useLocalPreviewPort } from "../lib/previewPort";
import { api } from "../hooks/useApi";
import type {
  ContextSyncResult,
  ProjectSharing,
  ShareRungID,
  ShareView,
  SyncStatus,
} from "../types/api";

/** Where the About Bowrain action leads. */
export const BOWRAIN_ABOUT_URL = "https://bowrain.cloud/docs/getting-started/kapi-vs-bowrain";

/** The command that connects a project to a Bowrain server. */
export const BOWRAIN_SHARE_COMMAND = "kapi share --via bowrain <server-url>";

/** The rungs of the ladder, nearest first. */
export const RUNGS: ShareRungID[] = ["machine", "team", "bowrain"];

/** True when kapi.yaml names a place to share the context through. */
export function sharingSetUp(sharing: ProjectSharing): boolean {
  return !!sharing.kind && sharing.kind !== "local";
}

/**
 * The rungs a project has reached. This Mac always is; the team rung once the
 * recipe names a place; Bowrain's once the recipe connects to a server. A
 * project connected to Bowrain without a team place has not reached the team
 * rung, and a context kept on Bowrain is the top rung, not a team place.
 */
export function reachedRungs(view: ShareView): Record<ShareRungID, boolean> {
  return {
    machine: true,
    team: sharingSetUp(view.sharing) && view.sharing.kind !== "bowrain",
    bowrain: view.rung === "bowrain",
  };
}

/** The last `n` segments of a path, so `…/kapi/workspaces/default` reads `workspaces/default`. */
export function lastSegments(path: string, n: number): string {
  return path
    .split(/[\\/]+/)
    .filter(Boolean)
    .slice(-n)
    .join("/");
}

/** True when a sync wrote or removed any rules file. */
export function syncWroteRulesFiles(result: ContextSyncResult): boolean {
  return (result.rules_files?.files ?? []).some((f) => f.action !== "unchanged");
}

export interface SharePanelProps {
  tabID: string;
  /** The open project's name, which leads the page title. */
  projectName?: string;
  /** A pre-loaded page for Storybook and tests, which reach no backend. */
  view?: ShareView;
  /**
   * A tab over a project's context alone, with no checkout here. Sharing is
   * named in kapi.yaml, so no rung can be set up from it.
   */
  contextOnly?: boolean;
  /** Called once Change has saved a place to kapi.yaml. */
  onSetUp?: (sharing: ProjectSharing) => void;
  /** What Preview reads through, for Storybook and tests; the tab's own project otherwise. */
  previewPort?: ProjectScreensPort;
}

export function SharePanel({
  tabID,
  projectName,
  view: given,
  contextOnly,
  onSetUp,
  previewPort: givenPort,
}: SharePanelProps) {
  const qc = useQueryClient();
  const [settingUp, setSettingUp] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const localPort = useLocalPreviewPort(tabID);
  const previewPort = givenPort ?? localPort;
  const [result, setResult] = useState<ContextSyncResult | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);

  const key = qk.shareView(tabID);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.shareView(tabID),
    enabled: !given,
    placeholderData: (previous) => previous,
  });
  // A sync from a terminal or another machine moves the counts.
  useInvalidateOnEvent("workspace:changed", [key]);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: key });
    void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
  };

  const sync = useMutation({
    mutationFn: () => api.syncProjectContext(tabID),
    onMutate: () => {
      setResult(null);
      setSyncError(null);
    },
    onSuccess: (r) => setResult(r ?? null),
    onError: (e) => setSyncError(parseAppError(e).title),
    onSettled: refresh,
  });

  const view = given ?? query.data ?? null;
  if (query.error && !given) {
    return (
      <ErrorNotice
        error={query.error}
        title="How far this project's rules reach could not be read"
      />
    );
  }
  if (!view) return query.isLoading ? <LoadingSpinner /> : null;

  const reached = reachedRungs(view);
  const change = () => setSettingUp(true);

  return (
    <div data-slot="share-panel">
      <PageHeader
        className="mb-5"
        title={projectName ? t("{project} · Share", { project: projectName }) : t("Share")}
        subtitle="How far your rules reach"
      />
      <ol
        data-slot="share-ladder"
        aria-label={t("How far your rules reach")}
        className="divide-y divide-border rounded-lg border border-border"
      >
        {RUNGS.map((rung) => (
          <RungRow
            key={rung}
            rung={rung}
            view={view}
            current={view.rung === rung}
            reached={reached[rung]}
          >
            {rung === "machine" && view.rung === "machine" && (
              <MachineDetail view={view} contextOnly={contextOnly} onChange={change} />
            )}
            {rung === "team" && reached.team && (
              <TeamDetail
                sharing={view.sharing}
                onChange={change}
                onSync={() => sync.mutate()}
                syncing={sync.isPending}
                result={result}
                syncError={syncError}
              />
            )}
            {rung === "team" && !reached.team && view.rung === "bowrain" && (
              <Actions>
                <Button size="sm" variant="outline" data-slot="share-change" onClick={change}>
                  Change
                </Button>
              </Actions>
            )}
            {rung === "bowrain" && (
              <BowrainDetail
                view={view}
                contextOnly={contextOnly}
                onPreview={contextOnly ? undefined : () => setPreviewing(true)}
              />
            )}
            {rung === "bowrain" && view.sharing.kind === "bowrain" && (
              <TeamDetail
                sharing={view.sharing}
                onChange={change}
                onSync={() => sync.mutate()}
                syncing={sync.isPending}
                result={result}
                syncError={syncError}
              />
            )}
          </RungRow>
        ))}
      </ol>

      {!contextOnly && (
        <BowrainPreviewSheet
          open={previewing}
          onOpenChange={setPreviewing}
          projectName={projectName || t("This project")}
          port={previewPort}
        />
      )}

      {!contextOnly && (
        <ShareSetupDialog
          tabID={tabID}
          open={settingUp}
          onClose={() => setSettingUp(false)}
          onSaved={(s) => {
            refresh();
            onSetUp?.(s);
          }}
          onSync={() => sync.mutate()}
        />
      )}
    </div>
  );
}

/** One rung: its mark, name and reach, the Current badge, and what sits under it. */
function RungRow({
  rung,
  view,
  current,
  reached,
  children,
}: {
  rung: ShareRungID;
  view: ShareView;
  current: boolean;
  reached: boolean;
  children?: React.ReactNode;
}) {
  return (
    <li
      data-slot="share-rung"
      data-rung={rung}
      data-current={current || undefined}
      data-reached={reached || undefined}
      aria-current={current ? "step" : undefined}
      className="px-4 py-3"
    >
      <div className="flex min-h-6 items-center gap-3">
        <RungIcon rung={rung} reached={reached} />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">
            <RungName rung={rung} />
          </p>
          <p data-slot="share-reach" className="text-[11px] text-muted-foreground">
            <RungReach rung={rung} view={view} />
          </p>
        </div>
        {current && (
          <Badge variant="secondary" data-slot="share-current" className="shrink-0">
            Current
          </Badge>
        )}
      </div>
      {children}
    </li>
  );
}

/** A rung's name, written here so it is translated. */
function RungName({ rung }: { rung: ShareRungID }) {
  switch (rung) {
    case "machine":
      return <>On this Mac</>;
    case "team":
      return <>With your team</>;
    case "bowrain":
      return <>Managed on Bowrain</>;
  }
}

/**
 * What reaching a rung changes for the person. A team rung the project has
 * reached says instead how many rules travel and when this Mac last synced.
 */
function RungReach({ rung, view }: { rung: ShareRungID; view: ShareView }) {
  switch (rung) {
    case "machine":
      return <>Your rules, your assistants</>;
    case "team":
      return reachedRungs(view).team ? <TeamStanding view={view} /> : <>A shared folder or git</>;
    case "bowrain":
      return <>A team queue, who kept what, the same screens in a browser</>;
  }
}

/** "12 rules · synced 10:03", for a project shared with the team. */
function TeamStanding({ view }: { view: ShareView }) {
  const contacted = view.sharing.status?.contacted;
  return (
    <span data-slot="share-team-standing">
      {view.rules !== undefined && (
        <span className="tabular-nums">
          {t("{count, plural, one {# rule} other {# rules}}", { count: view.rules })}
        </span>
      )}
      {view.rules !== undefined && " · "}
      {contacted ? (
        <span data-slot="share-last-sync">
          synced <ContactTime iso={contacted} />
        </span>
      ) : (
        <span data-slot="share-last-sync">not synced from this Mac yet</span>
      )}
    </span>
  );
}

function RungIcon({ rung, reached }: { rung: ShareRungID; reached: boolean }) {
  const props = {
    size: 15,
    "aria-hidden": true,
    className: reached ? "shrink-0 text-foreground" : "shrink-0 text-muted-foreground",
  };
  switch (rung) {
    case "machine":
      return <Laptop {...props} />;
    case "team":
      return <Users {...props} />;
    case "bowrain":
      return <Server {...props} />;
  }
}

/** The facts under a rung, one labelled row each. */
function Facts({ children }: { children: React.ReactNode }) {
  return (
    <dl className="mt-3 ml-7 grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-1.5">
      {children}
    </dl>
  );
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 items-center gap-1.5 text-sm">{children}</dd>
    </>
  );
}

/** A path, place, host or command: a value of the project's own, in the mono font. */
function MonoChip({ slot, children }: { slot: string; children: React.ReactNode }) {
  return (
    <Badge
      variant="outline"
      data-slot={slot}
      translate="no"
      className="max-w-full truncate font-mono text-[11px] font-normal text-muted-foreground"
    >
      {children}
    </Badge>
  );
}

/** The actions under a rung, in one row. */
function Actions({ children }: { children: React.ReactNode }) {
  return <div className="mt-3 ml-7 flex flex-wrap items-center gap-2">{children}</div>;
}

/** Why a context-only tab offers no change. */
function NeedsCheckout() {
  return (
    <p data-slot="share-needs-checkout" className="mt-3 ml-7 text-[11px] text-muted-foreground">
      Sharing is named in the project&apos;s <span translate="no">kapi.yaml</span>, so it needs a
      checkout of this project on this Mac.
    </p>
  );
}

function MachineDetail({
  view,
  contextOnly,
  onChange,
}: {
  view: ShareView;
  contextOnly?: boolean;
  onChange: () => void;
}) {
  const shorten = useShortenHome();
  const workspace = view.sharing.workspace;
  return (
    <>
      <Facts>
        <Fact label={t("Workspace")}>
          <SimpleTooltip content={<span translate="no">{shorten(workspace)}</span>}>
            <span
              tabIndex={0}
              className="inline-flex min-w-0 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            >
              <MonoChip slot="share-workspace">{lastSegments(workspace, 2)}</MonoChip>
            </span>
          </SimpleTooltip>
        </Fact>
      </Facts>
      {view.error && (
        <div className="mt-3 ml-7">
          <p className="flex items-center gap-1.5 text-sm text-warning-ink">
            <TriangleAlert size={14} aria-hidden className="shrink-0" />
            <span>The place kapi.yaml names could not be read.</span>
          </p>
          <p
            data-slot="share-view-error"
            className="mt-1 font-mono text-[11px] text-muted-foreground"
            translate="no"
          >
            {view.error}
          </p>
        </div>
      )}
      {contextOnly ? (
        <NeedsCheckout />
      ) : (
        <Actions>
          <Button size="sm" data-slot="share-change" onClick={onChange}>
            Change
          </Button>
        </Actions>
      )}
    </>
  );
}

function TeamDetail({
  sharing,
  onChange,
  onSync,
  syncing,
  result,
  syncError,
}: {
  sharing: ProjectSharing;
  onChange: () => void;
  onSync: () => void;
  syncing: boolean;
  result: ContextSyncResult | null;
  syncError: string | null;
}) {
  const status = sharing.status;
  return (
    <>
      <Facts>
        <Fact label={t("Through")}>
          <PlaceKind kind={sharing.kind} />
          {sharing.location && <MonoChip slot="share-place">{sharing.location}</MonoChip>}
        </Fact>
        <Standing status={status} />
      </Facts>
      {status?.error && (
        <div className="mt-3 ml-7">
          <p className="flex items-center gap-1.5 text-sm text-warning-ink">
            <TriangleAlert size={14} aria-hidden className="shrink-0" />
            <span>The place could not be reached.</span>
          </p>
          <p
            data-slot="share-status-error"
            className="mt-1 font-mono text-[11px] text-muted-foreground"
            translate="no"
          >
            {status.error}
          </p>
        </div>
      )}
      <Actions>
        <Button size="sm" data-slot="share-sync" disabled={syncing} onClick={onSync}>
          Sync now
        </Button>
        <Button size="sm" variant="ghost" data-slot="share-change" onClick={onChange}>
          Change
        </Button>
        {syncing && <LoadingSpinner size="sm" />}
      </Actions>
      {result && <SyncResultLine result={result} />}
      {syncError && (
        <p
          data-slot="share-sync-error"
          role="alert"
          className="mt-2 ml-7 font-mono text-xs text-destructive"
          translate="no"
        >
          {syncError}
        </p>
      )}
    </>
  );
}

/** The kind of place, with its mark. */
function PlaceKind({ kind }: { kind: string }) {
  const icon = { size: 14, "aria-hidden": true, className: "shrink-0 text-muted-foreground" };
  switch (kind) {
    case "git":
      return (
        <span data-slot="share-kind" className="inline-flex items-center gap-1.5">
          <GitBranch {...icon} />
          <span translate="no">git</span>
        </span>
      );
    case "file":
      return (
        <span data-slot="share-kind" className="inline-flex items-center gap-1.5">
          <Folder {...icon} />
          {t("A shared folder")}
        </span>
      );
    case "s3":
      return (
        <span data-slot="share-kind" className="inline-flex items-center gap-1.5">
          <Cloud {...icon} />
          <span translate="no">S3</span>
        </span>
      );
    case "bowrain":
      return (
        <span data-slot="share-kind" className="inline-flex items-center gap-1.5">
          <Cloud {...icon} />
          {t("Managed on Bowrain")}
        </span>
      );
  }
  return (
    <span data-slot="share-kind" className="inline-flex items-center gap-1.5">
      <Share2 {...icon} />
      <span translate="no">{kind}</span>
    </span>
  );
}

/**
 * How far apart this Mac and the place were at the last sync. When that was
 * is on the rung's own line above.
 */
function Standing({ status }: { status?: SyncStatus }) {
  const contacted = status?.contacted;
  return (
    <>
      <Fact label={t("To push")}>
        <span data-slot="share-to-push" className="tabular-nums">
          {status?.to_push ?? 0}
        </span>
      </Fact>
      <Fact label={t("To pull")}>
        <span data-slot="share-to-pull" className="tabular-nums">
          {contacted ? status?.to_pull : <>Known after the first sync</>}
        </span>
      </Fact>
    </>
  );
}

/** The time of the last contact; the date too when it was not today. */
function ContactTime({ iso }: { iso: string }) {
  const d = new Date(iso);
  const today = new Date();
  const sameDay =
    d.getFullYear() === today.getFullYear() &&
    d.getMonth() === today.getMonth() &&
    d.getDate() === today.getDate();
  return <When iso={iso} dateStyle={sameDay ? "none" : "medium"} timeStyle="short" />;
}

/** What one sync did: "Pulled 3, pushed 2, rules files written". */
function SyncResultLine({ result }: { result: ContextSyncResult }) {
  const pulled = result.pull?.merged ?? 0;
  const pushed = result.push?.pushed ?? 0;
  return (
    <div data-slot="share-sync-result" className="mt-2 ml-7">
      <p className="text-sm">
        {syncWroteRulesFiles(result)
          ? t("Pulled {pulled}, pushed {pushed}, rules files written", { pulled, pushed })
          : t("Pulled {pulled}, pushed {pushed}", { pulled, pushed })}
      </p>
      {result.rules_files_error && (
        <p className="mt-1 text-[11px] text-muted-foreground">
          The rules files could not be written:{" "}
          <span className="font-mono" translate="no">
            {result.rules_files_error}
          </span>
        </p>
      )}
    </div>
  );
}

/**
 * Under Bowrain's rung: Preview, which draws the project as a Bowrain project
 * page draws it, beside a link. Until the recipe connects to a server the
 * link reads about Bowrain, followed by the command that connects it, which
 * needs the kapi-bowrain plugin; once it does, the server and workspace, and
 * the link opens the project there. A tab with no checkout leaves out the
 * command and Preview, since neither reads anything from there.
 */
function BowrainDetail({
  view,
  contextOnly,
  onPreview,
}: {
  view: ShareView;
  contextOnly?: boolean;
  /** Open the preview. Absent, the rung draws no Preview. */
  onPreview?: () => void;
}) {
  const sharing = view.sharing;
  const preview = onPreview && (
    <Button size="sm" variant="secondary" data-slot="share-preview-bowrain" onClick={onPreview}>
      <Eye size={13} aria-hidden />
      Preview
    </Button>
  );
  if (view.rung !== "bowrain") {
    return (
      <Actions>
        {preview}
        <Button size="sm" variant="outline" asChild>
          <a
            href={BOWRAIN_ABOUT_URL}
            target="_blank"
            rel="noopener noreferrer"
            data-slot="share-about-bowrain"
          >
            About Bowrain
            <ExternalLink size={13} aria-hidden />
          </a>
        </Button>
        {!contextOnly && <MonoChip slot="share-bowrain-command">{BOWRAIN_SHARE_COMMAND}</MonoChip>}
        {!contextOnly && !view.bowrain_plugin && (
          <span data-slot="share-bowrain-plugin" className="text-[11px] text-muted-foreground">
            Needs the <span translate="no">kapi-bowrain</span> plugin
          </span>
        )}
      </Actions>
    );
  }
  const venue = venueParts(sharing.venue_url);
  const link = venueLink(sharing);
  return (
    <>
      <Facts>
        <Fact label={t("Server")}>
          <MonoChip slot="share-server">{sharing.venue_host || t("A Bowrain server")}</MonoChip>
        </Fact>
        {venue.workspace && (
          <Fact label={t("Workspace")}>
            <MonoChip slot="share-venue-workspace">{venue.workspace}</MonoChip>
          </Fact>
        )}
      </Facts>
      {(preview || link) && (
        <Actions>
          {preview}
          {link && (
            <Button size="sm" variant="outline" asChild>
              <a
                href={link}
                target="_blank"
                rel="noopener noreferrer"
                data-slot="share-open-bowrain"
              >
                Open the project
                <ExternalLink size={13} aria-hidden />
              </a>
            </Button>
          )}
        </Actions>
      )}
    </>
  );
}

/**
 * The project's Bowrain URL when it is safe to open: http or https, on the
 * host the recipe names. The URL comes from the recipe's `bowrain:` block,
 * which anyone who can commit to the project writes, so any other scheme,
 * such as `javascript:`, is refused rather than handed to the webview.
 */
export function venueLink(
  sharing: Pick<ProjectSharing, "venue_url" | "venue_host">,
): string | null {
  if (!sharing.venue_url) return null;
  let url: URL;
  try {
    url = new URL(sharing.venue_url);
  } catch {
    return null;
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") return null;
  // The host may carry a port, as the backend reports it from the URL's host.
  const host = sharing.venue_host;
  if (host && url.hostname !== host && url.host !== host) return null;
  return url.href;
}

/** The workspace and project a Bowrain URL names: its path, without slashes. */
export function venueProject(url?: string): string {
  if (!url) return "";
  try {
    return new URL(url).pathname.replace(/^\/+|\/+$/g, "");
  } catch {
    return "";
  }
}

/**
 * A Bowrain project URL's workspace and project. A URL of two segments names
 * both, and `/projects/<id>` names a project with no workspace.
 */
export function venueParts(url?: string): { workspace?: string; project?: string } {
  const segments = venueProject(url).split("/").filter(Boolean);
  if (segments.length === 2 && segments[0] !== "projects") {
    return { workspace: segments[0], project: segments[1] };
  }
  return { project: segments[segments.length - 1] };
}
