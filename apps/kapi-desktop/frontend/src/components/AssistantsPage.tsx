// The Assistants page: which assistants work on the project, and the sessions
// they and the person recorded in it. A settings-and-setup page: one row per
// assistant with its state and Connect, then the sessions as rows, each
// leading to the Sessions view where its entries are listed and decided. A
// session row counts what its check runs found, and the header totals what
// they caught across the sessions shown. What
// an assistant reads is the same for every assistant, so its action sits in
// the page header.
//
// Connect does what `kapi connect <host>` does for that one assistant: the
// rules files, its MCP entry and skill, and the recipe of a folder opened
// without one. Claude Desktop reads a configuration file of the person's own,
// outside the project, so its Connect asks before it writes there. The check
// when Claude Code finishes comes from the kapi Claude Code plugin; until the
// project or the person enables it, the row names the two commands that
// install it.

import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronRight, Inbox } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ActorMark,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  EmptyState,
  ErrorNotice,
  LoadingSpinner,
  PageHeader,
  SectionHeading,
  SimpleTooltip,
  When,
  cn,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useShortenHome } from "../hooks/useShortenHome";
import { qk } from "../lib/queryKeys";
import { ActorName, SessionMark, checkCounts } from "./ContextFeed";
import type {
  AgentWiringResult,
  AssistantHostView,
  AssistantSession,
  AssistantsView,
  ConnectResult,
} from "../types/api";

/** The assistant whose configuration is the person's own. */
export const CLAUDE_DESKTOP = "claude-desktop";

export interface AssistantsPageProps {
  tabID: string;
  /** The open project's name, for the title. */
  projectName: string;
  /** Show what an assistant reads: the rules files and the brief for a file. */
  onShowReads: () => void;
  /** Open the Sessions view, on one session or on the newest. */
  onOpenSession: (id?: string) => void;
  /** Pre-loaded page for Storybook and tests, which reach no backend. */
  view?: AssistantsView;
}

/** What one Connect wrote, for the row it was pressed on. */
type Wrote =
  | { kind: "connect"; result: ConnectResult }
  | { kind: "wiring"; result: AgentWiringResult };

