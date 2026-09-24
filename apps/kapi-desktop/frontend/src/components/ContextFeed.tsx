// What a person and the agents beside them have recorded about a project's
// context, newest first.
//
// An agent records in its own process while this app is open. Both read and
// write the same workspace operation log, so a proposal appears here within a
// second of being recorded, and a decision made here is what the agent's next
// read carries. Nothing is sent between the two.
//
// Candidates carry their evidence open: the file, the unit and the quotation
// the rule came from are on the card, because a decision taken without them is
// a guess. The keys are the review session's: j and k move, a confirms, r
// discards, e edits.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Bot,
  Check,
  ChevronDown,
  ChevronRight,
  FileText,
  Inbox,
  Terminal,
  Undo2,
  User,
  Wrench,
} from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Badge,
  Button,
  CoordinateChip,
  EmptyState,
  ErrorNotice,
  Input,
  Label,
  LoadingSpinner,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  When,
  cn,
} from "@neokapi/ui-primitives";
import type { ContextFeed, ContextFeedEntry, ContextFeedGroup } from "../types/api";

/** The edit a person makes to a rule before accepting it. */
export interface ContextRuleEdit {
  replacement?: string;
  /** Whether the rule only reports; absent leaves it as proposed. */
  advisory?: boolean;
}

export interface ContextFeedProps {
  /** The operations to show, null while the first read is in flight. */
  feed: ContextFeed | null;
  loading?: boolean;
  error?: unknown;
  /** Accept a candidate, with an edit when the person made one. */
  onKeep: (entry: ContextFeedEntry, edit?: ContextRuleEdit) => void | Promise<void>;
  /** Reject a candidate. */
  onDrop: (entry: ContextFeedEntry) => void | Promise<void>;
  /** Take a rule in force back out. */
  onRevert: (entry: ContextFeedEntry) => void;
  /** Undo everything one session recorded. */
  onRevertSession: (group: ContextFeedGroup) => void;
  /** Open the widen preview for a rule in force. */
  onWiden: (entry: ContextFeedEntry, to: string) => void;
  /** Show the project each entry belongs to. The workspace feed does. */
  showProject?: boolean;
  /** Take the keyboard. The panel that is on screen does. */
  keyboard?: boolean;
}

/** The severities a rule can carry. Minor and neutral report; the rest fail. */
const OUTCOMES = ["fails", "reports"] as const;

/** What each operation kind did, for the line above the subject. */
const KIND_LABELS: Record<string, string> = {
  observe: "recorded",
  correct: "corrected",
  import: "imported",
  edit: "edited",
  keep: "kept",
  drop: "dropped",
  withdraw: "withdrawn",
  revert: "reverted",
  widen: "widened",
};

