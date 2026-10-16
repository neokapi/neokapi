// What's new for a project: the rules and suggestions kapi holds for it, as a
// list and a detail pane.
//
// The list is one line per item in five groups: Needs you (each
// disagreement), Suggested (every suggestion, theme by theme), Established
// (rules that came into force since the person last looked), Earlier (rules
// in force from before that, collapsed) and Drift. The detail pane holds the
// selected item's evidence, who noticed it and when, and the decisions open
// on it. A suggestion nobody answers keeps advising, so nothing here counts
// unread work and nothing has to be cleared.
//
// The keys act on the selected row: j and k (and the arrows) move, a keeps a
// suggestion or chooses a disagreement's first side, c changes the rule
// before keeping it, d drops a suggestion or a rule in force, g keeps every
// suggestion that shares the selected one's theme and collection, o opens the
// file the item was seen in, Esc closes the change field. After a decision
// the selection moves on to the next row that waits for one, and the decided
// row stays dimmed until the digest is read again.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  EmptyState,
  ErrorNotice,
  KindIcon,
  ListDetail,
  ListDetailGroup,
  ListDetailItem,
  ListDetailList,
  LoadingSpinner,
  cn,
  digestKind,
  useListNavigation,
} from "@neokapi/ui-primitives";
import type { ContextDigest, DigestItem } from "../types/api";
import { defaultRowKey, digestRows, nextRowKey, type DigestRow } from "../lib/digestRows";
import { ConflictLine, RuleLine, themeLabel } from "./DigestRule";
import {
  DigestDetail,
  DigestNothingSelected,
  NoticedMark,
  type DigestChange,
} from "./ContextDigestDetail";

export { RuleSentence } from "./DigestRule";

export interface ContextDigestViewProps {
  /** The digest, null while the first read is in flight. */
  digest: ContextDigest | null;
  loading?: boolean;
  error?: unknown;
  /**
   * When the person last looked, as it stood when they arrived. Absent when
   * they never had.
   */
  since?: string;
  /** Decisions can be written: a checkout of the project is on this machine. */
  canDecide?: boolean;
  /** Take the keyboard. */
  keyboard?: boolean;
  /**
   * The row to open on, by its key in the list (`suggested:<id>`,
   * `established:<id>`, …). Absent opens on the first row that needs a
   * person, else the first suggestion.
   */
  defaultSelected?: string;
  /** Keep a suggestion, changing what it says to write when a form is given. */
  onKeep: (item: DigestItem, replacement?: string) => void | Promise<void>;
  /** Keep every suggestion of one theme and collection. */
  onKeepGroup: (items: DigestItem[]) => void | Promise<void>;
  onDrop: (item: DigestItem) => void | Promise<void>;
  /** Choose one side of a conflict, setting the others aside. */
  onChoose: (item: DigestItem, replacement?: string) => void | Promise<void>;
  /**
   * Apply a rule in force more widely: "workspace", or the axis it stops
   * being specific about. Absent hides the action.
   */
  onWiden?: (item: DigestItem, to: string) => void;
  /** Open the file an item was seen in. Absent hides the links. */
  onOpenFile?: (path: string) => void;
  /** Open the project's Assistant section. Absent leaves the empty state without its button. */
  onConnectAssistant?: () => void;
}

/** A group holding more of these suggestions is split under a header per theme. */
const THEME_HEADERS_ABOVE = 12;

/** The keys that move the selection from anywhere on the page. */
const MOVE_KEYS = new Set(["j", "k", "ArrowDown", "ArrowUp"]);

const NONE: ReadonlySet<string> = new Set();

