// The assistants kapi can connect to one project, one row each: connected,
// with the files that carry the connection, or not, with Connect.
//
// Connect writes the files `kapi init` writes for that assistant and lists
// what it wrote. Claude Desktop reads a configuration file of the person's
// own, outside the project, so its Connect asks before it writes there.
// Assistants that are neither installed here nor connected come last, muted.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  ErrorNotice,
  LoadingSpinner,
  cn,
} from "@neokapi/ui-primitives";
import { useShortenHome } from "../hooks/useShortenHome";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { AgentHostStatus, AgentWiringResult } from "../types/api";

/** The assistant whose configuration is the person's own. */
export const CLAUDE_DESKTOP = "claude-desktop";

const MONO = "font-mono text-xs";

/** The hosts in the order the list shows them: present first, then the rest. */
export function orderHosts(hosts: AgentHostStatus[]): AgentHostStatus[] {
  const present = hosts.filter((h) => h.detected || h.wired);
  const absent = hosts.filter((h) => !h.detected && !h.wired);
  return [...present, ...absent];
}

/** Whether a host's row offers Connect. */
export function canConnect(host: AgentHostStatus): boolean {
  if (host.wired) return false;
  // Claude Desktop's configuration lives where the app is installed.
  return host.host !== CLAUDE_DESKTOP || host.detected;
}

export interface AssistantHostsProps {
  tabID: string;
  /** Pre-loaded hosts for Storybook and tests, which reach no backend. */
  hosts?: AgentHostStatus[];
  /** A tab with no checkout: there are no project files to connect through. */
  contextOnly?: boolean;
}

export function AssistantHosts({ tabID, hosts: given, contextOnly }: AssistantHostsProps) {
  const qc = useQueryClient();
  const shorten = useShortenHome();
  const [confirming, setConfirming] = useState<AgentHostStatus | null>(null);
  const [wrote, setWrote] = useState<{ host: string; result: AgentWiringResult } | null>(null);
  const [failed, setFailed] = useState<{ host: string; error: unknown } | null>(null);

  const key = qk.agentHosts(tabID);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.agentHosts(tabID),
    enabled: !given && !contextOnly,
  });

  const connect = useMutation({
    mutationFn: (host: string) => api.connectAgentHost(tabID, host),
    onMutate: () => {
      setWrote(null);
      setFailed(null);
    },
    onSuccess: (result, host) => setWrote({ host, result: result ?? {} }),
    onError: (error, host) => setFailed({ host, error }),
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });

  if (contextOnly) {
    return (
      <p data-slot="assistant-hosts-needs-checkout" className="text-sm text-muted-foreground">
        Connecting an assistant needs a checkout of this project on this Mac.
      </p>
    );
  }
  if (query.error && !given) {
    return <ErrorNotice error={query.error} title="The assistants could not be read" />;
  }
  const hosts = given ?? query.data ?? null;
  if (!hosts) return query.isLoading ? <LoadingSpinner /> : null;

  const onConnect = (host: AgentHostStatus) => {
    if (host.host === CLAUDE_DESKTOP) setConfirming(host);
    else connect.mutate(host.host);
  };

  return (
    <div data-slot="assistant-hosts">
      <ul className="divide-y divide-border/60">
        {orderHosts(hosts).map((h) => (
          <HostRow
            key={h.host}
            host={h}
            shorten={shorten}
            busy={connect.isPending && connect.variables === h.host}
            disabled={connect.isPending}
            onConnect={() => onConnect(h)}
            wrote={wrote?.host === h.host ? wrote.result : undefined}
            error={failed?.host === h.host ? failed.error : undefined}
          />
        ))}
      </ul>
      <p className="mt-2 text-[11px] text-muted-foreground">
        The same as <code translate="no">kapi init</code>.
      </p>

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

/** One assistant: its name, whether it is connected, and Connect. */
function HostRow({
  host,
  shorten,
  busy,
  disabled,
  onConnect,
  wrote,
  error,
}: {
  host: AgentHostStatus;
  shorten: (path: string) => string;
  busy: boolean;
  disabled: boolean;
  onConnect: () => void;
  wrote?: AgentWiringResult;
  error?: unknown;
}) {
  const absent = !host.detected && !host.wired;
  const label = (
    <span className="font-medium text-foreground" translate="no">
      {host.label}
    </span>
  );
  const files = (
    <code className={MONO} translate="no">
      {(host.files ?? []).map(shorten).join(", ")}
    </code>
  );
  let state: React.ReactNode;
  if (!host.wired) state = <>not connected</>;
  else if (host.files?.length) state = <>connected ({files})</>;
  else state = <>connected</>;

  return (
    <li
      data-slot="assistant-host"
      data-host={host.host}
      data-wired={host.wired || undefined}
      data-absent={absent || undefined}
      className={cn("py-2.5", absent && "opacity-60")}
    >
      <div className="flex items-center justify-between gap-3">
        <p data-slot="assistant-host-state" className="text-sm text-muted-foreground">
          {label} · {state}
        </p>
        {canConnect(host) && (
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
      {wrote && <WroteFiles result={wrote} shorten={shorten} />}
      {error !== undefined && (
        <ErrorNotice error={error} title="The assistant could not be connected" className="mt-2" />
      )}
    </li>
  );
}

/** What connecting wrote, one file per line. */
function WroteFiles({
  result,
  shorten,
}: {
  result: AgentWiringResult;
  shorten: (path: string) => string;
}) {
  const files = result.files ?? [];
  if (files.length === 0) {
    return (
      <p data-slot="assistant-host-wrote" className="mt-1 text-[11px] text-muted-foreground">
        Nothing was written.
      </p>
    );
  }
  return (
    <ul
      data-slot="assistant-host-wrote"
      className="mt-1 space-y-0.5 text-[11px] text-muted-foreground"
    >
      {files.map((f) => (
        <li key={f.path}>
          <code className="font-mono" translate="no">
            {shorten(f.path)}
          </code>{" "}
          · <WiringAction action={f.action} />
        </li>
      ))}
    </ul>
  );
}

/** What happened to one file, in words. */
function WiringAction({ action }: { action: string }) {
  switch (action) {
    case "created":
      return <>created</>;
    case "updated":
      return <>updated</>;
    case "unchanged":
      return <>unchanged</>;
    case "kept":
      return <>kept as it was</>;
  }
  return <span translate="no">{action}</span>;
}

/** The question Claude Desktop's Connect asks before it writes. */
function ClaudeDesktopConfirm({
  host,
  shorten,
  onCancel,
  onConfirm,
}: {
  host: AgentHostStatus | null;
  shorten: (path: string) => string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const config = host?.files?.[0];
  const name = <span translate="no">Claude Desktop</span>;
  const path = (
    <code className={MONO} translate="no">
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
