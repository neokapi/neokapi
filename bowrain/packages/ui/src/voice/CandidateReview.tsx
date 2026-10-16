// A voice profile's suggested rules, as a list and a detail pane: the
// platform's twin of Kapi Desktop's What's new.
//
// The list is one line per rule in five groups: Needs you (a phrasing the team
// corrected to more than one form), Suggested, Approved (accepted by a
// reviewer, not yet promoted), Established (promoted into the workspace's
// terms) and Rejected. The decided groups start closed. The detail pane holds
// the selected rule's corrections, who decided it and when, and the acts open
// on it.
//
// The keys act on the selected row: j and k (and the arrows) move, a promotes
// a suggestion or chooses a disagreement's first form, c changes the form
// before promoting it, d rejects the phrasing, o opens the concept it denotes,
// Esc closes the change field. After a decision the selection moves on to the
// next row that waits for one, and the decided row stays dimmed until the list
// is read again.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  EmptyState,
  ErrorNotice,
  KindIcon,
  ListCapRow,
  ListDetail,
  ListDetailGroup,
  ListDetailItem,
  ListDetailList,
  LoadingSpinner,
  cn,
  useListNavigation,
} from "@neokapi/ui-primitives";
import { ChevronRight } from "../components/icons";
import type { CandidateRule } from "./types";
import {
  candidateKind,
  candidateRows,
  defaultRowKey,
  nextRowKey,
  rowCorrections,
  termKey,
  type CandidateRow,
  type CandidateRowGroup,
} from "./candidateRows";
import { CandidateLine, ConflictLine } from "./CandidateRule";
import {
  CandidateDetail,
  CandidateNothingSelected,
  DecidedMark,
  type CandidateChange,
  type NameOf,
} from "./CandidateReviewDetail";

export interface CandidateReviewProps {
  /** The profile's candidates, decided ones included; null while the first read is in flight. */
  candidates: CandidateRule[] | null;
  loading?: boolean;
  error?: unknown;
  /** The reader may decide: false hides the acts. */
  canDecide?: boolean;
  /** Take the keyboard. */
  keyboard?: boolean;
  /** The row to open on, by its key in the list. Absent opens on the first that waits. */
  defaultSelected?: string;
  /** The phrasing an act is in flight for: its row's buttons are disabled. */
  busyTerm?: string;
  /** The last act that failed, by phrasing: its row is selected again and shows why. */
  failed?: { term: string; error: unknown } | null;
  /** Promote a candidate, with another form when one is given. */
  onPromote: (candidate: CandidateRule, replacement?: string) => void | Promise<void>;
  /** Reject a phrasing. */
  onReject: (candidate: CandidateRule) => void | Promise<void>;
  /** Preview a form's blast radius. Absent hides the act. */
  onEvaluate?: (candidate: CandidateRule) => void;
  /** Open the concept a phrasing denotes. */
  onOpenConcept?: (conceptId: string) => void;
  nameOf?: NameOf;
  /** The signed-in member, whose own decisions read as "you". */
  currentUserId?: string;
  className?: string;
}

/** A group holding more rows than this draws the first ones and says how many it holds. */
const GROUP_CAP = 200;

/** The keys that move the selection from anywhere on the page. */
const MOVE_KEYS = new Set(["j", "k", "ArrowDown", "ArrowUp"]);

/** The groups that start closed, because their rows are decided. */
const CLOSED_GROUPS: ReadonlySet<CandidateRowGroup> = new Set(["promoted", "rejected"]);

const NONE: ReadonlySet<string> = new Set();

