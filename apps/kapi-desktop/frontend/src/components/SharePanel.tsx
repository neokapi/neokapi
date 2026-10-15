// Share: how far one project's context reaches, as a ladder of three rungs.
// On this Mac, the workspace every clone and worktree here reads. Shared with
// your team, the place kapi.yaml names under `context:`, which every machine
// that syncs the project reads. Managed on Bowrain, the server workspace a
// recipe's `bowrain:` block connects to. The rung the project stands on is
// marked, and each rung says where it stands and what can be done there.
//
// Sync is `kapi context sync` run here. Its result, or its error word for
// word, stays in the rung until the next sync.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  ErrorNotice,
  LoadingSpinner,
  PageHeader,
  When,
  cn,
  parseAppError,
} from "@neokapi/ui-primitives";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useShortenHome } from "../hooks/useShortenHome";
import { ShareSetupDialog } from "./ShareSetupDialog";
import {
  api,
  contextQk,
  type ContextSyncResult,
  type ProjectSharing,
  type SyncStatus,
} from "./contextTypes";

/** Paths, places and hosts: the project's own values, in the app's mono font. */
const MONO = "font-mono text-xs text-foreground";

/** Where the About Bowrain action leads. */
export const BOWRAIN_ABOUT_URL = "https://bowrain.cloud/docs/getting-started/kapi-vs-bowrain";

/** The rungs of the ladder, nearest first. */
export type ShareRung = "mac" | "team" | "bowrain";

/** The rung a project stands on: the farthest it reaches. */
export function shareRung(sharing: ProjectSharing): ShareRung {
  if (sharing.venue_host) return "bowrain";
  if (sharing.kind && sharing.kind !== "local") return "team";
  return "mac";
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
   * named in kapi.yaml, so only the first rung applies.
   */
  contextOnly?: boolean;
  /** Called once Set up sharing has saved a place to kapi.yaml. */
  onSetUp?: (sharing: ProjectSharing) => void;
}

