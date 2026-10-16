// Share: how far one project's context reaches, as a stepper over three rungs.
// On this Mac, the workspace every clone and worktree here reads. Shared with
// your team, the place kapi.yaml names under `context:`, which every machine
// that syncs the project reads. Managed on Bowrain, the server a recipe's
// `bowrain:` block connects to. The farthest rung reached is current.
//
// One card under the stepper describes the rung the reader selected, the
// current one until they click another: a sentence, the facts, and the one
// action that moves the project on. Sync is `kapi context sync` run here. Its
// result, or its error word for word, stays in the card until the next sync.

import { useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Cloud,
  ExternalLink,
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
  Stepper,
  When,
  parseAppError,
  type StepState,
  type StepperStep,
} from "@neokapi/ui-primitives";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useShortenHome } from "../hooks/useShortenHome";
import { ShareSetupDialog } from "./ShareSetupDialog";
import { api } from "../hooks/useApi";
import type { ContextSyncResult, ProjectSharing, SyncStatus } from "../types/api";

/** Where the About Bowrain action leads. */
export const BOWRAIN_ABOUT_URL = "https://bowrain.cloud/docs/getting-started/kapi-vs-bowrain";

/** The rungs of the ladder, nearest first. */
export type ShareRung = "mac" | "team" | "bowrain";

const RUNGS: ShareRung[] = ["mac", "team", "bowrain"];

/** True when kapi.yaml names a place to share the context through. */
export function sharingSetUp(sharing: ProjectSharing): boolean {
  return !!sharing.kind && sharing.kind !== "local";
}

/** The rung a project stands on: the farthest it reaches. */
export function shareRung(sharing: ProjectSharing): ShareRung {
  if (sharing.venue_host) return "bowrain";
  if (sharingSetUp(sharing)) return "team";
  return "mac";
}

/**
 * Each rung's state. The farthest rung reached is current, a reached rung
 * under it is done, and the rest are still to do, so a project connected to
 * Bowrain without a team place shows the team rung to do.
 */
export function rungStates(sharing: ProjectSharing): Record<ShareRung, StepState> {
  const reached: Record<ShareRung, boolean> = {
    mac: true,
    team: sharingSetUp(sharing),
    bowrain: !!sharing.venue_host,
  };
  const at = RUNGS.indexOf(shareRung(sharing));
  const state = (rung: ShareRung, i: number): StepState => {
    if (i === at) return "current";
    return i < at && reached[rung] ? "done" : "todo";
  };
  return { mac: state("mac", 0), team: state("team", 1), bowrain: state("bowrain", 2) };
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
  /** Pre-loaded sharing for Storybook and tests, which reach no backend. */
  sharing?: ProjectSharing;
  /**
   * A tab over a project's context alone, with no checkout here. Sharing is
   * named in kapi.yaml, so no rung can be set up from it.
   */
  contextOnly?: boolean;
  /** Called once Set up sharing has saved a place to kapi.yaml. */
  onSetUp?: (sharing: ProjectSharing) => void;
}