export function ContextFeedList({
  feed,
  loading,
  error,
  onKeep,
  onDrop,
  onRevert,
  onRevertSession,
  onWiden,
  showProject,
  keyboard = true,
}: ContextFeedProps) {
  const groups = useMemo(() => feed?.groups ?? [], [feed]);
  // Every candidate in view, in the order the keys walk them.
  const candidates = useMemo(
    () => groups.flatMap((group) => group.entries.filter((entry) => entry.decidable)),
    [groups],
  );
  const [cursor, setCursor] = useState(0);
  // The candidate whose edit form is open, and what has been typed into it.
  // Keyed by id so a refetch while the form is open leaves it standing.
  const [editing, setEditing] = useState<string | null>(null);
  const [edit, setEdit] = useState<ContextRuleEdit>({});
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const editRef = useRef<HTMLInputElement>(null);

  // A candidate decided elsewhere leaves the list; the cursor follows rather
  // than pointing past the end.
  useEffect(() => {
    setCursor((at) => (at >= candidates.length ? Math.max(0, candidates.length - 1) : at));
  }, [candidates.length]);
  useEffect(() => {
    if (editing && !candidates.some((entry) => entry.id === editing)) {
      setEditing(null);
      setEdit({});
    }
  }, [candidates, editing]);

  const active = candidates[cursor];

  const openEdit = useCallback((entry: ContextFeedEntry) => {
    setEditing(entry.id);
    setEdit({ replacement: entry.subject.replacement ?? "", advisory: entry.subject.advisory });
    window.setTimeout(() => editRef.current?.focus(), 0);
  }, []);

  const confirm = useCallback(
    (entry: ContextFeedEntry) => {
      const edits = editing === entry.id ? edit : undefined;
      setEditing(null);
      setEdit({});
      void onKeep(entry, edits);
    },
    [edit, editing, onKeep],
  );

  useEffect(() => {
    if (!keyboard) return;
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName ?? "";
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT") {
        if (e.key === "Escape") {
          el?.blur();
          setEditing(null);
        }
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (!active) return;
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          setCursor((at) => Math.min(at + 1, candidates.length - 1));
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          setCursor((at) => Math.max(at - 1, 0));
          break;
        case "a":
          e.preventDefault();
          confirm(active);
          break;
        case "r":
          e.preventDefault();
          setEditing(null);
          void onDrop(active);
          break;
        case "e":
          e.preventDefault();
          openEdit(active);
          break;
        case "Escape":
          setEditing(null);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, candidates.length, confirm, keyboard, onDrop, openEdit]);

  if (error) {
    return <ErrorNotice error={error} title="These recorded changes could not be read" />;
  }
  if (!feed && loading) {
    return <LoadingSpinner />;
  }
  if (groups.length === 0) {
    return (
      <EmptyState
        icon={<Inbox size={20} />}
        title="Nothing recorded yet"
        description="What you and the agents working in your projects record about terms, voice and wording appears here."
      />
    );
  }

  return (
    <div data-slot="context-feed" className="space-y-3">
      {feed?.read_only && (
        <p className="text-xs text-muted-foreground">
          This workspace is open for reading only, so decisions cannot be recorded here.
        </p>
      )}
      {groups.map((group) => (
        <SessionCard
          key={group.id}
          group={group}
          collapsed={!!collapsed[group.id]}
          onToggle={() => setCollapsed((open) => ({ ...open, [group.id]: !open[group.id] }))}
          activeID={active?.id}
          editing={editing}
          edit={edit}
          editRef={editRef}
          onEditChange={setEdit}
          onOpenEdit={openEdit}
          onCancelEdit={() => setEditing(null)}
          onKeep={confirm}
          onDrop={onDrop}
          onRevert={onRevert}
          onRevertSession={onRevertSession}
          onWiden={onWiden}
          showProject={showProject}
        />
      ))}
      {feed?.truncated && (
        <p className="text-xs text-muted-foreground">
          Older operations are not shown. Read the whole history with{" "}
          <code className="font-mono text-xs">kapi context log</code>.
        </p>
      )}
      {candidates.length > 0 && keyboard && <ShortcutLegend />}
    </div>
  );
}

/** The keys, stated where a person can see them, as the review session does. */
function ShortcutLegend() {
  return (
    <div
      data-slot="context-feed-shortcuts"
      className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground"
    >
      <span>
        <kbd className="rounded border border-border px-1">a</kbd> confirm
      </span>
      <span>
        <kbd className="rounded border border-border px-1">r</kbd> discard
      </span>
      <span>
        <kbd className="rounded border border-border px-1">e</kbd> edit
      </span>
      <span>
        <kbd className="rounded border border-border px-1">j</kbd>
        <kbd className="ml-1 rounded border border-border px-1">k</kbd> move
      </span>
    </div>
  );
}

interface SessionCardProps {
  group: ContextFeedGroup;
  collapsed: boolean;
  onToggle: () => void;
  activeID?: string;
  editing: string | null;
  edit: ContextRuleEdit;
  editRef: React.RefObject<HTMLInputElement | null>;
  onEditChange: (edit: ContextRuleEdit) => void;
  onOpenEdit: (entry: ContextFeedEntry) => void;
  onCancelEdit: () => void;
  onKeep: (entry: ContextFeedEntry) => void;
  onDrop: (entry: ContextFeedEntry) => void | Promise<void>;
  onRevert: (entry: ContextFeedEntry) => void;
  onRevertSession: (group: ContextFeedGroup) => void;
  onWiden: (entry: ContextFeedEntry, to: string) => void;
  showProject?: boolean;
}