export function SharePanel({ tabID, sharing: given, contextOnly, onSetUp }: SharePanelProps) {
  const qc = useQueryClient();
  const shorten = useShortenHome();
  const [settingUp, setSettingUp] = useState(false);
  const [result, setResult] = useState<ContextSyncResult | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);

  const key = contextQk.projectSharing(tabID);
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

  const at = shareRung(sharing);
  // Kept short and inline, so a full stop after it stays beside it.
  const workspace = (
    <code className={MONO} translate="no">
      {shorten(sharing.workspace)}
    </code>
  );
  return (
    <div data-slot="share-panel">
      <PageHeader title="Share" />
      <ol data-slot="share-ladder" className="divide-y divide-border/60">
        <Rung rung="mac" at={at} title={<>On this Mac</>}>
          <p data-slot="share-state" className="text-sm text-muted-foreground">
            In your workspace at {workspace}. Every clone and worktree of this project on this Mac
            reads it.
          </p>
        </Rung>

        {contextOnly ? (
          <li data-slot="share-needs-checkout" className="py-3 pl-4 text-sm text-muted-foreground">
            Sharing is named in the project&apos;s <span translate="no">kapi.yaml</span>, so it
            needs a checkout of this project on this Mac.
          </li>
        ) : (
          <>
            <Rung rung="team" at={at} title={<>Shared with your team</>}>
              <TeamState sharing={sharing} />
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {sharing.kind === "local" || !sharing.kind ? (
                  <Button
                    size="sm"
                    variant="outline"
                    data-slot="share-set-up"
                    onClick={() => setSettingUp(true)}
                  >
                    Set up sharing
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="outline"
                    data-slot="share-sync"
                    disabled={sync.isPending}
                    onClick={() => sync.mutate()}
                  >
                    Sync now
                  </Button>
                )}
                {sync.isPending && <LoadingSpinner size="sm" />}
              </div>
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
            </Rung>

            <Rung rung="bowrain" at={at} title={<>Managed on Bowrain</>}>
              <BowrainState sharing={sharing} />
              <div className="mt-2">
                <Button size="sm" variant="outline" asChild>
                  <a href={BOWRAIN_ABOUT_URL} data-slot="share-about-bowrain">
                    About Bowrain
                  </a>
                </Button>
              </div>
            </Rung>
          </>
        )}
      </ol>
      {!contextOnly && (
        <p className="mt-3 text-[11px] text-muted-foreground">
          The same as <code translate="no">kapi context sync</code>.
        </p>
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

/** One rung: its name, marked when the project stands on it, and its body. */
function Rung({
  rung,
  at,
  title,
  children,
}: {
  rung: ShareRung;
  at: ShareRung;
  title: React.ReactNode;
  children: React.ReactNode;
}) {
  const current = rung === at;
  return (
    <li
      data-slot="share-rung"
      data-rung={rung}
      data-current={current || undefined}
      aria-current={current ? "step" : undefined}
      className="relative py-3 pl-4"
    >
      {current && (
        <span
          aria-hidden
          data-slot="share-rung-marker"
          className="absolute left-0.5 top-[1.15rem] size-1.5 rounded-full bg-primary"
        />
      )}
      <h3 className={cn("text-sm", current ? "font-semibold" : "font-medium")}>{title}</h3>
      <div className="mt-1">{children}</div>
    </li>
  );
}

/** The state line of the team rung: not yet, in step, or not reached. */
function TeamState({ sharing }: { sharing: ProjectSharing }) {
  if (!sharing.kind || sharing.kind === "local") {
    return (
      <p data-slot="share-state" className="text-sm text-muted-foreground">
        Not yet. Name a place and sync, and every machine that syncs the project reads the same
        context.
      </p>
    );
  }
  return (
    <>
      <p data-slot="share-state" className="text-sm text-muted-foreground">
        <Through kind={sharing.kind} location={sharing.location} />{" "}
        {sharing.status && <Standing status={sharing.status} />}
      </p>
      {sharing.status?.error && (
        <p
          data-slot="share-status-error"
          className="mt-1 font-mono text-[11px] text-muted-foreground"
          translate="no"
        >
          {sharing.status.error}
        </p>
      )}
    </>
  );
}

/** "Through git (origin refs/kapi/context)." for the place kapi.yaml names. */
function Through({ kind, location }: { kind: string; location?: string }) {
  const where = (
    <code className={MONO} translate="no">
      {location}
    </code>
  );
  const name = <span translate="no">{kind}</span>;
  if (!location) {
    switch (kind) {
      case "git":
        return <>Through git.</>;
      case "file":
        return <>Through a folder.</>;
      case "s3":
        return <>Through S3.</>;
    }
    return <>Through {name}.</>;
  }
  switch (kind) {
    case "git":
      return <>Through git ({where}).</>;
    case "file":
      return <>Through a folder ({where}).</>;
    case "s3":
      return <>Through S3 ({where}).</>;
  }
  return (
    <>
      Through {name} ({where}).
    </>
  );
}

/** How far this Mac and the place are apart, as of the last contact. */
function Standing({ status }: { status: SyncStatus }) {
  if (!status.contacted) {
    return <>{status.to_push} to push. What waits to pull is known after the first sync.</>;
  }
  const when = <ContactTime iso={status.contacted} />;
  if (status.error) {
    return <>The place could not be reached at {when}.</>;
  }
  return (
    <>
      {status.to_push} to push, {status.to_pull} to pull, as of {when}.
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

/** The state line of the Bowrain rung. */
function BowrainState({ sharing }: { sharing: ProjectSharing }) {
  if (!sharing.venue_host) {
    return (
      <p data-slot="share-state" className="text-sm text-muted-foreground">
        Not connected. Bowrain holds every project&apos;s context in one workspace, with roles,
        review and a record of who decided what, and runs the project&apos;s loop on the server.
      </p>
    );
  }
  const host = (
    <span className="text-foreground" translate="no">
      {sharing.venue_host}
    </span>
  );
  const path = venueProject(sharing.venue_url);
  const project = <span translate="no">{path}</span>;
  return (
    <p data-slot="share-state" className="text-sm text-muted-foreground">
      {path ? (
        <>
          Connected to {host} ({project}).
        </>
      ) : (
        <>Connected to {host}.</>
      )}
    </p>
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
