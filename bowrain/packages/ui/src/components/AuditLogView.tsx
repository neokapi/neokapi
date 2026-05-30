import { Badge, Button, Card } from "@neokapi/ui-primitives";
import { useState, useMemo, useCallback } from "react";
import type {
  AuditEntry,
  AuditChainVerification,
  ProjectInfo,
} from "../types/api";
import type { FilterToken, FilterField, FilterPreset } from "./FilterBar";
import { FilterBar } from "./FilterBar";
import {
  ChevronDown,
  ChevronUp,
  Clock,
  Sparkles,
  GitBranch,
  Layers,
  Package,
  Globe,
  Plug,
  FileCode,
  ArrowRight,
  Search,
  Shield,
  KeyRound,
  UserPlus,
  Users,
  LogIn,
  AlertTriangle,
} from "./icons";

// ---------------------------------------------------------------------------
// Event description helpers
// ---------------------------------------------------------------------------

const EVENT_CATEGORIES: Record<
  string,
  { label: string; icon: typeof Package }
> = {
  project: { label: "Project", icon: Package },
  block: { label: "Block", icon: FileCode },
  stream: { label: "Stream", icon: GitBranch },
  collection: { label: "Collection", icon: Layers },
  item: { label: "Item", icon: Globe },
  connector: { label: "Connector", icon: Plug },
  version: { label: "Version", icon: Clock },
  flow: { label: "Flow", icon: ArrowRight },
  brand: { label: "Brand Voice", icon: Sparkles },
  quality: { label: "Quality", icon: Sparkles },
  extraction: { label: "Extraction", icon: Search },
  // Security & governance
  member: { label: "Membership", icon: UserPlus },
  role: { label: "Roles", icon: Shield },
  invite: { label: "Invites", icon: UserPlus },
  token: { label: "API Tokens", icon: KeyRound },
  auth: { label: "Authentication", icon: LogIn },
  session: { label: "Agent Sessions", icon: Users },
  authz: { label: "Access Denials", icon: AlertTriangle },
  rollback: { label: "Rollbacks", icon: ArrowRight },
};

const EVENT_DESCRIPTIONS: Record<string, string> = {
  "project.created": "Created project",
  "project.updated": "Updated project",
  "project.deleted": "Deleted project",
  "block.created": "Created block",
  "block.updated": "Updated block",
  "block.deleted": "Deleted block",
  "stream.created": "Created stream",
  "stream.merged": "Merged stream",
  "stream.deleted": "Deleted stream",
  "collection.created": "Created collection",
  "collection.updated": "Updated collection",
  "collection.deleted": "Deleted collection",
  "item.created": "Added item",
  "item.deleted": "Removed item",
  "connector.push.completed": "Push completed",
  "connector.pull.completed": "Pull completed",
  "connector.sync.completed": "Sync completed",
  "version.created": "Created version",
  "flow.started": "Started flow",
  "flow.completed": "Completed flow",
  "flow.failed": "Flow failed",
  "extraction.completed": "Extraction completed",
  "quality.gate.pass": "Quality gate passed",
  "quality.gate.fail": "Quality gate failed",
  "brand.voice.check.completed": "Brand voice check completed",
  "brand.profile.updated": "Updated brand profile",
  "member.added": "Added member",
  "member.removed": "Removed member",
  "member.role_changed": "Changed member role",
  "role.template.created": "Created role",
  "role.template.updated": "Updated role permissions",
  "role.template.deleted": "Deleted role",
  "invite.created": "Created invite",
  "invite.accepted": "Accepted invite",
  "invite.revoked": "Revoked invite",
  "token.created": "Created API token",
  "token.revoked": "Revoked API token",
  "auth.login": "Signed in",
  "auth.logout": "Signed out",
  "auth.failed": "Failed sign-in",
  "session.grant.created": "Granted agent session",
  "authz.denied": "Access denied",
  "rollback.performed": "Rolled back changes",
};

function describeEvent(eventType: string): string {
  return EVENT_DESCRIPTIONS[eventType] ?? eventType.replace(/\./g, " ");
}

function getCategoryFromType(eventType: string): string {
  return eventType.split(".")[0] ?? "other";
}

function getEventIcon(eventType: string) {
  const category = getCategoryFromType(eventType);
  return EVENT_CATEGORIES[category]?.icon ?? Package;
}

function relativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHr = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHr / 24);

  if (diffSec < 60) return "just now";
  if (diffMin < 60) return diffMin + "m ago";
  if (diffHr < 24) return diffHr + "h ago";
  if (diffDay === 1) return "yesterday";
  if (diffDay < 30) return diffDay + "d ago";
  return date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function parseData(dataStr: string): Record<string, string> {
  try {
    return JSON.parse(dataStr);
  } catch {
    return {};
  }
}

function buildDetails(
  _eventType: string,
  data: Record<string, string>,
): string {
  const parts: string[] = [];
  if (data.name) parts.push(data.name);
  if (data.item_name) parts.push(data.item_name);
  if (data.stream && data.stream !== "main")
    parts.push("on stream " + data.stream);
  if (data.parent) parts.push("from " + data.parent);
  if (data.format) parts.push("(" + data.format + ")");
  if (data.kind) parts.push("[" + data.kind + "]");
  if (data.items) {
    const itemList = data.items.split(",");
    parts.push(itemList.length + " item" + (itemList.length !== 1 ? "s" : ""));
  }
  if (data.block_id) parts.push("block " + data.block_id.slice(0, 8));
  if (data.collection_id)
    parts.push("collection " + data.collection_id.slice(0, 8));
  return parts.join(" · ");
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export interface AuditLogViewProps {
  entries: AuditEntry[];
  projects?: ProjectInfo[];
  loading?: boolean;
  hasMore?: boolean;
  onLoadMore?: () => void;
  onFiltersChange?: (filters: FilterToken[]) => void;
  onSearchChange?: (search: string) => void;
  activeFilters?: FilterToken[];
  activeSearch?: string;
  /** Tamper-evidence verification of the workspace audit chain. */
  verification?: AuditChainVerification | null;
  onVerify?: () => void;
  verifying?: boolean;
}

export function AuditLogView({
  entries,
  projects,
  loading,
  hasMore,
  onLoadMore,
  onFiltersChange,
  onSearchChange,
  activeFilters = [],
  activeSearch = "",
  verification,
  onVerify,
  verifying,
}: AuditLogViewProps) {
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set());

  const toggleExpanded = useCallback((id: number) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // Build filter field definitions for the FilterBar
  const filterFields = useMemo<FilterField[]>(() => {
    const fields: FilterField[] = [
      {
        key: "type",
        label: "Event type",
        hint: "filter by event category",
        values: Object.entries(EVENT_CATEGORIES).map(([k, v]) => ({
          value: k,
          label: v.label,
        })),
      },
      {
        key: "actor",
        label: "Actor",
        hint: "filter by who performed the action",
      },
    ];
    if (projects && projects.length > 0) {
      fields.unshift({
        key: "project",
        label: "Project",
        hint: "filter by project",
        values: projects.map((p) => ({ value: p.id, label: p.name })),
      });
    }
    return fields;
  }, [projects]);

  // Quick filter presets
  const filterPresets = useMemo<FilterPreset[]>(
    () => [
      { label: "Content changes", filters: [{ key: "type", value: "block" }] },
      {
        label: "Membership & roles",
        filters: [{ key: "type", value: "member" }],
      },
      { label: "Access denials", filters: [{ key: "type", value: "authz" }] },
      { label: "Sign-ins", filters: [{ key: "type", value: "auth" }] },
      {
        label: "Push & sync events",
        filters: [{ key: "type", value: "connector" }],
      },
    ],
    [],
  );

  // Group entries by date
  const groupedEntries = useMemo(() => {
    const groups: { label: string; entries: AuditEntry[] }[] = [];
    let currentLabel = "";
    for (const entry of entries) {
      const date = new Date(entry.created_at);
      const today = new Date();
      const yesterday = new Date(today);
      yesterday.setDate(yesterday.getDate() - 1);

      let label: string;
      if (date.toDateString() === today.toDateString()) {
        label = "Today";
      } else if (date.toDateString() === yesterday.toDateString()) {
        label = "Yesterday";
      } else {
        label = date.toLocaleDateString(undefined, {
          weekday: "long",
          month: "long",
          day: "numeric",
          year:
            date.getFullYear() !== today.getFullYear() ? "numeric" : undefined,
        });
      }

      if (label !== currentLabel) {
        groups.push({ label, entries: [] });
        currentLabel = label;
      }
      groups[groups.length - 1].entries.push(entry);
    }
    return groups;
  }, [entries]);

  const projectNames = useMemo(() => {
    const map: Record<string, string> = {};
    if (projects) {
      for (const p of projects) map[p.id] = p.name;
    }
    return map;
  }, [projects]);

  const hasActiveFilters = activeFilters.length > 0 || activeSearch !== "";

  return (
    <div className="flex-1 min-h-0 overflow-auto">
      <Card className="p-6 mb-4">
        <div className="mb-6 flex items-start justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold">Audit Log</h2>
            <p className="text-[13px] text-muted-foreground mt-1">
              Tamper-evident record of all activity and access changes in this
              workspace
            </p>
          </div>
          {onVerify && (
            <div className="flex items-center gap-2 shrink-0">
              {verification &&
                (verification.valid ? (
                  <Badge
                    variant="secondary"
                    className="gap-1 text-[11px] text-emerald-600 dark:text-emerald-400"
                  >
                    <Shield className="w-3 h-3" /> Chain verified (
                    {verification.rows})
                  </Badge>
                ) : (
                  <Badge variant="destructive" className="gap-1 text-[11px]">
                    <AlertTriangle className="w-3 h-3" /> Tampering detected
                    {verification.broken_at
                      ? ` (#${verification.broken_at})`
                      : ""}
                  </Badge>
                ))}
              <Button
                variant="outline"
                size="sm"
                onClick={onVerify}
                disabled={verifying}
              >
                {verifying ? "Verifying…" : "Verify integrity"}
              </Button>
            </div>
          )}
        </div>

        {/* FilterBar */}
        <FilterBar
          filters={activeFilters}
          onFiltersChange={onFiltersChange ?? (() => {})}
          search={activeSearch}
          onSearchChange={onSearchChange ?? (() => {})}
          fields={filterFields}
          presets={filterPresets}
          placeholder="Search audit logs..."
        />
      </Card>

      {/* Event list */}
      <Card className="p-0 overflow-hidden">
        {entries.length === 0 && !loading && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Clock className="w-10 h-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">
              No audit events found
            </p>
            <p className="text-[12px] text-muted-foreground/60 mt-1">
              {hasActiveFilters
                ? "Try adjusting your filters"
                : "Events will appear here as activity occurs"}
            </p>
          </div>
        )}

        {groupedEntries.map((group) => (
          <div key={group.label}>
            <div className="sticky top-0 z-10 px-5 py-2 bg-muted/40 backdrop-blur-sm border-b border-border/30">
              <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                {group.label}
              </span>
            </div>

            {group.entries.map((entry) => {
              const data = parseData(entry.data);
              const details = buildDetails(entry.event_type, data);
              const isExpanded = expandedIds.has(entry.id);
              const isDenied = entry.effect === "deny";
              const Icon = isDenied
                ? AlertTriangle
                : getEventIcon(entry.event_type);
              const projectName = entry.project_id
                ? (projectNames[entry.project_id] ??
                  entry.project_id.slice(0, 8))
                : entry.workspace_id
                  ? "workspace"
                  : "system";
              const dataEntries = Object.entries(data);
              const before = parseData(entry.before ?? "");
              const after = parseData(entry.after ?? "");
              const diffKeys = Array.from(
                new Set([...Object.keys(before), ...Object.keys(after)]),
              );
              const resourceLabel = entry.resource_type
                ? `${entry.resource_type}${entry.resource_id ? " " + entry.resource_id.slice(0, 8) : ""}`
                : "";
              const hasMeta = Boolean(
                entry.ip || entry.user_agent || entry.request_id || entry.hash,
              );
              const hasExtra =
                dataEntries.length > 0 || diffKeys.length > 0 || hasMeta;

              return (
                <div
                  key={entry.id}
                  className="border-b border-border/20 last:border-b-0 transition-colors hover:bg-accent/30"
                >
                  <div
                    className="flex items-start gap-3 px-5 py-3 cursor-pointer"
                    onClick={() => hasExtra && toggleExpanded(entry.id)}
                  >
                    <div
                      className={
                        "mt-0.5 w-8 h-8 rounded-full flex items-center justify-center shrink-0 " +
                        (isDenied ? "bg-destructive/10" : "bg-muted/60")
                      }
                    >
                      <Icon
                        className={
                          "w-4 h-4 " +
                          (isDenied
                            ? "text-destructive"
                            : "text-muted-foreground")
                        }
                      />
                    </div>

                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        {entry.actor && (
                          <span className="text-sm font-medium text-primary">
                            {entry.actor}
                          </span>
                        )}
                        {entry.actor && (
                          <span className="text-muted-foreground/40">—</span>
                        )}
                        <Badge
                          variant="secondary"
                          className="text-[11px] font-mono px-1.5 py-0"
                        >
                          {entry.event_type}
                        </Badge>
                        {isDenied && (
                          <Badge
                            variant="destructive"
                            className="text-[11px] px-1.5 py-0"
                          >
                            denied
                          </Badge>
                        )}
                      </div>

                      <p className="text-sm text-foreground/80 mt-0.5">
                        {describeEvent(entry.event_type)}
                        {resourceLabel && (
                          <span className="text-muted-foreground">
                            {" "}
                            · {resourceLabel}
                          </span>
                        )}
                        {details && (
                          <span className="text-muted-foreground">
                            {" "}
                            · {details}
                          </span>
                        )}
                      </p>

                      <div className="flex items-center gap-3 mt-1 text-[12px] text-muted-foreground/60">
                        <span>{projectName}</span>
                        <span>·</span>
                        <span>{relativeTime(entry.created_at)}</span>
                        {entry.ip && (
                          <>
                            <span>·</span>
                            <span className="font-mono">{entry.ip}</span>
                          </>
                        )}
                        {entry.source && (
                          <>
                            <span>·</span>
                            <span>{entry.source}</span>
                          </>
                        )}
                      </div>
                    </div>

                    {hasExtra && (
                      <button
                        className="mt-1 p-1 rounded text-muted-foreground/40 hover:text-muted-foreground transition-colors bg-transparent border-none cursor-pointer"
                        onClick={(e) => {
                          e.stopPropagation();
                          toggleExpanded(entry.id);
                        }}
                      >
                        {isExpanded ? (
                          <ChevronUp className="w-4 h-4" />
                        ) : (
                          <ChevronDown className="w-4 h-4" />
                        )}
                      </button>
                    )}
                  </div>

                  {isExpanded && hasExtra && (
                    <div className="mx-5 mb-3 ml-16 rounded-lg bg-muted/30 border border-border/30 overflow-hidden">
                      {diffKeys.length > 0 && (
                        <table className="w-full text-[12px] border-b border-border/30">
                          <thead>
                            <tr className="bg-muted/40 text-muted-foreground">
                              <th className="px-3 py-1 text-left font-medium w-[140px]">
                                field
                              </th>
                              <th className="px-3 py-1 text-left font-medium">
                                before
                              </th>
                              <th className="px-3 py-1 text-left font-medium">
                                after
                              </th>
                            </tr>
                          </thead>
                          <tbody>
                            {diffKeys.map((key) => (
                              <tr
                                key={key}
                                className="border-b border-border/20 last:border-b-0"
                              >
                                <td className="px-3 py-1.5 text-muted-foreground font-medium align-top">
                                  {key}
                                </td>
                                <td className="px-3 py-1.5 font-mono break-all text-destructive/80">
                                  {before[key] ?? "—"}
                                </td>
                                <td className="px-3 py-1.5 font-mono break-all text-emerald-600 dark:text-emerald-400">
                                  {after[key] ?? "—"}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      )}
                      {dataEntries.length > 0 && (
                        <table className="w-full text-[12px]">
                          <tbody>
                            {dataEntries.map(([key, value]) => (
                              <tr
                                key={key}
                                className="border-b border-border/20 last:border-b-0"
                              >
                                <td className="px-3 py-1.5 text-muted-foreground font-medium whitespace-nowrap align-top w-[140px]">
                                  {key}
                                </td>
                                <td className="px-3 py-1.5 text-foreground/80 font-mono break-all">
                                  {value}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      )}
                      {hasMeta && (
                        <div className="px-3 py-2 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground/70 border-t border-border/30 bg-muted/20">
                          {entry.user_agent && (
                            <span>UA: {entry.user_agent}</span>
                          )}
                          {entry.request_id && (
                            <span>req: {entry.request_id}</span>
                          )}
                          {entry.hash && (
                            <span className="font-mono" title={entry.hash}>
                              hash: {entry.hash.slice(0, 12)}…
                            </span>
                          )}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        ))}

        {hasMore && (
          <div className="flex justify-center py-4 border-t border-border/20">
            <Button
              variant="ghost"
              size="sm"
              onClick={onLoadMore}
              disabled={loading}
              className="text-muted-foreground"
            >
              {loading ? "Loading..." : "Load more events"}
            </Button>
          </div>
        )}
      </Card>
    </div>
  );
}