export function AssistantsPage({
  tabID,
  projectName,
  onShowReads,
  onOpenSession,
  view: given,
}: AssistantsPageProps) {
  const qc = useQueryClient();
  const shorten = useShortenHome();
  const [confirming, setConfirming] = useState<AssistantHostView | null>(null);
  const [wrote, setWrote] = useState<{ host: string; wrote: Wrote } | null>(null);
  const [failed, setFailed] = useState<{ host: string; error: unknown } | null>(null);

  const key = qk.assistants(tabID);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.assistantsView(tabID),
    enabled: !given,
    placeholderData: (previous) => previous,
  });
  // A session an assistant records in its own process reaches the page
  // through the workspace watcher.
  useInvalidateOnEvent("workspace:changed", [key]);

  const connect = useMutation({
    mutationFn: async (host: string): Promise<Wrote> => {
      if (host === CLAUDE_DESKTOP) {
        return { kind: "wiring", result: (await api.connectAgentHost(tabID, host)) ?? {} };
      }
      const result = await api.connectAssistant(tabID, host);
      return { kind: "connect", result: result ?? emptyResult() };
    },
    onMutate: () => {
      setWrote(null);
      setFailed(null);
    },
    onSuccess: (result, host) => setWrote({ host, wrote: result }),
    onError: (error, host) => setFailed({ host, error }),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: key });
      void qc.invalidateQueries({ queryKey: qk.rulesFiles(tabID) });
      void qc.invalidateQueries({ queryKey: qk.agentHosts(tabID) });
    },
  });

  const view = given ?? query.data ?? null;
  const onConnect = (host: AssistantHostView) => {
    if (host.personal) setConfirming(host);
    else connect.mutate(host.host);
  };
  // The empty sessions' action takes the person to the first Connect.
  const hostsRef = useRef<HTMLElement>(null);
  const focusConnect = () => {
    const button = hostsRef.current?.querySelector<HTMLButtonElement>(
      "[data-slot='assistant-host-connect']",
    );
    button?.scrollIntoView?.({ block: "center" });
    button?.focus();
  };

  return (
    <div data-slot="assistants-page">
      <PageHeader
        title={t("{project} · Assistants", { project: projectName })}
        subtitle={
          view &&
          !view.needs_checkout &&
          view.rules !== undefined && <PitchLine rules={view.rules} />
        }
        actions={
          view && (
            <>
              {!view.sessions_error && <CaughtTotal sessions={view.sessions} />}
              {/* Every assistant reads the same rules files and briefs, so what
                  it reads is the page's, reachable before any assistant is
                  connected. */}
              {!view.needs_checkout && (
                <Button
                  size="sm"
                  variant="outline"
                  data-slot="assistants-reads"
                  onClick={onShowReads}
                >
                  Show what it reads
                </Button>
              )}
            </>
          )
        }
      />
      {query.error && !given ? (
        <ErrorNotice error={query.error} title="The assistants could not be read" />
      ) : !view ? (
        query.isLoading && <LoadingSpinner />
      ) : (
        <>
          <section data-slot="assistants-hosts" ref={hostsRef}>
            <SectionHeading className="mb-1">Assistants</SectionHeading>
            {view.needs_checkout ? (
              <p data-slot="assistants-needs-checkout" className="text-sm text-muted-foreground">
                Connecting an assistant needs a checkout of this project on this Mac.
              </p>
            ) : view.hosts_error ? (
              <div data-slot="assistants-hosts-error">
                <ErrorNotice error={view.hosts_error} title="The assistants could not be read" />
              </div>
            ) : (
              <ul className="divide-y divide-border/60">
                {view.hosts.map((h) => (
                  <HostRow
                    key={h.host}
                    host={h}
                    shorten={shorten}
                    busy={connect.isPending && connect.variables === h.host}
                    disabled={connect.isPending}
                    onConnect={() => onConnect(h)}
                    wrote={wrote?.host === h.host ? wrote.wrote : undefined}
                    error={failed?.host === h.host ? failed.error : undefined}
                  />
                ))}
              </ul>
            )}
          </section>

          <section data-slot="assistants-sessions" className="mt-8">
            <SectionHeading className="mb-1">Sessions</SectionHeading>
            {view.sessions_error ? (
              <div data-slot="assistants-sessions-error">
                <ErrorNotice error={view.sessions_error} title="The sessions could not be read" />
              </div>
            ) : (
              <SessionList
                sessions={view.sessions}
                onOpenSession={onOpenSession}
                onConnect={
                  view.hosts.some((h) => !h.connected && h.available) ? focusConnect : undefined
                }
              />
            )}
          </section>
        </>
      )}

      <ClaudeDesktopConfirm
        host={confirming}
        shorten={shorten}
        onCancel={() => setConfirming(null)}
        onConfirm={() => {
          setConfirming(null);
          connect.mutate(CLAUDE_DESKTOP);
        }}
      />
    </div>
  );
}

function emptyResult(): ConnectResult {
  return { root: "", recipe: "", dry_run: false, hosts: [], files: [] };
}

/** What connecting does, in three facts. */
function PitchLine({ rules }: { rules: number }) {
  return (
    <span data-slot="assistants-pitch">
      {t("Checks every edit")} ·{" "}
      {t("{count, plural, one {follows your # rule} other {follows your # rules}}", {
        count: rules,
      })}{" "}
      · {t("leaves suggestions here")}
    </span>
  );
}

/**
 * What the check on every edit caught across the sessions shown, once any of
 * them ran a check.
 */
function CaughtTotal({ sessions }: { sessions: AssistantSession[] }) {
  if (!sessions.some((s) => s.checked > 0)) return null;
  const caught = sessions.reduce((sum, s) => sum + s.caught, 0);
  return (
    <span data-slot="assistants-caught" className="text-xs text-muted-foreground tabular-nums">
      {t("{count} caught", { count: caught })}
    </span>
  );
}