export function SharePanel({ tabID, sharing: given, contextOnly, onSetUp }: SharePanelProps) {
  const qc = useQueryClient();
  const titleId = useId();
  const [selected, setSelected] = useState<ShareRung | null>(null);
  const [settingUp, setSettingUp] = useState(false);
  const [result, setResult] = useState<ContextSyncResult | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);

  const key = qk.projectSharing(tabID);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.projectSharing(tabID),
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

  const sharing = given ?? query.data ?? null;
  if (query.error && !given) {
    return (
      <ErrorNotice
        error={query.error}
        title="Where this project's context is shared could not be read"
      />
    );
  }
  if (!sharing) return query.isLoading ? <LoadingSpinner /> : null;

  const states = rungStates(sharing);
  const shown = selected ?? shareRung(sharing);
  const steps: StepperStep[] = RUNGS.map((rung) => ({
    id: rung,
    label: <RungName rung={rung} />,
    state: states[rung],
  }));
  const setUp = () => setSettingUp(true);

  return (
    <div data-slot="share-panel">
      <PageHeader
        className="mb-5"
        title="Share"
        subtitle="How far this project's context reaches."
      />
      <Stepper
        data-slot="share-ladder"
        aria-label="How far this context reaches"
        value={shown}
        onChange={(id) => setSelected(id as ShareRung)}
        steps={steps}
        stepAttributes={(step) => ({
          "data-slot": "share-rung",
          "data-rung": step.id,
          "data-current": step.state === "current" || undefined,
        })}
      />

      <section
        data-slot="share-card"
        data-rung={shown}
        aria-labelledby={titleId}
        className="mt-5 rounded-lg border border-border px-4 py-3.5"
      >
        <h2 id={titleId} className="flex items-center gap-2 text-sm font-semibold">
          <RungIcon rung={shown} />
          <RungName rung={shown} />
        </h2>
        {shown === "mac" && <MacCard sharing={sharing} contextOnly={contextOnly} onSetUp={setUp} />}
        {shown === "team" && (
          <TeamCard
            sharing={sharing}
            contextOnly={contextOnly}
            onSetUp={setUp}
            onSync={() => sync.mutate()}
            syncing={sync.isPending}
            result={result}
            syncError={syncError}
          />
        )}
        {shown === "bowrain" && <BowrainCard sharing={sharing} />}
      </section>

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

/** A rung's name, written here so the stepper and the card say it the same way. */
function RungName({ rung }: { rung: ShareRung }) {
  switch (rung) {
    case "mac":
      return <>On this Mac</>;
    case "team":
      return <>Shared with your team</>;
    case "bowrain":
      return <>Managed on Bowrain</>;
  }
}

function RungIcon({ rung }: { rung: ShareRung }) {
  const props = { size: 15, "aria-hidden": true, className: "shrink-0 text-muted-foreground" };
  switch (rung) {
    case "mac":
      return <Laptop {...props} />;
    case "team":
      return <Users {...props} />;
    case "bowrain":
      return <Server {...props} />;
  }
}

/** The card's one sentence. */
function ShareSentence({ children }: { children: React.ReactNode }) {
  return (
    <p data-slot="share-state" className="mt-1 text-sm text-muted-foreground">
      {children}
    </p>
  );
}

/** The card's facts, one labelled row each. */
function ShareFacts({ children }: { children: React.ReactNode }) {
  return (
    <dl className="mt-3 grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-1.5">
      {children}
    </dl>
  );
}

function ShareFact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 items-center gap-1.5 text-sm">{children}</dd>
    </>
  );
}

/** A path, place or host: the project's own value, in the mono font. */
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

/** The card's actions, in one row. */
function Actions({ children }: { children: React.ReactNode }) {
  return <div className="mt-3.5 flex flex-wrap items-center gap-2">{children}</div>;
}

/** Why a context-only tab offers no setup. */
function NeedsCheckout() {
  return (
    <p data-slot="share-needs-checkout" className="mt-3 text-[11px] text-muted-foreground">
      Sharing is named in the project&apos;s <span translate="no">kapi.yaml</span>, so it needs a
      checkout of this project on this Mac.
    </p>
  );
}

function MacCard({
  sharing,
  contextOnly,
  onSetUp,
}: {
  sharing: ProjectSharing;
  contextOnly?: boolean;
  onSetUp: () => void;
}) {
  const shorten = useShortenHome();
  return (
    <>
      <ShareSentence>
        Every clone and worktree of this project on this Mac reads the same context.
      </ShareSentence>
      <ShareFacts>
        <ShareFact label="Workspace">
          <SimpleTooltip content={<span translate="no">{shorten(sharing.workspace)}</span>}>
            <span
              tabIndex={0}
              className="inline-flex min-w-0 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            >
              <MonoChip slot="share-workspace">{lastSegments(sharing.workspace, 2)}</MonoChip>
            </span>
          </SimpleTooltip>
        </ShareFact>
      </ShareFacts>
      {contextOnly ? (
        <NeedsCheckout />
      ) : (
        !sharingSetUp(sharing) && (
          <Actions>
            <Button size="sm" data-slot="share-set-up" onClick={onSetUp}>
              Set up sharing
            </Button>
          </Actions>
        )
      )}
    </>
  );
}