export function ContextDigestView({
  digest,
  loading,
  error,
  since,
  canDecide = true,
  keyboard = true,
  defaultSelected,
  onKeep,
  onKeepGroup,
  onDrop,
  onChoose,
  onWiden,
  onOpenFile,
  onConnectAssistant,
}: ContextDigestViewProps) {
  const rows = useMemo(() => (digest ? digestRows(digest) : []), [digest]);
  const [earlierOpen, setEarlierOpen] = useState(false);
  const visible = useMemo(
    () => (earlierOpen ? rows : rows.filter((r) => r.group !== "earlier")),
    [rows, earlierOpen],
  );

  // Rows decided on this screen, until the digest is read again: a decided
  // row stays in the list, dimmed, and the selection passes over it.
  const [pending, setPending] = useState<{ of: ContextDigest | null; keys: ReadonlySet<string> }>({
    of: null,
    keys: NONE,
  });
  const decided = pending.of === digest ? pending.keys : NONE;

  const [chosen, setChosen] = useState<string | null>(defaultSelected ?? null);
  const selectedKey =
    chosen && visible.some((r) => r.key === chosen) ? chosen : defaultRowKey(visible, decided);
  const selected = visible.find((r) => r.key === selectedKey);

  const [change, setChange] = useState<DigestChange | null>(null);
  const activeChange = change && change.key === selectedKey ? change : null;

  const select = useCallback((key: string) => {
    setChosen(key);
    setChange(null);
  }, []);
  const nav = useListNavigation({
    ids: visible.map((r) => r.key),
    selectedId: selectedKey,
    onSelect: select,
  });

  /** Mark rows decided and move the selection past them. */
  const advance = (from: DigestRow, also: string[] = []) => {
    const keys = new Set([...decided, from.key, ...also]);
    setPending({ of: digest, keys });
    setChange(null);
    setChosen(nextRowKey(visible, from.key, keys));
  };
  const keep = (row: DigestRow, replacement?: string) => {
    advance(row);
    void onKeep(row.item, replacement);
  };
  const choose = (row: DigestRow, side: DigestItem, replacement?: string) => {
    advance(row);
    void onChoose(side, replacement);
  };
  const drop = (row: DigestRow, item: DigestItem) => {
    advance(row);
    void onDrop(item);
  };
  const keepGroup = (row: DigestRow) => {
    const items = (row.siblings ?? []).filter((s) => s.keepable);
    if (items.length === 0) return false;
    const ids = new Set(items.map((i) => i.id));
    const keys = rows
      .filter((r) => r.group === "suggested" && ids.has(r.item.id))
      .map((r) => r.key);
    advance(row, keys);
    void onKeepGroup(items);
    return true;
  };
  const startChange = (row: DigestRow, item: DigestItem) =>
    setChange({ key: row.key, itemId: item.id, form: item.subject.term?.replacement ?? "" });

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
    const first = row.conflict?.sides[0];
    const subject =
      row.group === "conflict" ? first : row.group === "suggested" ? row.item : undefined;
    let acted = false;
    switch (e.key) {
      case "a":
        if (!canDecide || !subject?.keepable) break;
        if (row.group === "conflict") choose(row, subject);
        else keep(row);
        acted = true;
        break;
      case "c":
        if (!canDecide || !subject?.keepable || !subject.subject.term) break;
        startChange(row, subject);
        acted = true;
        break;
      case "d": {
        if (!canDecide) break;
        // A disagreement with a rival offers no Drop: choosing one side sets the others aside.
        const target =
          row.group === "conflict"
            ? row.conflict!.sides.length === 1
              ? first
              : undefined
            : row.item;
        if (!target?.droppable) break;
        drop(row, target);
        acted = true;
        break;
      }
      case "g":
        if (canDecide && row.group === "suggested") acted = keepGroup(row);
        break;
      case "o": {
        const path = (first ?? row.item).quote?.path;
        if (!onOpenFile || !path) break;
        onOpenFile(path);
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
      <div className="p-6">
        <ErrorNotice error={error} title="The rules and suggestions could not be read" />
      </div>
    );
  }
  if (!digest) {
    return loading ? (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    ) : null;
  }
  if (rows.length === 0) {
    return (
      <div className="mx-auto max-w-xl p-6">
        <DigestEmpty digest={digest} since={since} onConnectAssistant={onConnectAssistant} />
      </div>
    );
  }

  const inGroup = (group: DigestRow["group"]) => visible.filter((r) => r.group === group);
  const earlier = rows.filter((r) => r.group === "earlier");
  const row = (r: DigestRow) => <DigestListRow key={r.key} row={r} decided={decided.has(r.key)} />;

  return (
    <div ref={container} data-slot="context-digest" className="h-full min-h-0">
      <ListDetail
        list={
          <>
            <DigestCaption digest={digest} since={since} />
            <ListDetailList selectedId={selectedKey} onSelect={select} aria-label={t("What's new")}>
              {inGroup("conflict").length > 0 && (
                <ListDetailGroup
                  data-slot="digest-conflicts"
                  label={t("Needs you")}
                  count={inGroup("conflict").length}
                >
                  {inGroup("conflict").map(row)}
                </ListDetailGroup>
              )}
              {inGroup("suggested").length > 0 && (
                <ListDetailGroup
                  data-slot="digest-suggested"
                  label={t("Suggested")}
                  count={inGroup("suggested").length}
                >
                  {inGroup("suggested").length > THEME_HEADERS_ABOVE
                    ? digest.suggested.map((theme) => {
                        const ofTheme = inGroup("suggested").filter(
                          (r) => r.item.theme === theme.theme,
                        );
                        if (ofTheme.length === 0) return null;
                        return [
                          <ListDetailGroup
                            key={`theme:${theme.theme}`}
                            data-slot="digest-theme"
                            label={themeLabel(theme.theme)}
                            count={ofTheme.length}
                            className="top-[30px] font-medium"
                          />,
                          ...ofTheme.map(row),
                        ];
                      })
                    : inGroup("suggested").map(row)}
                </ListDetailGroup>
              )}
              {inGroup("established").length > 0 && (
                <ListDetailGroup
                  data-slot="digest-established"
                  label={t("Established")}
                  count={inGroup("established").length}
                >
                  {inGroup("established").map(row)}
                </ListDetailGroup>
              )}
              {earlier.length > 0 && (
                <ListDetailGroup
                  data-slot="digest-earlier"
                  data-open={earlierOpen || undefined}
                  label={
                    <button
                      type="button"
                      data-slot="digest-earlier-toggle"
                      aria-expanded={earlierOpen}
                      onClick={() => setEarlierOpen((open) => !open)}
                      className="inline-flex items-center gap-1 rounded-sm hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
                    >
                      <ChevronRight
                        aria-hidden
                        className={cn("size-3 transition-transform", earlierOpen && "rotate-90")}
                      />
                      Earlier
                    </button>
                  }
                  count={earlier.length}
                >
                  {earlierOpen ? earlier.map(row) : null}
                </ListDetailGroup>
              )}
              {inGroup("drift").length > 0 && (
                <ListDetailGroup
                  data-slot="digest-drift"
                  label={t("Drift")}
                  count={inGroup("drift").length}
                >
                  {inGroup("drift").map(row)}
                </ListDetailGroup>
              )}
            </ListDetailList>
          </>
        }
        detail={
          selected ? (
            <DigestDetail
              key={selected.key}
              row={selected}
              canDecide={canDecide}
              change={activeChange}
              onKeep={(replacement) => keep(selected, replacement)}
              onChoose={(side, replacement) => choose(selected, side, replacement)}
              onDrop={(item) => drop(selected, item)}
              onWiden={onWiden ? (to) => onWiden(selected.item, to) : undefined}
              onOpenFile={onOpenFile}
              onStartChange={(item) => startChange(selected, item)}
              onChangeForm={(form) => setChange((c) => (c ? { ...c, form } : c))}
              onCancelChange={() => setChange(null)}
            />
          ) : (
            <DigestNothingSelected />
          )
        }
      />
    </div>
  );
}