/** One assistant: its mark and name, its state, and Connect while it is not connected. */
function HostRow({
  host,
  shorten,
  busy,
  disabled,
  onConnect,
  wrote,
  error,
}: {
  host: AssistantHostView;
  shorten: (path: string) => string;
  busy: boolean;
  disabled: boolean;
  onConnect: () => void;
  wrote?: Wrote;
  error?: unknown;
}) {
  const files = (host.files ?? []).map(shorten);
  const pending = host.connected && (host.pending?.length ?? 0) > 0;

  return (
    <li
      data-slot="assistant-host"
      data-host={host.host}
      data-connected={host.connected || undefined}
      className="py-2.5"
    >
      <div className="flex min-h-8 items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <ActorMark actor={host.host} name={host.label} size="md" decorative />
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-foreground" translate="no">
              {host.label}
            </p>
            <p data-slot="assistant-host-state" className="text-[11px] text-muted-foreground">
              {host.connected ? (
                <SimpleTooltip
                  content={
                    files.length > 0 ? (
                      <span className="font-mono" translate="no">
                        {files.join(", ")}
                      </span>
                    ) : undefined
                  }
                >
                  <span className="inline-flex items-center gap-1">
                    <Check size={11} aria-hidden className="text-success" />
                    {t("Connected")} · <Gets check={host.check} />
                  </span>
                </SimpleTooltip>
              ) : host.reason === "rules files missing" ? (
                t("Not connected: rules files missing")
              ) : (
                t("Not connected")
              )}
            </p>
          </div>
        </div>
        {!host.connected && host.available && (
          <Button
            size="sm"
            variant="outline"
            data-slot="assistant-host-connect"
            disabled={disabled}
            onClick={onConnect}
          >
            {busy && <LoadingSpinner size="sm" />}
            Connect
          </Button>
        )}
      </div>
      {pending && (
        <p
          data-slot="assistant-host-pending"
          className="mt-1 flex flex-wrap items-center gap-1.5 pl-[32px] text-[11px] text-muted-foreground"
        >
          <span>{t("Check on every edit, in Claude Code:")}</span>
          {host.pending!.map((cmd) => (
            <code
              key={cmd}
              translate="no"
              className="rounded border border-border px-1 font-mono text-[11px]"
            >
              {cmd}
            </code>
          ))}
        </p>
      )}
      {wrote && <WroteFiles wrote={wrote} shorten={shorten} />}
      {error !== undefined && (
        <ErrorNotice error={error} title="The assistant could not be connected" className="mt-2" />
      )}
    </li>
  );
}

/** What a connected assistant gets. */
function Gets({ check }: { check: boolean }) {
  return check ? <>{t("rules + check on every edit")}</> : <>{t("rules")}</>;
}

/** The files one Connect wrote, one per line; the files it kept go unnamed. */
function WroteFiles({ wrote, shorten }: { wrote: Wrote; shorten: (path: string) => string }) {
  const files =
    wrote.kind === "connect"
      ? [...wrote.result.hosts.flatMap((h) => h.files ?? []), ...wrote.result.files]
          .filter((f) => f.path && f.action !== "kept" && f.action !== "pending")
          .map((f) => ({ path: f.path!, action: f.action }))
      : (wrote.result.files ?? [])
          .filter((f) => f.action !== "kept" && f.action !== "unchanged")
          .map((f) => ({ path: f.path, action: f.action }));
  if (files.length === 0) {
    return (
      <p
        data-slot="assistant-host-wrote"
        className="mt-1 pl-[32px] text-[11px] text-muted-foreground"
      >
        Nothing was written.
      </p>
    );
  }
  return (
    <ul
      data-slot="assistant-host-wrote"
      className="mt-1 space-y-0.5 pl-[32px] text-[11px] text-muted-foreground"
    >
      {files.map((f) => (
        <li key={f.path}>
          <code className="font-mono" translate="no">
            {shorten(f.path)}
          </code>{" "}
          · <FileAction action={f.action} />
        </li>
      ))}
    </ul>
  );
}

/** What happened to one file, in words. */
function FileAction({ action }: { action: string }) {
  switch (action) {
    case "created":
      return <>created</>;
    case "updated":
      return <>updated</>;
    case "removed":
      return <>section removed</>;
    case "deleted":
      return <>deleted</>;
  }
  return <span translate="no">{action}</span>;
}