function TeamCard({
  sharing,
  contextOnly,
  onSetUp,
  onSync,
  syncing,
  result,
  syncError,
}: {
  sharing: ProjectSharing;
  contextOnly?: boolean;
  onSetUp: () => void;
  onSync: () => void;
  syncing: boolean;
  result: ContextSyncResult | null;
  syncError: string | null;
}) {
  if (!sharingSetUp(sharing)) {
    return (
      <>
        <ShareSentence>
          Every machine that syncs this project reads the same context, through a git ref, a folder
          or an S3 bucket.
        </ShareSentence>
        {contextOnly ? (
          <NeedsCheckout />
        ) : (
          <Actions>
            <Button size="sm" data-slot="share-set-up" onClick={onSetUp}>
              Set up sharing
            </Button>
          </Actions>
        )}
      </>
    );
  }
  const status = sharing.status;
  return (
    <>
      <ShareSentence>Every machine that syncs this project reads the same context.</ShareSentence>
      <ShareFacts>
        <ShareFact label="Through">
          <PlaceKind kind={sharing.kind} />
          {sharing.location && <MonoChip slot="share-place">{sharing.location}</MonoChip>}
        </ShareFact>
        {status && <Standing status={status} />}
      </ShareFacts>
      {status?.error && (
        <div className="mt-3">
          <p className="flex items-center gap-1.5 text-sm text-warning">
            <TriangleAlert size={14} aria-hidden className="shrink-0" />
            {status.contacted ? (
              <>
                The place could not be reached at <ContactTime iso={status.contacted} />.
              </>
            ) : (
              <>The place could not be reached.</>
            )}
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
        <Button size="sm" variant="ghost" data-slot="share-change" onClick={onSetUp}>
          Change
        </Button>
        {syncing && <LoadingSpinner size="sm" />}
      </Actions>
      {result && <SyncResultLine result={result} />}
      {syncError && (
        <p
          data-slot="share-sync-error"
          role="alert"
          className="mt-2 font-mono text-xs text-destructive"
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
          {t("A folder")}
        </span>
      );
    case "s3":
      return (
        <span data-slot="share-kind" className="inline-flex items-center gap-1.5">
          <Cloud {...icon} />
          <span translate="no">S3</span>
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

/** When this Mac last reached the place, and how far apart the two were then. */
function Standing({ status }: { status: SyncStatus }) {
  return (
    <>
      <ShareFact label="Last sync">
        <span data-slot="share-last-sync">
          {status.contacted ? <ContactTime iso={status.contacted} /> : <>Not yet from this Mac</>}
        </span>
      </ShareFact>
      <ShareFact label="To push">
        <span data-slot="share-to-push" className="tabular-nums">
          {status.to_push}
        </span>
      </ShareFact>
      <ShareFact label="To pull">
        <span data-slot="share-to-pull" className="tabular-nums">
          {status.contacted ? status.to_pull : <>Known after the first sync</>}
        </span>
      </ShareFact>
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
    <div data-slot="share-sync-result" className="mt-2">
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

function BowrainCard({ sharing }: { sharing: ProjectSharing }) {
  if (!sharing.venue_host) {
    return (
      <>
        <ShareSentence>
          Bowrain holds every project&apos;s context in one workspace, with roles, review and a
          record of who decided what, and runs the project&apos;s loop on the server.
        </ShareSentence>
        <Actions>
          <Button size="sm" variant="outline" asChild>
            <a href={BOWRAIN_ABOUT_URL} data-slot="share-about-bowrain">
              About Bowrain
            </a>
          </Button>
        </Actions>
      </>
    );
  }
  const venue = venueParts(sharing.venue_url);
  return (
    <>
      <ShareSentence>The recipe connects this project to a Bowrain server.</ShareSentence>
      <ShareFacts>
        <ShareFact label="Server">
          <MonoChip slot="share-server">{sharing.venue_host}</MonoChip>
        </ShareFact>
        {venue.workspace && (
          <ShareFact label="Workspace">
            <MonoChip slot="share-venue-workspace">{venue.workspace}</MonoChip>
          </ShareFact>
        )}
      </ShareFacts>
      {sharing.venue_url && (
        <Actions>
          <Button size="sm" variant="outline" asChild>
            <a href={sharing.venue_url} data-slot="share-open-bowrain">
              Open the project
              <ExternalLink size={13} aria-hidden />
            </a>
          </Button>
        </Actions>
      )}
    </>
  );
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