export function CandidateReview({
  candidates,
  loading,
  error,
  canDecide = true,
  keyboard = true,
  defaultSelected,
  busyTerm,
  failed,
  onPromote,
  onReject,
  onEvaluate,
  onOpenConcept,
  nameOf,
  currentUserId,
  className,
}: CandidateReviewProps) {
  const rows = useMemo(() => (candidates ? candidateRows(candidates) : []), [candidates]);
  const [opened, setOpened] = useState<ReadonlySet<CandidateRowGroup>>(() => {
    const at = rows.find((r) => r.key === defaultSelected);
    return at && CLOSED_GROUPS.has(at.group) ? new Set([at.group]) : new Set();
  });
  const isShown = useCallback(
    (group: CandidateRowGroup) => !CLOSED_GROUPS.has(group) || opened.has(group),
    [opened],
  );
  const visible = useMemo(() => {
    const out: CandidateRow[] = [];
    const counts = new Map<CandidateRowGroup, number>();
    for (const row of rows) {
      if (!isShown(row.group)) continue;
      const n = counts.get(row.group) ?? 0;
      counts.set(row.group, n + 1);
      if (n < GROUP_CAP) out.push(row);
    }
    return out;
  }, [rows, isShown]);

  // Rows decided on this screen, until the list is read again: a decided row
  // stays in the list, dimmed, and the selection passes over it.
  const [pending, setPending] = useState<{
    of: CandidateRule[] | null;
    keys: ReadonlySet<string>;
  }>({ of: null, keys: NONE });
  const decided = pending.of === candidates ? pending.keys : NONE;

  const [chosen, setChosen] = useState<string | null>(defaultSelected ?? null);
  const selectedKey =
    chosen && visible.some((r) => r.key === chosen) ? chosen : defaultRowKey(visible, decided);
  const selected = visible.find((r) => r.key === selectedKey);

  const [change, setChange] = useState<CandidateChange | null>(null);
  const activeChange = change && change.key === selectedKey ? change : null;

  // A failed act puts its row back: undimmed, selected, with the reason in
  // the detail pane.
  const failedTerm = failed ? termKey(failed.term) : undefined;
  useEffect(() => {
    if (!failedTerm) return;
    const row = rows.find((r) => termKey(r.candidate.term) === failedTerm);
    if (!row) return;
    setPending((p) => {
      const keys = new Set(p.keys);
      for (const r of rows) if (termKey(r.candidate.term) === failedTerm) keys.delete(r.key);
      return { of: p.of, keys };
    });
    setChosen(row.key);
  }, [failed, failedTerm, rows]);

  const select = useCallback((key: string) => {
    setChosen(key);
    setChange(null);
  }, []);
  const nav = useListNavigation({
    ids: visible.map((r) => r.key),
    selectedId: selectedKey,
    onSelect: select,
  });

  /** Mark a row decided and move the selection past it. */
  const advance = (from: CandidateRow) => {
    const keys = new Set([...decided, from.key]);
    setPending({ of: candidates, keys });
    setChange(null);
    setChosen(nextRowKey(visible, from.key, keys));
  };
  const promote = (row: CandidateRow, candidate: CandidateRule, replacement?: string) => {
    advance(row);
    void onPromote(candidate, replacement);
  };
  const reject = (row: CandidateRow) => {
    advance(row);
    void onReject(row.candidate);
  };
  const startChange = (row: CandidateRow, candidate: CandidateRule) =>
    setChange({ key: row.key, from: candidate.replacement, form: candidate.replacement });

  const busy = (row: CandidateRow) =>
    !!busyTerm && termKey(busyTerm) === termKey(row.candidate.term);
  const open = (row: CandidateRow) =>
    row.group === "conflict" || row.group === "suggested" || row.group === "approved";

  // The page's keys, read through a ref so the listener is bound once and
  // always acts on the row selected now.
  const container = useRef<HTMLDivElement>(null);
  const onKey = useRef<(e: KeyboardEvent) => void>(() => {});
  onKey.current = (e) => {
    if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey || e.isComposing) return;
    const el = e.target instanceof Element ? e.target : null;
    if (
      el?.closest(
        'input, textarea, select, [contenteditable="true"], [role="dialog"], [role="menu"]',
      )
    ) {
      return;
    }
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
      setChange(null);
      return;
    }
    const row = selected;
    if (!row) return;
    const decidable = canDecide && open(row) && !busy(row) && !decided.has(row.key);
    let acted = false;
    switch (e.key) {
      case "a":
        if (!decidable) break;
        promote(row, row.candidate);
        acted = true;
        break;
      case "c":
        if (!decidable) break;
        startChange(row, row.candidate);
        acted = true;
        break;
      case "d":
        if (!decidable) break;
        reject(row);
        acted = true;
        break;
      case "o": {
        const id = row.candidate.concept_id;
        if (!onOpenConcept || !id) break;
        onOpenConcept(id);
        acted = true;
        break;
      }
    }
    if (acted) e.preventDefault();
  };
  useEffect(() => {
    if (!keyboard) return;
    const listener = (e: KeyboardEvent) => onKey.current(e);
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, [keyboard]);

  if (error) {
    return (
      <div className={cn("p-6", className)}>
        <ErrorNotice error={error} title={t("The suggested rules could not be read")} />
      </div>
    );
  }
  if (!candidates) {
    return loading ? (
      <div className={cn("p-6", className)}>
        <LoadingSpinner />
      </div>
    ) : null;
  }
  if (rows.length === 0) {
    return (
      <div data-slot="voice-review-empty" className={cn("mx-auto w-full max-w-xl p-6", className)}>
        <EmptyState
          title={t("No suggested rules yet")}
          description={t(
            "When your team corrects the same phrasing again and again, in the editor or through an assistant, it is suggested here as a rule.",
          )}
        />
      </div>
    );
  }

  const inGroup = (group: CandidateRowGroup) => rows.filter((r) => r.group === group);
  const shown = (group: CandidateRowGroup) => visible.filter((r) => r.group === group);
  const row = (r: CandidateRow) => (
    <CandidateListRow key={r.key} row={r} decided={decided.has(r.key)} nameOf={nameOf} />
  );
  const toggle = (group: CandidateRowGroup) =>
    setOpened((was) => {
      const next = new Set(was);
      if (next.has(group)) next.delete(group);
      else next.add(group);
      return next;
    });

  const group = (id: CandidateRowGroup, label: string, slot: string) => {
    const all = inGroup(id);
    if (all.length === 0) return null;
    const closable = CLOSED_GROUPS.has(id);
    const isOpen = isShown(id);
    return (
      <ListDetailGroup
        key={id}
        data-slot={slot}
        data-open={closable ? isOpen || undefined : undefined}
        label={
          closable ? (
            <button
              type="button"
              data-slot={`${slot}-toggle`}
              aria-expanded={isOpen}
              onClick={() => toggle(id)}
              className="inline-flex items-center gap-1 rounded-sm hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <ChevronRight
                aria-hidden
                className={cn("size-3 transition-transform", isOpen && "rotate-90")}
              />
              {label}
            </button>
          ) : (
            label
          )
        }
        count={all.length}
      >
        {isOpen ? shown(id).map(row) : null}
        {isOpen && all.length > GROUP_CAP && (
          <ListCapRow shown={GROUP_CAP} total={all.length} noun={t("rules")} className="mx-2" />
        )}
      </ListDetailGroup>
    );
  };

  return (
    <div
      ref={container}
      data-slot="voice-review"
      className={cn("h-full min-h-0 overflow-hidden rounded-lg border border-border", className)}
    >
      <ListDetail
        list={
          <ListDetailList
            data-testid="voice-review-list"
            selectedId={selectedKey}
            onSelect={select}
            aria-label={t("Suggested rules")}
          >
            {group("conflict", t("Needs you"), "review-conflicts")}
            {group("suggested", t("Suggested"), "review-suggested")}
            {group("approved", t("Approved"), "review-approved")}
            {group("promoted", t("Established"), "review-established")}
            {group("rejected", t("Rejected"), "review-rejected")}
          </ListDetailList>
        }
        detail={
          selected ? (
            <CandidateDetail
              key={selected.key}
              row={selected}
              canDecide={canDecide && !decided.has(selected.key)}
              busy={busy(selected)}
              error={
                failed && termKey(failed.term) === termKey(selected.candidate.term)
                  ? failed.error
                  : undefined
              }
              change={activeChange}
              onPromote={(candidate, replacement) => promote(selected, candidate, replacement)}
              onReject={() => reject(selected)}
              onEvaluate={onEvaluate}
              onOpenConcept={onOpenConcept}
              onStartChange={(candidate) => startChange(selected, candidate)}
              onChangeForm={(form) => setChange((c) => (c ? { ...c, form } : c))}
              onCancelChange={() => setChange(null)}
              nameOf={nameOf}
              currentUserId={currentUserId}
            />
          ) : (
            <CandidateNothingSelected />
          )
        }
      />
    </div>
  );
}

/** One row of the list: the kind, the rule on one line, and who decided it or how often it was corrected. */
function CandidateListRow({
  row,
  decided,
  nameOf,
}: {
  row: CandidateRow;
  decided: boolean;
  nameOf?: NameOf;
}) {
  const { candidate } = row;
  const trailing =
    row.group === "promoted" || row.group === "rejected" || row.group === "approved" ? (
      <DecidedMark candidate={candidate} nameOf={nameOf} />
    ) : (
      <span
        className="text-[11px] text-muted-foreground tabular-nums"
        title={t("{count, plural, one {# correction} other {# corrections}}", {
          count: rowCorrections(row),
        })}
      >
        {rowCorrections(row)}
      </span>
    );
  return (
    <ListDetailItem
      id={row.key}
      data-slot="candidate-item"
      data-group={row.group}
      data-term={candidate.term}
      data-decided={decided || undefined}
      className={cn(decided && "opacity-50")}
      leading={<KindIcon kind={candidateKind(row)} />}
      trailing={trailing}
    >
      {row.group === "conflict" ? (
        <ConflictLine row={row} />
      ) : (
        <CandidateLine candidate={candidate} />
      )}
    </ListDetailItem>
  );
}