/** The newest sessions, each leading to its entries. */
function SessionList({
  sessions,
  onOpenSession,
  onConnect,
}: {
  sessions: AssistantSession[];
  onOpenSession: (id?: string) => void;
  /** Take the person to an assistant's Connect. Absent when none is left to connect. */
  onConnect?: () => void;
}) {
  if (sessions.length === 0) {
    return (
      <EmptyState
        icon={<Inbox size={20} />}
        title="No sessions yet"
        description="What an assistant or you record in this project appears here, one row per session."
        action={
          onConnect && (
            <Button
              size="sm"
              variant="outline"
              data-slot="assistant-sessions-connect"
              onClick={onConnect}
            >
              Connect an assistant
            </Button>
          )
        }
      />
    );
  }
  return (
    <>
      <ul data-slot="assistant-sessions" className="-mx-2 flex flex-col">
        {sessions.map((s) => (
          <li key={s.id}>
            <button
              type="button"
              data-slot="assistant-session"
              data-session={s.id}
              onClick={() => onOpenSession(s.id)}
              className={cn(
                "flex min-h-11 w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left",
                "hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none",
              )}
            >
              <SessionMark actor={s.actor} size="md" />
              <span className="min-w-0 flex-1">
                <span className="flex min-w-0 items-center gap-1.5 text-sm">
                  <span className="truncate">
                    <ActorName actor={s.actor} />
                  </span>
                  <When
                    iso={s.last}
                    relative
                    className="shrink-0 text-[11px] text-muted-foreground"
                  />
                </span>
                {s.operations > 0 && (
                  <span
                    data-slot="assistant-session-counts"
                    className="block text-[11px] text-muted-foreground"
                  >
                    {sessionCounts(s)}
                  </span>
                )}
              </span>
              {s.checked > 0 && (
                <span
                  data-slot="assistant-session-checks"
                  className="shrink-0 text-[11px] text-muted-foreground tabular-nums"
                >
                  {checkCounts(s).join(" · ")}
                </span>
              )}
              {s.awaiting > 0 && (
                <span
                  role="img"
                  data-slot="assistant-session-awaiting"
                  aria-label={t("{count} suggested", { count: s.awaiting })}
                  className="size-1.5 shrink-0 rounded-full bg-warning"
                />
              )}
              <ChevronRight size={14} aria-hidden className="shrink-0 text-muted-foreground" />
            </button>
          </li>
        ))}
      </ul>
      <Button
        size="sm"
        variant="ghost"
        data-slot="assistant-sessions-all"
        className="mt-1 -ml-2"
        onClick={() => onOpenSession()}
      >
        {t("All sessions")}
      </Button>
    </>
  );
}

/**
 * A session's counts, at most three, each only where the session has one. A
 * session with none of them counts its operations.
 */
export function sessionCounts(s: AssistantSession): string {
  const parts: string[] = [];
  if (s.suggestions > 0)
    parts.push(
      t("{count, plural, one {# suggestion} other {# suggestions}}", { count: s.suggestions }),
    );
  if (s.corrections > 0)
    parts.push(
      t("{count, plural, one {# correction} other {# corrections}}", { count: s.corrections }),
    );
  if (s.kept > 0) parts.push(t("{count} kept", { count: s.kept }));
  if (s.dropped > 0) parts.push(t("{count} dropped", { count: s.dropped }));
  if (parts.length === 0)
    parts.push(
      t("{count, plural, one {# operation} other {# operations}}", { count: s.operations }),
    );
  return parts.slice(0, 3).join(" · ");
}

/** The question Claude Desktop's Connect asks before it writes. */
function ClaudeDesktopConfirm({
  host,
  shorten,
  onCancel,
  onConfirm,
}: {
  host: AssistantHostView | null;
  shorten: (path: string) => string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const config = host?.files?.[0];
  const name = <span translate="no">Claude Desktop</span>;
  const path = (
    <code className="font-mono text-xs" translate="no">
      {config ? shorten(config) : ""}
    </code>
  );
  return (
    <Dialog open={host !== null} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="sm:max-w-md" data-slot="claude-desktop-confirm">
        <DialogHeader>
          <DialogTitle>Connect {name}?</DialogTitle>
          <DialogDescription>
            {config ? (
              <>
                This writes an entry into your own {name} configuration at {path}.
              </>
            ) : (
              <>This writes an entry into your own {name} configuration.</>
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
          <Button data-slot="claude-desktop-confirm-connect" onClick={onConfirm}>
            Connect
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
