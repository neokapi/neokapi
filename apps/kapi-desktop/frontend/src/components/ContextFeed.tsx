// What a person and the agents beside them have recorded about a project's
// context, as a list of sessions beside a detail pane.
//
// An agent records in its own process while this app is open. Both read and
// write the same workspace operation log, so a proposal appears here within a
// second of being recorded, and a decision made here is what the agent's next
// read carries. Nothing is sent between the two.
//
// The list has one row per session, newest first: who worked, when, and how
// many operations the session holds. The detail pane lists the selected
// session's operations and its latest check runs, newest first and one row
// each, with the reset the session offers in its toolbar: Reset to before this
// session, and Take this reset back on a session that holds a reset. A check
// run's row says what it checked and what it found, and marks a finding the
// session's next run no longer found as caught. A proposal is an operation
// that still takes a decision. It keeps Keep, Edit and Not a rule on its row,
// and the file, the unit and the quotation it came from sit in a disclosure
// under the row.
//
// The keys are the review session's: j and k (and the arrows) move between
// sessions, and a keeps, r drops and e edits the selected session's proposal,
// the first one undecided unless the person clicked another. After a decision
// the next proposal in the session takes the keys.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Check,
  ChevronRight,
  Circle,
  FilePen,
  FileText,
  History,
  Inbox,
  ListChecks,
  Maximize2,
  Pencil,
  Terminal,
  Trash2,
  Undo2,
  X,
} from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ActorMark,
  Badge,
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  CoordinateChip,
  EmptyState,
  ErrorNotice,
  Input,
  KindIcon,
  Label,
  ListDetail,
  ListDetailEmpty,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
  LoadingSpinner,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  SimpleTooltip,
  When,
  cn,
  digestKind,
  useListNavigation,
} from "@neokapi/ui-primitives";
import type {
  ContextActor,
  ContextCheckRun,
  ContextFeed,
  ContextFeedEntry,
  ContextFeedGroup,
} from "../types/api";
import { lastSegment } from "./WorkspaceProjectRow";

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
  /** Drop a suggestion, or a rule in force, which takes it back out of the project's stores. */
  onDrop: (entry: ContextFeedEntry) => void | Promise<void>;
  /** Reset the project's context to before one session. */
  onResetSession: (group: ContextFeedGroup) => void;
  /**
   * Reset the project's context to before one operation: a reset's own, which
   * brings back everything it set aside. Absent hides the action.
   */
  onResetBefore?: (entry: ContextFeedEntry) => void;
  /** Open the widen preview for a rule in force. */
  onWiden: (entry: ContextFeedEntry, to: string) => void;
  /** Show the project each session belongs to. The workspace feed does. */
  showProject?: boolean;
  /** Take the keyboard. The panel that is on screen does. */
  keyboard?: boolean;
  /** The session to open on, by its group id. Absent opens on the newest. */
  defaultSelected?: string;
}

/** The severities a rule can carry. Minor and neutral report; the rest fail. */
const OUTCOMES = ["fails", "reports"] as const;

/** The keys that move the selection from anywhere on the page. */
const MOVE_KEYS = new Set(["j", "k", "ArrowDown", "ArrowUp"]);

const NONE: ReadonlySet<string> = new Set();

/** What an operation kind did, in the active UI language. */
function kindLabel(kind: string): string {
  switch (kind) {
    case "observe":
      return t("recorded", "context operation kind");
    case "correct":
      return t("corrected", "context operation kind");
    case "import":
      return t("imported", "context operation kind");
    case "edit":
      return t("edited", "context operation kind");
    case "keep":
      return t("kept", "context operation kind");
    case "drop":
      return t("dropped", "context operation kind");
    case "withdraw":
      return t("withdrawn", "context operation kind");
    case "reset":
      return t("reset", "context operation kind");
    case "widen":
      return t("widened", "context operation kind");
    default:
      return kind;
  }
}