/** One session: who worked, what it did, and the operations themselves. */
function SessionCard({
  group,
  collapsed,
  onToggle,
  activeID,
  editing,
  edit,
  editRef,
  onEditChange,
  onOpenEdit,
  onCancelEdit,
  onKeep,
  onDrop,
  onRevert,
  onRevertSession,
  onWiden,
  showProject,
}: SessionCardProps) {
  return (
    <section
      data-slot="context-feed-session"
      data-session={group.id}
      className="rounded-lg border border-border/60"
    >
      <header className="flex items-start gap-2 px-3 py-2">
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={onToggle}
          aria-label={collapsed ? t("Show these operations") : t("Hide these operations")}
          aria-expanded={!collapsed}
          className="mt-0.5 text-muted-foreground"
        >
          {collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
        </Button>
        <ActorIcon kind={group.actor.kind} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <span className="text-sm font-medium">
              <ActorName actor={group.actor} />
            </span>
            {showProject && group.project_name && (
              <span className="text-xs text-muted-foreground" translate="no">
                {group.project_name}
              </span>
            )}
            <When iso={group.last} relative className="text-xs text-muted-foreground" />
            {group.awaiting > 0 && (
              <Badge variant="secondary" data-slot="session-awaiting">
                {group.awaiting} awaiting you
              </Badge>
            )}
          </div>
          <p data-slot="session-summary" className="mt-0.5 text-xs text-muted-foreground">
            <SessionSummaryLine group={group} />
          </p>
        </div>
        {group.entries.some((entry) => entry.revertible) && group.session && (
          <Button
            variant="ghost"
            size="sm"
            data-slot="revert-session"
            className="shrink-0 text-muted-foreground"
            onClick={() => onRevertSession(group)}
          >
            <Undo2 size={13} />
            Undo session
          </Button>
        )}
      </header>

      {!collapsed && (
        <ul className="space-y-px border-t border-border/60">
          {group.entries.map((entry) => (
            <li key={entry.id}>
              <FeedEntryCard
                entry={entry}
                active={entry.id === activeID}
                editing={editing === entry.id}
                edit={edit}
                editRef={editRef}
                onEditChange={onEditChange}
                onOpenEdit={() => onOpenEdit(entry)}
                onCancelEdit={onCancelEdit}
                onKeep={() => onKeep(entry)}
                onDrop={() => void onDrop(entry)}
                onRevert={() => onRevert(entry)}
                onWiden={(to) => onWiden(entry, to)}
                showProject={showProject}
              />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** The one line a finished session reads out. */
function SessionSummaryLine({ group }: { group: ContextFeedGroup }) {
  const parts: string[] = [];
  if (group.recorded > 0) parts.push(`${group.recorded} recorded`);
  if (group.corrected > 0) parts.push(`${group.corrected} corrected`);
  if (group.kept > 0) parts.push(`${group.kept} kept`);
  if (group.dropped > 0) parts.push(`${group.dropped} dropped`);
  return (
    <>
      {parts.length > 0 ? parts.join(", ") : `${group.entries.length} operations`}
      {group.quiet ? "" : " · still working"}
    </>
  );
}

function ActorIcon({ kind }: { kind: string }) {
  const className = "mt-1 shrink-0 text-muted-foreground";
  if (kind === "agent") return <Bot size={15} className={className} />;
  if (kind === "tool") return <Wrench size={15} className={className} />;
  return <User size={15} className={className} />;
}

/** Who worked: the class of actor, its name, and the machine when known. */
function ActorName({ actor }: { actor: ContextFeedGroup["actor"] }) {
  if (actor.kind === "agent") {
    return (
      <>
        <span translate="no">{actor.name || "An agent"}</span>
        {actor.host && (
          <span className="ml-1 text-xs font-normal text-muted-foreground" translate="no">
            on {actor.host}
          </span>
        )}
      </>
    );
  }
  if (actor.kind === "tool") {
    return <span translate="no">{actor.name || "A tool"}</span>;
  }
  return actor.name ? <span translate="no">{actor.name}</span> : <>You</>;
}

interface FeedEntryCardProps {
  entry: ContextFeedEntry;
  active: boolean;
  editing: boolean;
  edit: ContextRuleEdit;
  editRef: React.RefObject<HTMLInputElement | null>;
  onEditChange: (edit: ContextRuleEdit) => void;
  onOpenEdit: () => void;
  onCancelEdit: () => void;
  onKeep: () => void;
  onDrop: () => void;
  onRevert: () => void;
  onWiden: (to: string) => void;
  showProject?: boolean;
}

/** One operation: what it was, what it was about, and what came of it. */
function FeedEntryCard({
  entry,
  active,
  editing,
  edit,
  editRef,
  onEditChange,
  onOpenEdit,
  onCancelEdit,
  onKeep,
  onDrop,
  onRevert,
  onWiden,
  showProject,
}: FeedEntryCardProps) {
  const coordinates = Object.entries(entry.scope.coordinates ?? {});
  return (
    <div
      data-slot="context-feed-entry"
      data-entry={entry.id}
      data-status={entry.status}
      {...(active ? { "data-active": "" } : {})}
      className={cn("px-3 py-2.5", active && "bg-accent/40")}
    >
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <span>{KIND_LABELS[entry.kind] ?? entry.kind}</span>
        <StatusBadge status={entry.status} kind={entry.kind} contestedBy={entry.contested_by} />
        {showProject && entry.project_name && <span translate="no">{entry.project_name}</span>}
        <When iso={entry.at} relative />
        <span className="font-mono text-[11px] text-muted-foreground/70" translate="no">
          #{entry.id}
        </span>
      </div>

      <div className="mt-1">
        <SubjectLine entry={entry} />
      </div>

      {coordinates.length > 0 && (
        <div className="mt-1.5 flex flex-wrap items-center gap-1">
          {entry.scope.level === "workspace" && (
            <Badge variant="outline" className="text-xs">
              Every project
            </Badge>
          )}
          {coordinates.map(([axis, value]) => (
            <CoordinateChip key={axis} axis={axis} value={value} />
          ))}
        </div>
      )}

      {entry.evidence.length > 0 && (
        <ul data-slot="context-evidence" className="mt-2 space-y-1">
          {entry.evidence.map((e, i) => (
            <li key={`${e.path ?? ""}:${e.unit ?? ""}:${i}`} className="text-xs">
              <div className="flex flex-wrap items-center gap-2 text-muted-foreground">
                {e.path && (
                  <span className="flex items-center gap-1">
                    <FileText size={12} className="shrink-0" />
                    <span className="font-mono" translate="no">
                      {e.path}
                    </span>
                  </span>
                )}
                {e.unit && (
                  <span className="font-mono text-[11px] text-muted-foreground/70" translate="no">
                    {e.unit}
                  </span>
                )}
              </div>
              {e.quote && (
                <blockquote
                  data-slot="context-quote"
                  className="mt-0.5 border-l-2 border-border pl-2 text-muted-foreground"
                  translate="no"
                >
                  {e.quote}
                </blockquote>
              )}
            </li>
          ))}
        </ul>
      )}

      {entry.note && (
        <p className="mt-1.5 text-xs text-muted-foreground" translate="no">
          {entry.note}
        </p>
      )}

      {editing && (
        <EditForm
          entry={entry}
          edit={edit}
          editRef={editRef}
          onChange={onEditChange}
          onCancel={onCancelEdit}
          onKeep={onKeep}
        />
      )}

      {entry.decidable && !editing && (
        <DecisionActions
          entry={entry}
          active={active}
          onKeep={onKeep}
          onDrop={onDrop}
          onOpenEdit={onOpenEdit}
        />
      )}

      {entry.revertible && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Button variant="outline" size="sm" data-slot="revert-entry" onClick={onRevert}>
            <Undo2 size={13} />
            Undo
          </Button>
          {entry.widen_to.length > 0 && <WidenPicker options={entry.widen_to} onWiden={onWiden} />}
        </div>
      )}
    </div>
  );
}

/** Confirm, edit and discard, with the keys the review session uses. */
function DecisionActions({
  entry,
  active,
  onKeep,
  onDrop,
  onOpenEdit,
}: {
  entry: ContextFeedEntry;
  active: boolean;
  onKeep: () => void;
  onDrop: () => void;
  onOpenEdit: () => void;
}) {
  if (!entry.recipe) {
    return (
      <p data-slot="context-undecidable" className="mt-2 text-xs text-muted-foreground">
        No copy of this project is on this machine, so a decision cannot be written into its files.
        Clone it anywhere and run kapi once.
      </p>
    );
  }
  return (
    <div data-slot="context-actions" className="mt-2 flex flex-wrap items-center gap-2">
      <Button size="sm" data-slot="keep-suggestion" onClick={onKeep}>
        <Check size={13} />
        Keep
        {active && <kbd className="ml-1 rounded border border-border/60 px-1 text-[10px]">a</kbd>}
      </Button>
      <Button variant="outline" size="sm" data-slot="edit-suggestion" onClick={onOpenEdit}>
        Edit
        {active && <kbd className="ml-1 rounded border border-border/60 px-1 text-[10px]">e</kbd>}
      </Button>
      <Button variant="ghost" size="sm" data-slot="drop-suggestion" onClick={onDrop}>
        Drop
        {active && <kbd className="ml-1 rounded border border-border/60 px-1 text-[10px]">r</kbd>}
      </Button>
    </div>
  );
}

/** Change what the rule says to write instead, and how hard it bites. */
function EditForm({
  entry,
  edit,
  editRef,
  onChange,
  onCancel,
  onKeep,
}: {
  entry: ContextFeedEntry;
  edit: ContextRuleEdit;
  editRef: React.RefObject<HTMLInputElement | null>;
  onChange: (edit: ContextRuleEdit) => void;
  onCancel: () => void;
  onKeep: () => void;
}) {
  return (
    <div data-slot="context-edit" className="mt-2 space-y-2 rounded-md border border-border p-2">
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-0 flex-1">
          <Label htmlFor={`replacement-${entry.id}`} className="text-xs">
            Use instead
          </Label>
          <Input
            id={`replacement-${entry.id}`}
            ref={editRef}
            value={edit.replacement ?? ""}
            onChange={(e) => onChange({ ...edit, replacement: e.target.value })}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                onKeep();
              }
            }}
            className="mt-1 font-mono"
            translate="no"
            aria-label={t("What to write instead")}
          />
        </div>
        <div className="w-40">
          <Label className="text-xs">How hard it bites</Label>
          <Select
            value={edit.advisory === undefined ? "" : edit.advisory ? "reports" : "fails"}
            onValueChange={(outcome) => onChange({ ...edit, advisory: outcome === "reports" })}
          >
            <SelectTrigger size="sm" className="mt-1 w-full" aria-label={t("Outcome")}>
              <SelectValue placeholder={t("Unchanged")} />
            </SelectTrigger>
            <SelectContent>
              {OUTCOMES.map((outcome) => (
                <SelectItem key={outcome} value={outcome}>
                  {outcome}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <p className="text-[11px] text-muted-foreground">
        A rule fails a check unless it is marked to report only.
      </p>
      <div className="flex items-center gap-2">
        <Button size="sm" data-slot="keep-edited" onClick={onKeep}>
          <Check size={13} />
          Keep with this edit
        </Button>
        <Button variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

/** Where to widen a rule in force: the workspace, or past one axis. */
function WidenPicker({ options, onWiden }: { options: string[]; onWiden: (to: string) => void }) {
  return (
    <Select value="" onValueChange={onWiden}>
      <SelectTrigger size="sm" className="w-44" data-slot="widen-picker" aria-label={t("Widen")}>
        <SelectValue placeholder={t("Widen")} />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option} value={option}>
            {option === "workspace" ? (
              "To every project"
            ) : (
              <>
                Past <span translate="no">{option}</span>
              </>
            )}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** The rule or fact an operation is about. */
function SubjectLine({ entry }: { entry: ContextFeedEntry }) {
  const { subject, correction } = entry;
  if (correction) {
    return (
      <p className="text-sm">
        <span className="font-mono line-through decoration-muted-foreground" translate="no">
          {correction.from}
        </span>{" "}
        <span className="font-mono" translate="no">
          {correction.to}
        </span>
      </p>
    );
  }
  if (subject.kind === "memory") {
    return (
      <p className="text-sm">
        <span translate="no">{subject.source}</span>
        <span className="mx-1 text-muted-foreground">into</span>
        <span translate="no">{subject.target}</span>
        {subject.target_locale && (
          <Badge variant="outline" className="ml-2 text-xs" translate="no">
            {subject.target_locale}
          </Badge>
        )}
      </p>
    );
  }
  if (subject.kind === "term") {
    return (
      <p className="flex flex-wrap items-baseline gap-2 text-sm">
        <span className="font-mono" translate="no">
          {[subject.term, ...(subject.forms ?? [])].join(", ")}
        </span>
        {subject.replacement && (
          <span className="text-muted-foreground">
            use{" "}
            <span className="font-mono text-foreground" translate="no">
              {subject.replacement}
            </span>
          </span>
        )}
        {subject.advisory && (
          <Badge variant="outline" className="text-xs" translate="no">
            reports
          </Badge>
        )}
      </p>
    );
  }
  if (subject.kind === "note") {
    return (
      <p className="text-sm" translate="no">
        {subject.text}
      </p>
    );
  }
  if (entry.target_session) {
    return (
      <p className="text-sm text-muted-foreground">
        the whole session{" "}
        <span className="font-mono" translate="no">
          {entry.target_session}
        </span>
      </p>
    );
  }
  if (entry.target) {
    return (
      <p className="text-sm text-muted-foreground">
        operation{" "}
        <span className="font-mono" translate="no">
          #{entry.target}
        </span>
      </p>
    );
  }
  return null;
}

/** What became of a subject-bearing operation. */
function StatusBadge({
  status,
  kind,
  contestedBy,
}: {
  status: string;
  kind: string;
  contestedBy?: string[];
}) {
  if (
    kind === "keep" ||
    kind === "drop" ||
    kind === "withdraw" ||
    kind === "revert" ||
    kind === "widen"
  ) {
    return null;
  }
  if (status === "contested") {
    const others = (contestedBy ?? []).map((id) => `#${id}`).join(", ");
    return (
      <Badge variant="secondary" className="text-xs">
        {others ? `Contested by ${others}` : "Contested"}
      </Badge>
    );
  }
  if (status === "suggested") {
    return (
      <Badge variant="secondary" className="text-xs">
        Awaiting you
      </Badge>
    );
  }
  if (status === "established") {
    return (
      <Badge variant="outline" className="text-xs">
        In force
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="text-xs text-muted-foreground">
      {status === "dropped" ? "Dropped" : status === "withdrawn" ? "Withdrawn" : "Undone"}
    </Badge>
  );
}

/** A count of suggestions, quiet at zero. */
export function AwaitingBadge({ count, label }: { count: number; label?: string }) {
  if (count <= 0) return null;
  return (
    <Badge variant="secondary" data-slot="awaiting-badge">
      {label ?? `${count} awaiting you`}
    </Badge>
  );
}

/** A terminal hint for a workspace with nothing recorded in it. */
export function ContextFeedHint() {
  return (
    <p className="flex items-center gap-2 text-xs text-muted-foreground">
      <Terminal size={13} className="shrink-0" />
      An agent working in one of your projects records what it learns here.
    </p>
  );
}