/** One row of the list: what kind of thing it is, the rule on one line, and who noticed it. */
function DigestListRow({ row, decided }: { row: DigestRow; decided: boolean }) {
  const { item } = row;
  let trailing: React.ReactNode;
  if (row.conflict) {
    trailing = row.conflict.sides.map((side) => (
      <NoticedMark key={side.id} who={side.noticed_by} />
    ));
  } else if (row.group === "established" || row.group === "earlier") {
    trailing = <KeptBy item={item} />;
  } else if (row.group === "suggested") {
    trailing = <NoticedMark who={item.noticed_by} />;
  }
  return (
    <ListDetailItem
      id={row.key}
      data-slot="digest-item"
      data-id={item.id}
      data-group={row.group}
      data-decided={decided || undefined}
      className={cn(decided && "opacity-50")}
      leading={<KindIcon kind={row.drift ? "drift" : digestKind(item)} />}
      trailing={trailing}
    >
      {row.conflict ? <ConflictLine conflict={row.conflict} /> : <RuleLine item={item} />}
    </ListDetailItem>
  );
}

/** How a rule in force came to be, as the muted caption at the end of its row. */
function KeptBy({ item }: { item: DigestItem }) {
  const how = item.how?.[0];
  const who = item.noticed_by;
  const text = how || (who.kind === "person" ? who.name || t("you") : who.name);
  if (!text) return null;
  return <span className="max-w-32 truncate text-[11px] text-muted-foreground">{text}</span>;
}