export function ContextFeedList({
  feed,
  loading,
  error,
  onKeep,
  onDrop,
  onResetSession,
  onResetBefore,
  onWiden,
  showProject,
  keyboard = true,
  defaultSelected,
}: ContextFeedProps) {
  const groups = useMemo(() => feed?.groups ?? [], [feed]);

  // The selected session, by id, so a refetch that brings new operations
  // leaves the same session selected. A session that is gone falls back to
  // the newest.
  const [chosen, setChosen] = useState<string | null>(defaultSelected ?? null);
  const selectedId =
    chosen && groups.some((g) => g.id === chosen) ? chosen : (groups[0]?.id ?? null);
  const selected = groups.find((g) => g.id === selectedId) ?? null;

  // Operations decided on this screen, until the feed says so itself: the row
  // dims, its buttons go, and the keys pass to the next proposal.
  const [decided, setDecided] = useState<ReadonlySet<string>>(NONE);
  const markDecided = useCallback((id: string, on: boolean) => {
    setDecided((prev) => {
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  // The proposals in the selected session still open, in the order shown.
  const candidates = useMemo(
    () => (selected?.entries ?? []).filter((e) => e.decidable && !decided.has(e.id)),
    [selected, decided],
  );
  const [picked, setPicked] = useState<string | null>(null);
  const active = candidates.find((e) => e.id === picked) ?? candidates[0];

  // The candidate whose edit form is open, and what has been typed into it.
  // Keyed by id so a refetch while the form is open leaves it standing.
  const [editing, setEditing] = useState<string | null>(null);
  const [edit, setEdit] = useState<ContextRuleEdit>({});
  const editRef = useRef<HTMLInputElement>(null);

  // A candidate decided elsewhere closes its form.
  useEffect(() => {
    if (!editing) return;
    const still = groups.some((g) => g.entries.some((e) => e.id === editing && e.decidable));
    if (!still) {
      setEditing(null);
      setEdit({});
    }
  }, [groups, editing]);

  const select = useCallback((id: string) => {
    setChosen(id);
    setPicked(null);
    setEditing(null);
  }, []);
  const nav = useListNavigation({
    ids: groups.map((g) => g.id),
    selectedId,
    onSelect: select,
  });

  const openEdit = useCallback((entry: ContextFeedEntry) => {
    setPicked(entry.id);
    setEditing(entry.id);
    setEdit({ replacement: entry.subject.replacement ?? "", advisory: entry.subject.advisory });
    window.setTimeout(() => editRef.current?.focus(), 0);
  }, []);

  const closeEdit = useCallback(() => {
    setEditing(null);
    setEdit({});
  }, []);

  const keep = useCallback(
    (entry: ContextFeedEntry) => {
      const edits = editing === entry.id ? edit : undefined;
      closeEdit();
      markDecided(entry.id, true);
      void Promise.resolve(onKeep(entry, edits)).catch(() => markDecided(entry.id, false));
    },
    [closeEdit, edit, editing, markDecided, onKeep],
  );

  const drop = useCallback(
    (entry: ContextFeedEntry) => {
      if (editing === entry.id) closeEdit();
      markDecided(entry.id, true);
      void Promise.resolve(onDrop(entry)).catch(() => markDecided(entry.id, false));
    },
    [closeEdit, editing, markDecided, onDrop],
  );

  // The page's keys, read through a ref so the listener is bound once and
  // always acts on the session and the proposal selected now.
  const container = useRef<HTMLDivElement>(null);
  const onKey = useRef<(e: KeyboardEvent) => void>(() => {});
  onKey.current = (e) => {
    if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey || e.isComposing) return;
    const el = e.target instanceof Element ? e.target : null;
    if (el?.closest('input, textarea, select, [contenteditable="true"]')) {
      if (e.key === "Escape") {
        (el as HTMLElement).blur();
        closeEdit();
      }
      return;
    }
    if (el?.closest('[role="dialog"], [role="menu"]')) return;
    // A listbox other than this list (a select's options) keeps its keys.
    const box = el?.closest('[role="listbox"]');
    if (
      box &&
      !(box.getAttribute("data-slot") === "list-detail-list" && container.current?.contains(box))
    ) {
      return;
    }
    if (MOVE_KEYS.has(e.key)) {
      if (nav.handleKey(e)) e.preventDefault();
      return;
    }
    if (e.key === "Escape") {
      closeEdit();
      return;
    }
    if (!active) return;
    switch (e.key) {
      case "a":
        e.preventDefault();
        keep(active);
        break;
      case "r":
        e.preventDefault();
        drop(active);
        break;
      case "e":
        e.preventDefault();
        openEdit(active);
        break;
    }
  };
  useEffect(() => {
    if (!keyboard) return;
    const listener = (e: KeyboardEvent) => onKey.current(e);
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, [keyboard]);

  if (error) {
    return (
      <div className="p-6">
        <ErrorNotice error={error} title="These recorded changes could not be read" />
      </div>
    );
  }
  if (!feed && loading) {
    return (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    );
  }
  if (groups.length === 0) {
    return (
      <div className="p-6">
        <EmptyState
          icon={<Inbox size={20} />}
          title="Nothing recorded yet"
          description="What you and the agents working in your projects record about terms, voice and wording appears here."
        />
      </div>
    );
  }

  const list = (
    <>
      <ListDetailList selectedId={selectedId} onSelect={select} aria-label={t("Sessions")}>
        {groups.map((group) => (
          <SessionRow key={group.id} group={group} showProject={showProject} />
        ))}
      </ListDetailList>
      {feed?.truncated && (
        <p className="px-4 pt-1 pb-3 text-[11px] text-muted-foreground">
          Older operations are not shown. Read the whole history with{" "}
          <code className="font-mono">kapi context log</code>.
        </p>
      )}
    </>
  );

  return (
    <div ref={container} data-slot="context-feed" className="h-full min-h-0">
      <ListDetail
        list={list}
        listWidth={340}
        detail={
          selected ? (
            <SessionDetail
              group={selected}
              readOnly={!!feed?.read_only}
              showProject={showProject}
              activeID={active?.id}
              decided={decided}
              editing={editing}
              edit={edit}
              editRef={editRef}
              onPick={setPicked}
              onEditChange={setEdit}
              onOpenEdit={openEdit}
              onCancelEdit={closeEdit}
              onKeep={keep}
              onDrop={drop}
              onWiden={onWiden}
              onResetSession={onResetSession}
              onResetBefore={feed?.read_only ? undefined : onResetBefore}
            />
          ) : (
            <ListDetailEmpty title={t("Choose a session to see what it recorded")} />
          )
        }
      />
    </div>
  );
}

/** One session in the list: who worked, when, and how many operations it holds. */
function SessionRow({ group, showProject }: { group: ContextFeedGroup; showProject?: boolean }) {
  return (
    <ListDetailItem
      id={group.id}
      data-slot="context-feed-session"
      data-session={group.id}
      // Rows off screen skip layout and paint, so a long history scrolls as
      // lightly as a short one. The intrinsic size keeps the scrollbar honest.
      className="[contain-intrinsic-size:auto_36px] [content-visibility:auto]"
      leading={<SessionMark actor={group.actor} />}
      trailing={
        <>
          {group.awaiting > 0 && (
            <span
              role="img"
              data-slot="session-awaiting-mark"
              aria-label={t("{count} suggested", { count: group.awaiting })}
              className="size-1.5 rounded-full bg-warning"
            />
          )}
          {group.entries.length > 0 && (
            <span className="text-[11px] text-muted-foreground tabular-nums">
              {group.entries.length}
            </span>
          )}
        </>
      }
    >
      <span data-slot="session-summary">
        <ActorName actor={group.actor} />
      </span>
      {showProject && group.project_name && (
        <span className="ml-1.5 text-[11px] text-muted-foreground" translate="no">
          {group.project_name}
        </span>
      )}
      <When iso={group.last} relative className="ml-1.5 text-[11px] text-muted-foreground" />
    </ListDetailItem>
  );
}

/** The mark for who worked a session: a person, an assistant by its host, or a kapi tool. */
export function SessionMark({ actor, size = "sm" }: { actor: ContextActor; size?: "sm" | "md" }) {
  if (actor.kind === "person") {
    return <ActorMark actor="person" name={actor.name} size={size} decorative />;
  }
  const who = actor.kind === "tool" ? "kapi" : actor.name || "agent";
  return <ActorMark actor={who} size={size} decorative />;
}

/** Who worked: the agent and the machine it ran on, a tool, or a person. */
export function ActorName({ actor }: { actor: ContextActor }) {
  if (actor.kind === "agent") {
    return (
      <>
        <span translate="no">{actor.name || t("An agent")}</span>
        {actor.host && (
          <span className="ml-1 text-muted-foreground">
            on <span translate="no">{actor.host}</span>
          </span>
        )}
      </>
    );
  }
  if (actor.kind === "tool") {
    return <span translate="no">{actor.name || t("A tool")}</span>;
  }
  return actor.name ? <span translate="no">{actor.name}</span> : <span>You</span>;
}

/** What a session's check runs found, as the feed and the Assistants page carry it. */
export type CheckOutcomes = Pick<
  ContextFeedGroup,
  "files" | "checked" | "failing" | "caught" | "clean" | "failed"
>;

/**
 * A session's check counts, one label each: the files its turns edited, its
 * runs, then "clean" when every run was clean, or what was left failing and
 * what was caught when a run failed. A session that ran no check has none.
 */
export function checkCounts(c: CheckOutcomes): string[] {
  if (!c.checked) return [];
  const parts: string[] = [];
  if (c.files > 0)
    parts.push(t("{count, plural, one {# file} other {# files}}", { count: c.files }));
  parts.push(t("{count, plural, one {# check} other {# checks}}", { count: c.checked }));
  if (c.clean) {
    parts.push(t("clean"));
  } else if (c.failed) {
    if (c.failing > 0) parts.push(t("{count} failing", { count: c.failing }));
    parts.push(t("{count} caught", { count: c.caught }));
  }
  return parts;
}

/** The one line a session's counts read out. */
function SessionCounts({ group }: { group: ContextFeedGroup }) {
  const parts: string[] = [];
  // The same labels as the Assistants page's session rows, for the same counts.
  if (group.recorded > 0)
    parts.push(
      t("{count, plural, one {# suggestion} other {# suggestions}}", { count: group.recorded }),
    );
  if (group.corrected > 0)
    parts.push(
      t("{count, plural, one {# correction} other {# corrections}}", { count: group.corrected }),
    );
  if (group.kept > 0) parts.push(t("{count} kept", { count: group.kept }));
  if (group.dropped > 0) parts.push(t("{count} dropped", { count: group.dropped }));
  const resets = group.entries.filter((entry) => entry.kind === "reset").length;
  if (resets > 0)
    parts.push(t("{count, plural, one {# reset} other {# resets}}", { count: resets }));
  const checks = checkCounts(group);
  const counts =
    parts.length > 0
      ? parts.join(", ")
      : group.entries.length > 0 || checks.length === 0
        ? t("{count, plural, one {# operation} other {# operations}}", {
            count: group.entries.length,
          })
        : "";
  return (
    <span data-slot="session-counts">
      {[counts, checks.join(" · "), group.quiet ? "" : t("still working")]
        .filter(Boolean)
        .join(" · ")}
    </span>
  );
}

interface SessionDetailProps {
  group: ContextFeedGroup;
  readOnly: boolean;
  showProject?: boolean;
  activeID?: string;
  decided: ReadonlySet<string>;
  editing: string | null;
  edit: ContextRuleEdit;
  editRef: React.RefObject<HTMLInputElement | null>;
  onPick: (id: string) => void;
  onEditChange: (edit: ContextRuleEdit) => void;
  onOpenEdit: (entry: ContextFeedEntry) => void;
  onCancelEdit: () => void;
  onKeep: (entry: ContextFeedEntry) => void;
  onDrop: (entry: ContextFeedEntry) => void;
  onWiden: (entry: ContextFeedEntry, to: string) => void;
  onResetSession: (group: ContextFeedGroup) => void;
  onResetBefore?: (entry: ContextFeedEntry) => void;
}

/** The selected session: its title and resets in the toolbar, its operations below. */
function SessionDetail({
  group,
  readOnly,
  showProject,
  activeID,
  decided,
  editing,
  edit,
  editRef,
  onPick,
  onEditChange,
  onOpenEdit,
  onCancelEdit,
  onKeep,
  onDrop,
  onWiden,
  onResetSession,
  onResetBefore,
}: SessionDetailProps) {
  const canReset = !!group.session && group.entries.some((entry) => entry.droppable);
  // The newest reset in the session is the one taking it back undoes.
  const reset = onResetBefore ? group.entries.find((entry) => entry.kind === "reset") : undefined;
  const noCheckout = group.entries.some((entry) => entry.decidable && !entry.recipe);

  return (
    <ListDetailPane
      data-slot="context-feed-detail"
      data-session={group.id}
      toolbar={
        <ListDetailToolbar
          title={
            <span className="flex min-w-0 items-center gap-2">
              <SessionMark actor={group.actor} size="md" />
              <span className="min-w-0 truncate">
                <ActorName actor={group.actor} />
              </span>
              {group.awaiting > 0 && (
                <Badge variant="secondary" data-slot="session-awaiting">
                  {t("{count} suggested", { count: group.awaiting })}
                </Badge>
              )}
            </span>
          }
          actions={
            (reset || canReset) && (
              <>
                {reset && onResetBefore && (
                  <Button
                    variant="outline"
                    size="sm"
                    data-slot="reset-reset"
                    onClick={() => onResetBefore(reset)}
                  >
                    <Undo2 size={13} />
                    Take this reset back
                  </Button>
                )}
                {canReset && (
                  <Button
                    variant="warning"
                    size="sm"
                    data-slot="reset-session"
                    onClick={() => onResetSession(group)}
                  >
                    <History size={13} />
                    Reset to before this session
                  </Button>
                )}
              </>
            )
          }
        />
      }
    >
      <p className="-mt-1.5 text-[11px] text-muted-foreground">
        <SessionCounts group={group} />
        {showProject && group.project_name && (
          <>
            {" · "}
            <span translate="no">{group.project_name}</span>
          </>
        )}
        {" · "}
        <When iso={group.last} relative />
      </p>
      {readOnly && (
        <p className="text-[11px] text-muted-foreground">
          This workspace is open for reading only, so decisions cannot be recorded here.
        </p>
      )}
      {noCheckout && (
        <p data-slot="context-no-checkout" className="text-[11px] text-muted-foreground">
          No copy of this project is on this machine. A decision is recorded in the workspace, where
          the project's terms and content memory live.
        </p>
      )}
      <ul data-slot="context-feed-entries" className="-mx-2 flex flex-col gap-px">
        {sessionRows(group).map((row) =>
          row.kind === "run" ? (
            <li key={`run:${row.run.id}`}>
              <CheckRunRow run={row.run} />
            </li>
          ) : (
            <li key={row.entry.id}>
              <FeedEntryRow
                entry={row.entry}
                groupProject={group.project_key}
                active={row.entry.id === activeID}
                decided={decided.has(row.entry.id)}
                editing={editing === row.entry.id}
                edit={edit}
                editRef={editRef}
                onPick={() => row.entry.decidable && onPick(row.entry.id)}
                onEditChange={onEditChange}
                onOpenEdit={() => onOpenEdit(row.entry)}
                onCancelEdit={onCancelEdit}
                onKeep={() => onKeep(row.entry)}
                onDrop={() => onDrop(row.entry)}
                onWiden={(to) => onWiden(row.entry, to)}
              />
            </li>
          ),
        )}
      </ul>
    </ListDetailPane>
  );
}

/** One row of a session's detail: an operation it recorded, or a check run. */
export type SessionDetailRow =
  | { kind: "entry"; at: string; entry: ContextFeedEntry }
  | { kind: "run"; at: string; run: ContextCheckRun };

/**
 * A session's operations and check runs in one list, newest first. Both
 * arrive newest first with their times in one format, and the sort is
 * stable, so an operation and a run at the same second keep the operation
 * above.
 */
export function sessionRows(group: Pick<ContextFeedGroup, "entries" | "runs">): SessionDetailRow[] {
  const rows: SessionDetailRow[] = [
    ...group.entries.map((entry) => ({ kind: "entry" as const, at: entry.at, entry })),
    ...(group.runs ?? []).map((run) => ({ kind: "run" as const, at: run.at, run })),
  ];
  return rows.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));
}

/** What a check run found: clean, how many findings failed, or that it did not run. */
function runResult(run: ContextCheckRun): string {
  if (!run.reached) return t("did not run", "check run result");
  if (run.clean) return t("clean");
  return t("{count} failing", { count: run.failing });
}

/**
 * One check run: its hook's mark, what it found and the rules that failed on
 * the first line, then what it checked and when, then its failing findings,
 * each marked caught when the session's next run on the file no longer found
 * it. The edit hook's run checked the one file an edit wrote; the Stop hook's
 * ran the ship gates as the turn ended.
 */
function CheckRunRow({ run }: { run: ContextCheckRun }) {
  const onEdit = run.hook === "edit";
  const label = onEdit
    ? t("checked on edit", "context operation kind")
    : t("checked", "context operation kind");
  const Mark = onEdit ? FilePen : ListChecks;
  const more = run.failing - run.findings.length;
  const unlisted = t("{count} more", { count: more });
  return (
    <div
      data-slot="context-check-run"
      data-run={run.id}
      data-result={!run.reached ? "did-not-run" : run.clean ? "clean" : "failing"}
      data-hook={run.hook}
      className="rounded-md px-2 py-2"
    >
      <div className="flex min-w-0 items-start gap-2">
        <span className="mt-0.5 flex w-4 shrink-0 justify-center">
          <SimpleTooltip content={label}>
            <span
              role="img"
              aria-label={label}
              data-slot="operation-mark"
              className="inline-flex text-muted-foreground"
            >
              <Mark aria-hidden className="size-3.5" />
            </span>
          </SimpleTooltip>
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm" data-slot="check-run-result">
            {runResult(run)}
            {run.reached && !run.clean && run.rules.length > 0 && (
              <span className="ml-1.5 font-mono text-xs text-muted-foreground" translate="no">
                {run.rules.join(", ")}
              </span>
            )}
          </div>
          <div className="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted-foreground">
            <span>{label}</span>
            <RunFiles run={run} />
            <span aria-hidden>·</span>
            <When iso={run.at} relative />
          </div>
          {run.findings.length > 0 && (
            <ul data-slot="check-run-findings" className="mt-1.5 flex flex-col gap-0.5">
              {run.findings.map((f, i) => (
                <li
                  // A run can report one wording twice in a file, so the
                  // position keeps the two rows apart.
                  key={i}
                  data-slot="check-run-finding"
                  {...(f.caught ? { "data-caught": "" } : {})}
                  className="flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground"
                >
                  {f.quote ? (
                    <span className="min-w-0 truncate text-foreground" translate="no">
                      “{f.quote}”
                    </span>
                  ) : (
                    f.message && <span className="min-w-0 truncate">{f.message}</span>
                  )}
                  <span className="shrink-0 font-mono" translate="no">
                    {f.rule}
                  </span>
                  {f.file && (
                    <SimpleTooltip content={<span translate="no">{f.file}</span>}>
                      <span className="shrink-0 font-mono" translate="no">
                        {lastSegment(f.file)}
                      </span>
                    </SimpleTooltip>
                  )}
                  {f.caught && (
                    <span
                      data-slot="finding-caught"
                      className="inline-flex shrink-0 items-center gap-0.5 text-foreground"
                    >
                      <Check size={11} aria-hidden className="text-success" />
                      {t("caught")}
                    </span>
                  )}
                </li>
              ))}
              {more > 0 && <li className="text-[11px] text-muted-foreground">{unlisted}</li>}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * What a run checked: the files the session's turn edited, by their last
 * segment with the full paths on hover, or how many files the check read when
 * the run knows no edit.
 */
function RunFiles({ run }: { run: ContextCheckRun }) {
  if (run.edited && run.files.length > 0) {
    const shown = run.files.slice(0, 2);
    const rest = run.file_count - shown.length;
    return (
      <>
        <span aria-hidden>·</span>
        <SimpleTooltip
          content={
            <span className="font-mono" translate="no">
              {run.files.join(", ")}
            </span>
          }
        >
          <span data-slot="check-run-files" className="min-w-0 truncate">
            <span className="font-mono" translate="no">
              {shown.map(lastSegment).join(", ")}
            </span>
            {rest > 0 && <span className="ml-1">{t("{count} more", { count: rest })}</span>}
          </span>
        </SimpleTooltip>
      </>
    );
  }
  if (run.file_count <= 0) return null;
  const read = t("{count, plural, one {# file} other {# files}}", { count: run.file_count });
  return (
    <>
      <span aria-hidden>·</span>
      <span data-slot="check-run-files">{read}</span>
    </>
  );
}

interface FeedEntryRowProps {
  entry: ContextFeedEntry;
  /** The project the session belongs to; an operation elsewhere names its own. */
  groupProject?: string;
  active: boolean;
  decided: boolean;
  editing: boolean;
  edit: ContextRuleEdit;
  editRef: React.RefObject<HTMLInputElement | null>;
  onPick: () => void;
  onEditChange: (edit: ContextRuleEdit) => void;
  onOpenEdit: () => void;
  onCancelEdit: () => void;
  onKeep: () => void;
  onDrop: () => void;
  onWiden: (to: string) => void;
}

/** One operation: its kind, what it was about, and the decisions open on it. */
function FeedEntryRow({
  entry,
  groupProject,
  active,
  decided,
  editing,
  edit,
  editRef,
  onPick,
  onEditChange,
  onOpenEdit,
  onCancelEdit,
  onKeep,
  onDrop,
  onWiden,
}: FeedEntryRowProps) {
  const otherProject = entry.project_key !== groupProject ? entry.project_name : undefined;
  return (
    <div
      data-slot="context-feed-entry"
      data-entry={entry.id}
      data-status={entry.status}
      {...(active ? { "data-active": "" } : {})}
      {...(decided ? { "data-decided": "" } : {})}
      className={cn("rounded-md px-2 py-2", active && "bg-accent/40", decided && "opacity-60")}
      onClick={onPick}
    >
      <div className="flex flex-wrap items-start gap-x-3 gap-y-1.5">
        <div className="flex min-w-0 flex-1 items-start gap-2">
          <span className="mt-0.5 flex w-4 shrink-0 justify-center">
            <OperationMark entry={entry} />
          </span>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm">
              <SubjectLine entry={entry} />
            </div>
            <div className="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted-foreground">
              <span>{kindLabel(entry.kind)}</span>
              <StatusWord
                status={entry.status}
                kind={entry.kind}
                contestedBy={entry.contested_by}
              />
              {otherProject && (
                <>
                  <span aria-hidden>·</span>
                  <span translate="no">{otherProject}</span>
                </>
              )}
              <span aria-hidden>·</span>
              <When iso={entry.at} relative />
            </div>
          </div>
        </div>
        {!editing && !decided && (
          <RowActions
            entry={entry}
            active={active}
            onKeep={onKeep}
            onDrop={onDrop}
            onOpenEdit={onOpenEdit}
            onWiden={onWiden}
          />
        )}
      </div>

      <EntryDetails entry={entry} />

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
    </div>
  );
}

/** The decisions open on one operation: a proposal's three, or a rule's two. */
function RowActions({
  entry,
  active,
  onKeep,
  onDrop,
  onOpenEdit,
  onWiden,
}: {
  entry: ContextFeedEntry;
  active: boolean;
  onKeep: () => void;
  onDrop: () => void;
  onOpenEdit: () => void;
  onWiden: (to: string) => void;
}) {
  if (entry.decidable) {
    return (
      <div data-slot="context-actions" className="flex shrink-0 items-center gap-1.5">
        <Button
          size="sm"
          variant="success"
          data-slot="keep-suggestion"
          aria-keyshortcuts={active ? "a" : undefined}
          onClick={onKeep}
        >
          <Check size={13} />
          Keep
          {active && <Shortcut k="a" />}
        </Button>
        <Button
          variant="outline"
          size="sm"
          data-slot="edit-suggestion"
          aria-keyshortcuts={active ? "e" : undefined}
          onClick={onOpenEdit}
        >
          Edit
          {active && <Shortcut k="e" />}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          data-slot="drop-suggestion"
          aria-keyshortcuts={active ? "r" : undefined}
          onClick={onDrop}
        >
          Not a rule
          {active && <Shortcut k="r" />}
        </Button>
      </div>
    );
  }
  if (entry.droppable) {
    return (
      <div className="flex shrink-0 items-center gap-1.5">
        <Button variant="outline" size="sm" data-slot="drop-rule" onClick={onDrop}>
          <Trash2 size={13} />
          Drop
        </Button>
        {entry.widen_to.length > 0 && <WidenPicker options={entry.widen_to} onWiden={onWiden} />}
      </div>
    );
  }
  return null;
}

/** A key, shown on its button while the button is hovered or focused. */
function Shortcut({ k }: { k: string }) {
  return (
    <kbd
      aria-hidden
      translate="no"
      className="ml-1 hidden rounded border border-current/30 px-1 font-mono text-[10px] leading-4 group-hover/button:inline-block group-focus-visible/button:inline-block"
    >
      {k}
    </kbd>
  );
}

/**
 * Where an operation was seen, the point it answers at, and the counts behind
 * it, behind one disclosure under the row. The id the command line takes sits
 * at its foot.
 */
function EntryDetails({ entry }: { entry: ContextFeedEntry }) {
  const coordinates = Object.entries(entry.scope.coordinates ?? {});
  const hasDetails =
    entry.evidence.length > 0 || coordinates.length > 0 || !!entry.note || !!entry.standing;
  if (!hasDetails) return null;
  return (
    <Collapsible className="mt-1 pl-6">
      <CollapsibleTrigger
        data-slot="context-details-toggle"
        className="group/details flex items-center gap-1 rounded text-[11px] text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
        onClick={(e) => e.stopPropagation()}
      >
        <ChevronRight
          size={12}
          aria-hidden
          className="transition-transform group-data-[state=open]/details:rotate-90"
        />
        {entry.evidence.length > 0 ? (
          <span>
            {t("{count, plural, one {Seen in # place} other {Seen in # places}}", {
              count: entry.evidence.length,
            })}
          </span>
        ) : (
          <span>Details</span>
        )}
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-1.5 space-y-2">
        {entry.evidence.length > 0 && (
          <ul data-slot="context-evidence" className="space-y-1.5">
            {entry.evidence.map((e, i) => (
              <li key={`${e.path ?? ""}:${e.unit ?? ""}:${i}`} className="text-xs">
                <div className="flex flex-wrap items-center gap-2 text-muted-foreground">
                  {e.path && (
                    <SimpleTooltip content={e.path}>
                      <span className="flex min-w-0 items-center gap-1">
                        <FileText size={12} className="shrink-0" />
                        <span className="truncate font-mono" translate="no">
                          {e.path}
                        </span>
                      </span>
                    </SimpleTooltip>
                  )}
                  {e.unit && (
                    <span className="font-mono text-[10px] text-muted-foreground/70" translate="no">
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
        {coordinates.length > 0 && (
          <div className="flex flex-wrap items-center gap-1">
            {entry.scope.level === "workspace" && (
              <Badge variant="outline" className="text-[11px]">
                Every project
              </Badge>
            )}
            {coordinates.map(([axis, value]) => (
              <CoordinateChip key={axis} axis={axis} value={value} />
            ))}
          </div>
        )}
        {entry.note && (
          <p className="text-xs text-muted-foreground" translate="no">
            {entry.note}
          </p>
        )}
        {entry.standing && (
          <p className="text-xs text-muted-foreground tabular-nums" translate="no">
            {entry.standing}
          </p>
        )}
        <p className="font-mono text-[10px] text-muted-foreground/70" translate="no">
          #{entry.short}
        </p>
      </CollapsibleContent>
    </Collapsible>
  );
}

/** The icon for each operation that carries no subject of its own. */
const OPERATION_ICONS: Record<string, React.ComponentType<{ className?: string }>> = {
  reset: Undo2,
  keep: Check,
  establish: Check,
  drop: X,
  withdraw: X,
  widen: Maximize2,
  edit: Pencil,
};

/**
 * What an operation is about, as an icon: the digest's kind for one that
 * carries a subject, and the act itself for one that only acts on another
 * (a keep, a drop, a reset).
 */
function OperationMark({ entry }: { entry: ContextFeedEntry }) {
  if (entry.subject.kind) return <KindIcon kind={digestKind(entry)} />;
  const Icon = OPERATION_ICONS[entry.kind] ?? Circle;
  const label = kindLabel(entry.kind);
  return (
    <SimpleTooltip content={label}>
      <span
        role="img"
        aria-label={label}
        data-slot="operation-mark"
        className="inline-flex text-muted-foreground"
      >
        <Icon aria-hidden className="size-3.5" />
      </span>
    </SimpleTooltip>
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
    <div
      data-slot="context-edit"
      className="mt-2 ml-6 space-y-2 rounded-md border border-border p-2"
      onClick={(e) => e.stopPropagation()}
    >
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
        <Button size="sm" variant="success" data-slot="keep-edited" onClick={onKeep}>
          <Check size={13} />
          Keep with this edit
        </Button>
        <Button variant="ghost" size="sm" onClick={onCancel} aria-keyshortcuts="Escape">
          Cancel
        </Button>
      </div>
    </div>
  );
}

/**
 * Where to widen a rule in force: the workspace, or past one axis. The label
 * names the action where the picker sits; History says "Widen".
 */
export function WidenPicker({
  options,
  onWiden,
  label = t("Widen"),
}: {
  options: string[];
  onWiden: (to: string) => void;
  label?: string;
}) {
  return (
    <Select value="" onValueChange={onWiden}>
      <SelectTrigger size="sm" className="w-44" data-slot="widen-picker" aria-label={label}>
        <SelectValue placeholder={label} />
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

/** The rule or fact an operation is about, on one line. */
function SubjectLine({ entry }: { entry: ContextFeedEntry }) {
  const { subject, correction } = entry;
  if (correction) {
    return (
      <>
        <span className="font-mono line-through decoration-muted-foreground" translate="no">
          {correction.from}
        </span>{" "}
        <span className="font-mono" translate="no">
          {correction.to}
        </span>
      </>
    );
  }
  if (subject.kind === "memory") {
    return (
      <>
        <span translate="no">{subject.source}</span>
        <span className="mx-1 text-muted-foreground">into</span>
        <span translate="no">{subject.target}</span>
        {subject.target_locale && (
          <Badge variant="outline" className="ml-2 font-mono text-[11px]" translate="no">
            {subject.target_locale}
          </Badge>
        )}
      </>
    );
  }
  if (subject.kind === "term") {
    return (
      <>
        <span className="font-mono" translate="no">
          {[subject.term, ...(subject.forms ?? [])].join(", ")}
        </span>
        {subject.replacement && (
          <span className="ml-2 text-muted-foreground">
            use{" "}
            <span className="font-mono text-foreground" translate="no">
              {subject.replacement}
            </span>
          </span>
        )}
        {subject.advisory && (
          <Badge variant="outline" className="ml-2 text-[11px]" translate="no">
            reports
          </Badge>
        )}
      </>
    );
  }
  if (subject.kind === "note") {
    return <span translate="no">{subject.text}</span>;
  }
  if (entry.kind === "reset" && entry.before) {
    return (
      <span className="text-muted-foreground">
        back to before{" "}
        <span className="font-mono" translate="no">
          #{entry.before}
        </span>
      </span>
    );
  }
  if (entry.target) {
    return (
      <span className="text-muted-foreground">
        operation{" "}
        <span className="font-mono" translate="no">
          #{entry.target}
        </span>
      </span>
    );
  }
  return null;
}

/** What became of a subject-bearing operation, as a word in a badge. */
function StatusWord({
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
    kind === "reset" ||
    kind === "widen"
  ) {
    return null;
  }
  let word: React.ReactNode;
  let variant: "secondary" | "outline" = "outline";
  if (status === "contested") {
    const others = (contestedBy ?? []).map((id) => `#${id}`).join(", ");
    word = others ? <>Contested by {others}</> : <>Contested</>;
    variant = "secondary";
  } else if (status === "suggested") {
    word = <>Suggested</>;
    variant = "secondary";
  } else if (status === "established") {
    word = <>In force</>;
  } else if (status === "dropped") {
    word = <>Dropped</>;
  } else if (status === "withdrawn") {
    word = <>Withdrawn</>;
  } else {
    word = <>Set aside</>;
  }
  return (
    <Badge
      variant={variant}
      className={cn("h-4 px-1.5 text-[11px]", variant === "outline" && "text-muted-foreground")}
    >
      {word}
    </Badge>
  );
}

/** A terminal hint for a workspace with nothing recorded in it. */
export function ContextFeedHint() {
  return (
    <p className="flex items-center gap-2 text-xs text-muted-foreground">
      <Terminal size={13} className="shrink-0" />
      An agent working in one of your projects records what it notices here.
    </p>
  );
}