const CAPTION_CLASS = "px-4 pt-3 pb-0.5 text-[11px] text-muted-foreground";

/** The line above the list: how much is new, measured from when the person last looked. */
function DigestCaption({ digest, since }: { digest: ContextDigest; since?: string }) {
  const count = digest.numbers.new;
  const quiet = count === 0 && digest.conflicts.length === 0 && digest.drift.length === 0;
  if (since && quiet) {
    return (
      <p data-slot="digest-quiet" className={CAPTION_CLASS}>
        {t("Nothing new since {when}.", { when: formatLook(since) })}
      </p>
    );
  }
  if (count === 0) return null;
  return (
    <p data-slot="digest-new" className={CAPTION_CLASS}>
      {since
        ? t("{count} new since {when}", { count, when: formatLook(since) })
        : t("{count} new", { count })}
    </p>
  );
}

/** A digest with nothing in it: what would appear, and the one action that fills it. */
function DigestEmpty({
  digest,
  since,
  onConnectAssistant,
}: {
  digest: ContextDigest;
  since?: string;
  onConnectAssistant?: () => void;
}) {
  if (digest.numbers.rules > 0) {
    return (
      <div data-slot="digest-settled">
        <EmptyState
          title={
            since
              ? t("Nothing new since {when}.", { when: formatLook(since) })
              : t("Nothing to review")
          }
          description={t(
            "Every rule kapi holds for this project is settled. What an assistant notices next appears here.",
          )}
        />
      </div>
    );
  }
  return (
    <div data-slot="digest-empty">
      <EmptyState
        title={t("Nothing is recorded yet")}
        description={t(
          "When you or an assistant working here notice a name, a word the project avoids or how it addresses its readers, it appears here. It advises assistants and checks from the moment it is recorded.",
        )}
        action={
          onConnectAssistant && (
            <Button size="sm" data-slot="digest-connect-assistant" onClick={onConnectAssistant}>
              Connect an assistant
            </Button>
          )
        }
      />
    </div>
  );
}

/** When the person last looked, as a reader says it: "Tuesday", "12 September". */
export function formatLook(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return iso;
  const days = (Date.now() - at.getTime()) / 86_400_000;
  if (days < 1) return at.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  if (days < 7) return at.toLocaleDateString(undefined, { weekday: "long" });
  return at.toLocaleDateString(undefined, { day: "numeric", month: "long" });
}
