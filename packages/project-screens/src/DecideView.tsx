// Decide for a project: each suggestion waiting for a person as one question,
// with the evidence as its options (page-patterns "List and detail").
//
// The list holds one row per suggestion: its kind, the wording it is about,
// and how many forms, words or blocks it covers. The detail pane asks the
// question `kapi rules` asks (host/rules_decide.go): the forms of a name with
// the blocks and files that write each, the most used first, and Other… for a
// form of the person's own; or the two sides of a disagreement, each with what
// keeping it does. What keeping the picked option does is stated above the
// actions, and the sentence the suggestion was seen in, with its file and who
// filed it, below them.
//
// Keep, Not a rule and Later answer it; k, n and l do the same, and the
// option numbers pick an option. An answer is recorded at once through
// onAnswer, and the next question is selected. Without onAnswer the pane draws
// the question and its evidence and no way to answer it.
//
// On a host that assigns questions (onAssign), each row ends with the member
// it is assigned to, the detail pane holds a picker of the members, and the
// header holds an Assigned to me filter. Later leaves the assignment as it is.

import { useEffect, useMemo, useRef, useState } from "react";
import { FileText } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  EmptyState,
  ErrorNotice,
  Input,
  KindIcon,
  ListDetail,
  ListDetailGroup,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
  LoadingSpinner,
  PageHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  SimpleTooltip,
  Toggle,
  cn,
  useListNavigation,
  type DigestKind,
} from "@neokapi/ui-primitives";
import { NoticedBy } from "./NoticedBy";
import { opens, type ScreenNavigation } from "./port";
import type {
  ContextActor,
  RuleAnswer,
  RuleDecision,
  RuleOption,
  RulesDecideView,
  ScreenMember,
} from "./types";

/** The option a keep takes: one of the options, or a form the person typed. */
export type DecidePick = { option: number } | { other: string };

/** A question's key in the list: the suggestion's id, or a disagreement's first side's. */
export function decisionKey(d: RuleDecision): string {
  return d.id || `disagreement:${d.options[0]?.id ?? d.label}`;
}

/**
 * The question a Map row opens on. The row's key is its group and its item's
 * id (`suggested:<id>`, `conflict:<id>`), and the id is a suggestion's own or
 * one side's of a disagreement.
 */
export function decisionForRow(
  decisions: readonly RuleDecision[],
  rowKey: string | undefined,
): RuleDecision | undefined {
  if (!rowKey) return undefined;
  const at = rowKey.indexOf(":");
  const id = at === -1 ? rowKey : rowKey.slice(at + 1);
  return decisions.find((d) => d.id === id || d.options.some((o) => o.id === id));
}

/**
 * The arguments an answer is recorded with: the id it names and the form a
 * kept name writes. A keep on a disagreement names the side it chooses; every
 * other answer on one names its first side, which reaches all of them.
 */
export function answerArgs(
  d: RuleDecision,
  answer: RuleAnswer,
  pick: DecidePick,
): { id: string; form: string } {
  if (d.kind === "disagreement") {
    const side = answer === "keep" && "option" in pick ? d.options[pick.option] : d.options[0];
    return { id: side?.id ?? "", form: "" };
  }
  if (answer !== "keep" || d.kind !== "name") return { id: d.id ?? "", form: "" };
  const form = "other" in pick ? pick.other.trim() : (d.options[pick.option]?.form ?? "");
  return { id: d.id ?? "", form };
}

/** The kind mark a question's row takes. */
function decisionKind(d: RuleDecision): DigestKind {
  return d.kind === "disagreement" ? "conflict" : d.kind;
}

/** The name a person is shown by, where the host knows one. */
type NameOf = (who: ContextActor) => string | undefined;

/** Who holds a question: the member's id and name, and whether it is the reader. */
interface Held {
  id: string | null;
  name?: string;
  mine: boolean;
}

/** The value the picker gives a question assigned to no one. */
const NOBODY = "nobody";

/**
 * Who holds a question: the member an assignment made on this screen names,
 * or else the assignee the view carries. A person with no name is the reader.
 */
function heldBy(
  d: RuleDecision,
  local: string | null | undefined,
  members: readonly ScreenMember[],
  nameOf: NameOf | undefined,
): Held {
  if (local !== undefined) {
    if (local === null) return { id: null, mine: false };
    const m = members.find((x) => x.id === local);
    return { id: local, name: m?.name ?? local, mine: !!m?.me };
  }
  const who = d.assignee;
  if (!who) return { id: null, mine: false };
  const mine = !who.name;
  const id = d.assignee_id ?? (mine ? (members.find((m) => m.me)?.id ?? null) : null);
  const name =
    nameOf?.(who) ||
    who.name ||
    (mine ? t("You", "the reader, as a question's assignee") : undefined);
  return { id, name, mine };
}

export interface DecideViewProps extends ScreenNavigation {
  /** The questions, null while the first read is in flight. */
  view: RulesDecideView | null;
  loading?: boolean;
  error?: unknown;
  /** The Map row to open on (`suggested:<id>`, `conflict:<id>`). */
  defaultSelected?: string;
  /** Take the keyboard. */
  keyboard?: boolean;
  /**
   * Record an answer. A rejection puts the question back and says so. Absent,
   * the pane draws no Keep, Not a rule or Later.
   */
  onAnswer?: (d: RuleDecision, answer: RuleAnswer, pick: DecidePick) => Promise<unknown>;
  /** What keeping a name with a typed form does. Absent states nothing for Other…. */
  otherConsequence?: (d: RuleDecision, form: string) => string | undefined;
  /** Report the form typed for Other…, so its consequence can be read. */
  onOtherForm?: (d: RuleDecision, form: string) => void;
  /** The name a person is shown by. Absent, a person with no name is "you". */
  nameOf?: NameOf;
  /**
   * Assign a question to a member, or to no one. A rejection puts the
   * assignment back and says so. Absent, no assignment is drawn.
   */
  onAssign?: (d: RuleDecision, userId: string | null) => Promise<unknown>;
  /** The members the picker offers. */
  members?: readonly ScreenMember[];
  /** Open with the list narrowed to the questions assigned to the reader. */
  defaultAssignedToMe?: boolean;
}

export function DecideView({
  view,
  loading,
  error,
  defaultSelected,
  keyboard = true,
  onAnswer,
  otherConsequence,
  onOtherForm,
  nameOf,
  onAssign,
  members = [],
  defaultAssignedToMe = false,
  navigate,
  canNavigate,
}: DecideViewProps) {
  const screenNav: ScreenNavigation = { navigate, canNavigate };
  const onOpenMap = opens(screenNav, { kind: "map" }) ? () => navigate({ kind: "map" }) : undefined;
  // The file a suggestion was seen in opens in the Reader, when the host opens it.
  const onOpenFile = (path: string) =>
    opens(screenNav, { kind: "reader", path })
      ? () => navigate({ kind: "reader", path })
      : undefined;
  // Questions answered on this screen, until the view is read again.
  const [answered, setAnswered] = useState<{ of: RulesDecideView | null; keys: Set<string> }>({
    of: null,
    keys: new Set(),
  });
  const gone = answered.of === view ? answered.keys : new Set<string>();
  // Questions left for later, until the page is opened again: later records
  // nothing, so every read of the view still holds them.
  const [later, setLater] = useState<ReadonlySet<string>>(() => new Set());
  const decisions = useMemo(
    () =>
      (view?.decisions ?? []).filter(
        (d) => !gone.has(decisionKey(d)) && !later.has(decisionKey(d)),
      ),
    [view, gone, later],
  );
  const waiting = (view?.decisions ?? []).filter((d) => later.has(decisionKey(d))).length;

  // Assignments made on this screen, by question key, until the view is read again.
  const [assigned, setAssigned] = useState<{
    of: RulesDecideView | null;
    keys: Record<string, string | null>;
  }>({ of: null, keys: {} });
  const local = assigned.of === view ? assigned.keys : {};
  const held = (d: RuleDecision): Held => {
    const key = decisionKey(d);
    return heldBy(d, key in local ? local[key] : undefined, members, nameOf);
  };
  const [mineOnly, setMineOnly] = useState(defaultAssignedToMe && !!onAssign);
  const mine = onAssign ? decisions.filter((d) => held(d).mine) : [];
  const shown = onAssign && mineOnly ? mine : decisions;

  const opened = decisionForRow(shown, defaultSelected);
  const [chosen, setChosen] = useState<string | null>(opened ? decisionKey(opened) : null);
  const selected =
    shown.find((d) => decisionKey(d) === chosen) ??
    (chosen === null ? (opened ?? shown[0]) : shown[0]);
  const selectedKey = selected ? decisionKey(selected) : null;

  // The option picked on each question, by its key, until it is answered.
  const [picks, setPicks] = useState<Record<string, DecidePick>>({});
  const pick: DecidePick = (selectedKey && picks[selectedKey]) || {
    option: selected?.chosen ?? 0,
  };
  const setPick = (p: DecidePick) => {
    if (selectedKey) setPicks((all) => ({ ...all, [selectedKey]: p }));
    if (selected && "other" in p) onOtherForm?.(selected, p.other);
  };

  // An answer the host did not record, drawn in its question's pane with the
  // host's reason, such as a permission the member lacks for that subject.
  const [failure, setFailure] = useState<{ key: string; error: unknown } | null>(null);
  const answer = (a: RuleAnswer) => {
    if (!selected || !onAnswer) return;
    if (a === "keep" && "other" in pick && !pick.other.trim()) return;
    const key = decisionKey(selected);
    const at = shown.indexOf(selected);
    const next = shown[at + 1] ?? shown[at - 1];
    if (a === "later") setLater((s) => new Set([...s, key]));
    else setAnswered({ of: view, keys: new Set([...gone, key]) });
    setChosen(next ? decisionKey(next) : null);
    setFailure(null);
    onAnswer(selected, a, pick).catch((err: unknown) => {
      setFailure({ key, error: err });
      const without = (s: ReadonlySet<string>) => new Set([...s].filter((k) => k !== key));
      setLater(without);
      setAnswered((s) => ({ ...s, keys: without(s.keys) }));
      setChosen(key);
    });
  };

  const [assignFailure, setAssignFailure] = useState<{ key: string; error: unknown } | null>(null);
  const assign = (d: RuleDecision, userId: string | null) => {
    if (!onAssign) return;
    const key = decisionKey(d);
    setAssigned({ of: view, keys: { ...local, [key]: userId } });
    setAssignFailure(null);
    onAssign(d, userId).catch((err: unknown) => {
      setAssignFailure({ key, error: err });
      setAssigned((s) => {
        const keys = { ...s.keys };
        delete keys[key];
        return { ...s, keys };
      });
    });
  };

  const nav = useListNavigation({
    ids: shown.map(decisionKey),
    selectedId: selectedKey,
    onSelect: setChosen,
  });
  const otherField = useRef<HTMLInputElement>(null);

  // The page's keys, read through a ref so the listener is bound once and
  // acts on the question selected now. It listens in the capture phase, so k
  // keeps rather than moving the list's selection up.
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
    if (!selected) return;
    let acted = true;
    switch (e.key) {
      case "k":
      case "n":
      case "l":
        if (onAnswer) answer(e.key === "k" ? "keep" : e.key === "n" ? "drop" : "later");
        else acted = false;
        break;
      case "ArrowDown":
      case "ArrowUp":
      case "j":
        acted = nav.handleKey(e);
        break;
      default: {
        const n = /^[1-9]$/.test(e.key) ? Number(e.key) : 0;
        if (n >= 1 && n <= selected.options.length) setPick({ option: n - 1 });
        else if (n === selected.options.length + 1 && selected.other && onAnswer) {
          setPick({ other: "other" in pick ? pick.other : "" });
          otherField.current?.focus();
        } else acted = false;
      }
    }
    if (acted) e.preventDefault();
  };
  useEffect(() => {
    if (!keyboard) return;
    const listener = (e: KeyboardEvent) => onKey.current(e);
    window.addEventListener("keydown", listener, true);
    return () => window.removeEventListener("keydown", listener, true);
  }, [keyboard]);

  const header = view && (
    <PageHeader
      className="mb-0 shrink-0 px-6 pt-5 pb-3"
      title={t("{project} · Decide", "the Decide page title", { project: view.project_name })}
      // With nothing waiting, the empty state states the kept count.
      subtitle={
        decisions.length > 0 ? (
          <span data-slot="decide-counts" className="tabular-nums">
            {t("{open} to decide · {kept} kept", "the Decide page's counts", {
              open: decisions.length,
              kept: view.rules,
            })}
          </span>
        ) : undefined
      }
      actions={
        onAssign &&
        decisions.length > 0 && (
          <Toggle
            size="sm"
            variant="outline"
            data-slot="decide-mine"
            pressed={mineOnly}
            onPressedChange={setMineOnly}
          >
            {t("Assigned to me")}
            <span className="tabular-nums text-muted-foreground">{mine.length}</span>
          </Toggle>
        )
      }
    />
  );

  if (error) {
    return (
      <div className="p-6">
        <ErrorNotice error={error} title={t("The questions could not be read")} />
      </div>
    );
  }
  if (!view) {
    return loading ? (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    ) : null;
  }

  return (
    <div data-slot="decide" className="flex h-full min-h-0 flex-col">
      {header}
      <div className="min-h-0 flex-1 border-t border-border">
        {decisions.length === 0 ? (
          <div className="mx-auto max-w-xl p-6" data-slot="decide-empty">
            <EmptyState
              title={t("Nothing to decide.")}
              description={
                waiting > 0
                  ? t(
                      "{count, plural, one {# rule kept} other {# rules kept}} · {later} left for later",
                      "the empty Decide page",
                      { count: view.rules, later: waiting },
                    )
                  : t(
                      "{count, plural, one {# rule kept} other {# rules kept}}",
                      "the empty Decide page",
                      { count: view.rules },
                    )
              }
              action={
                onOpenMap && (
                  <Button
                    size="sm"
                    variant="outline"
                    data-slot="decide-open-map"
                    onClick={onOpenMap}
                  >
                    Open the Map
                  </Button>
                )
              }
            />
          </div>
        ) : shown.length === 0 ? (
          <div className="mx-auto max-w-xl p-6" data-slot="decide-mine-empty">
            <EmptyState
              title={t("Nothing assigned to you.")}
              action={
                <Button size="sm" variant="outline" onClick={() => setMineOnly(false)}>
                  Show every question
                </Button>
              }
            />
          </div>
        ) : (
          <ListDetail
            list={
              <ListDetailList
                selectedId={selectedKey}
                onSelect={setChosen}
                aria-label={t("Suggestions to decide")}
              >
                {onAssign && (
                  <ListDetailGroup
                    data-slot="decide-columns"
                    label={t("Suggestions")}
                    count={t("Assigned to")}
                  />
                )}
                {shown.map((d) => (
                  <DecideRow
                    key={decisionKey(d)}
                    decision={d}
                    held={onAssign ? held(d) : undefined}
                  />
                ))}
              </ListDetailList>
            }
            detail={
              selected && (
                <Question
                  key={selectedKey}
                  decision={selected}
                  pick={pick}
                  onPick={setPick}
                  onAnswer={onAnswer ? answer : undefined}
                  otherConsequence={otherConsequence}
                  otherField={otherField}
                  onOpenFile={onOpenFile}
                  nameOf={nameOf}
                  failure={failure?.key === selectedKey ? failure.error : undefined}
                  assignee={
                    onAssign && (
                      <AssigneePicker
                        held={held(selected)}
                        members={members}
                        onAssign={(userId) => assign(selected, userId)}
                        failure={
                          assignFailure?.key === selectedKey ? assignFailure.error : undefined
                        }
                      />
                    )
                  }
                />
              )
            }
          />
        )}
      </div>
    </div>
  );
}

/**
 * One row of the list: the kind, the wording, and how much it covers, and on
 * a host that assigns questions the member who holds it, or a dash.
 */
function DecideRow({ decision: d, held }: { decision: RuleDecision; held?: Held }) {
  return (
    <ListDetailItem
      id={decisionKey(d)}
      data-slot="decide-item"
      data-id={d.id ?? d.options[0]?.id}
      data-kind={d.kind}
      leading={<KindIcon kind={decisionKind(d)} />}
      trailing={
        <>
          <RowMeta decision={d} />
          {held && (
            <span
              data-slot="decide-assignee"
              data-mine={held.mine || undefined}
              className="w-24 truncate text-right text-[11px] text-muted-foreground"
              title={held.name}
              translate="no"
            >
              {held.id === null && !held.name ? (
                <span aria-label={t("Nobody", "no member holds the question")}>–</span>
              ) : (
                held.name
              )}
            </span>
          )}
        </>
      }
    >
      <span translate="no">{d.label || d.seen?.quote || d.sentence}</span>
    </ListDetailItem>
  );
}

/** How much a question covers, as the muted count at the end of its row. */
function RowMeta({ decision: d }: { decision: RuleDecision }) {
  let text: string | undefined;
  if (d.kind === "name" && d.options.length > 0) {
    text = t("{count, plural, one {# form} other {# forms}}", "a Decide row", {
      count: d.options.length,
    });
  } else if (d.kind === "word" && d.avoided > 0) {
    text = t("{count, plural, one {# word} other {# words}}", "a Decide row", {
      count: d.avoided,
    });
  } else if (d.kind === "disagreement" && d.options.length > 0) {
    const total = d.options.reduce((n, o) => n + o.blocks, 0);
    const lead = d.options[d.chosen]?.blocks ?? 0;
    if (total > 0) {
      return (
        <SimpleTooltip
          content={t("{lead} of {total} blocks write what the leading side keeps", {
            lead,
            total,
          })}
        >
          <span className="text-[11px] text-muted-foreground tabular-nums">
            {t("{lead} of {total, plural, one {# block} other {# blocks}}", "a Decide row", {
              lead,
              total,
            })}
          </span>
        </SimpleTooltip>
      );
    }
  }
  if (!text) return null;
  return <span className="text-[11px] text-muted-foreground tabular-nums">{text}</span>;
}

interface QuestionProps {
  decision: RuleDecision;
  pick: DecidePick;
  onPick: (pick: DecidePick) => void;
  /** Absent, the pane draws no actions and no Other…. */
  onAnswer?: (answer: RuleAnswer) => void;
  otherConsequence?: (d: RuleDecision, form: string) => string | undefined;
  otherField: React.RefObject<HTMLInputElement | null>;
  /** How a file the suggestion was seen in opens, or undefined when it does not. */
  onOpenFile: (path: string) => (() => void) | undefined;
  nameOf?: NameOf;
  /** Why the last answer to this question was not recorded. */
  failure?: unknown;
  /** The picker of who holds the question, on a host that assigns questions. */
  assignee?: React.ReactNode;
}

/** The detail pane: the question, its options, what keeping does, the actions and the evidence. */
function Question({
  decision: d,
  pick,
  onPick,
  onAnswer,
  otherConsequence,
  otherField,
  onOpenFile,
  nameOf,
  failure,
  assignee,
}: QuestionProps) {
  const disagreement = d.kind === "disagreement";
  // A typed form is only ever kept, so a pane that keeps nothing takes none.
  const takesOther = d.other && !!onAnswer;
  const other = "other" in pick ? pick.other : null;
  // A disagreement states what keeping each side does on the side itself.
  let consequence: string | undefined;
  if (disagreement) consequence = undefined;
  else if ("other" in pick)
    consequence = pick.other.trim() ? otherConsequence?.(d, pick.other.trim()) : undefined;
  else if (d.options.length > 0) consequence = d.options[pick.option]?.consequence;
  else consequence = d.consequence;

  return (
    <ListDetailPane
      data-slot="decide-question"
      data-id={d.id ?? d.options[0]?.id}
      className="max-w-3xl"
      toolbar={
        <ListDetailToolbar
          title={<span data-slot="decide-question-text">{d.question}</span>}
          actions={
            d.reason && (
              <span data-slot="decide-reason" className="text-[11px] text-muted-foreground">
                {d.reason}
              </span>
            )
          }
        />
      }
    >
      {d.options.length === 0 && (
        <p data-slot="decide-sentence" className="text-sm">
          {d.sentence}
        </p>
      )}
      {(d.options.length > 0 || takesOther) && (
        <div role="radiogroup" aria-label={d.question} className="flex flex-col gap-1">
          {d.options.map((o, i) => (
            <OptionRow
              key={o.id ?? o.form ?? i}
              option={o}
              index={i}
              disagreement={disagreement}
              picked={"option" in pick && pick.option === i}
              onPick={() => onPick({ option: i })}
              nameOf={nameOf}
            />
          ))}
          {takesOther && (
            <div
              data-slot="decide-other"
              className={cn(
                "grid grid-cols-[1.5rem_minmax(0,1fr)] items-center gap-2 rounded-md border px-2 py-1.5",
                other !== null ? "border-primary bg-primary/5" : "border-transparent",
              )}
            >
              <OptionNumber n={d.options.length + 1} />
              <Input
                ref={otherField}
                value={other ?? ""}
                placeholder={t("Other…")}
                onFocus={() => other === null && onPick({ other: "" })}
                onChange={(e) => onPick({ other: e.target.value })}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    onAnswer?.("keep");
                  } else if (e.key === "Escape") {
                    e.preventDefault();
                    onPick({ option: d.chosen });
                    e.currentTarget.blur();
                  }
                }}
                className="h-8 max-w-64 font-mono"
                translate="no"
                aria-label={t("Form to write")}
              />
            </div>
          )}
        </div>
      )}
      {consequence && (
        <p data-slot="decide-consequence" className="text-sm font-medium">
          {consequence}
        </p>
      )}
      {onAnswer && (
        <div data-slot="decide-actions" className="flex flex-wrap items-center gap-1.5">
          <Button
            size="sm"
            variant="success"
            data-slot="decide-keep"
            aria-keyshortcuts="k"
            disabled={other !== null && !other.trim()}
            onClick={() => onAnswer("keep")}
          >
            Keep
            <Shortcut k="k" />
          </Button>
          <Button
            size="sm"
            variant="outline"
            data-slot="decide-drop"
            aria-keyshortcuts="n"
            onClick={() => onAnswer("drop")}
          >
            Not a rule
            <Shortcut k="n" />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            data-slot="decide-later"
            aria-keyshortcuts="l"
            onClick={() => onAnswer("later")}
          >
            Later
            <Shortcut k="l" />
          </Button>
        </div>
      )}
      {failure !== undefined && (
        <div data-slot="decide-refused">
          <ErrorNotice error={failure} title={t("The answer was not recorded")} />
        </div>
      )}
      {assignee}
      <Evidence decision={d} onOpenFile={onOpenFile} nameOf={nameOf} />
    </ListDetailPane>
  );
}

/** One option: its number, the form or the side, and the blocks and files that write it. */
function OptionRow({
  option: o,
  index,
  disagreement,
  picked,
  onPick,
  nameOf,
}: {
  option: RuleOption;
  index: number;
  disagreement: boolean;
  picked: boolean;
  onPick: () => void;
  nameOf?: NameOf;
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={picked}
      data-slot="decide-option"
      data-picked={picked || undefined}
      onClick={onPick}
      className={cn(
        "grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-2 rounded-md border px-2 py-1.5 text-left text-sm",
        picked ? "border-primary bg-primary/5" : "border-transparent hover:bg-muted",
      )}
    >
      <OptionNumber n={index + 1} />
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className={cn("truncate", !disagreement && "font-mono")} translate="no">
          {disagreement ? o.sentence : o.form}
        </span>
        {disagreement && (
          <span data-slot="decide-side-consequence" className="text-[11px] text-muted-foreground">
            {o.consequence}
          </span>
        )}
      </span>
      <span className="flex items-center gap-2 text-[11px] text-muted-foreground tabular-nums">
        {t(
          "{blocks, plural, one {# block} other {# blocks}} · {files, plural, one {# file} other {# files}}",
          "an option's uses on Decide",
          { blocks: o.blocks, files: o.files },
        )}
        {o.noticed_by && <NoticedBy who={o.noticed_by} nameOf={nameOf} />}
      </span>
    </button>
  );
}

/** An option's number, the key that picks it. */
function OptionNumber({ n }: { n: number }) {
  return (
    <kbd
      aria-hidden
      className="rounded border border-border text-center font-mono text-[10px] text-muted-foreground"
    >
      {n}
    </kbd>
  );
}

/** Where the suggestion was seen: the sentence, its file, and who filed it. */
function Evidence({
  decision: d,
  onOpenFile,
  nameOf,
}: {
  decision: RuleDecision;
  onOpenFile: (path: string) => (() => void) | undefined;
  nameOf?: NameOf;
}) {
  const seen = d.seen;
  const path = seen?.path;
  const file = path ? path.slice(path.lastIndexOf("/") + 1) : "";
  const open = path ? onOpenFile(path) : undefined;
  if (!seen?.quote && !path && d.kind === "disagreement") return null;
  return (
    <div
      data-slot="decide-evidence"
      className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1 border-t border-border pt-3 text-sm"
    >
      {(seen?.quote || path) && (
        <span className="text-[11px] text-muted-foreground">
          <>Seen in</>
        </span>
      )}
      {seen?.quote && (
        <q data-slot="decide-quote" className="italic" translate="no">
          {seen.quote}
        </q>
      )}
      {seen?.quote && path && <span className="text-muted-foreground">·</span>}
      {path &&
        (open ? (
          <button
            type="button"
            data-slot="decide-open-file"
            title={path}
            onClick={open}
            className="inline-flex items-center gap-1 font-mono text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline focus-visible:text-foreground focus-visible:underline focus-visible:outline-none"
          >
            <FileText size={11} aria-hidden />
            <span translate="no">{file}</span>
          </button>
        ) : (
          <span className="font-mono text-[11px] text-muted-foreground" title={path} translate="no">
            {file}
          </span>
        ))}
      {d.kind !== "disagreement" && (
        <span data-slot="decide-noticed" className="ml-auto text-[11px]">
          <NoticedBy who={d.noticed_by} nameOf={nameOf} />
        </span>
      )}
    </div>
  );
}

/** Who holds a question, as a picker of the members and Nobody. */
function AssigneePicker({
  held,
  members,
  onAssign,
  failure,
}: {
  held: Held;
  members: readonly ScreenMember[];
  onAssign: (userId: string | null) => void;
  /** Why the last assignment of this question was not recorded. */
  failure?: unknown;
}) {
  const value = held.id ?? NOBODY;
  // An assignee the members list does not hold (a former member) stays shown.
  const listed = held.id === null || members.some((m) => m.id === held.id);
  return (
    <div data-slot="decide-assign" className="flex flex-wrap items-center gap-2 text-sm">
      <span className="text-[11px] text-muted-foreground">
        <>Assigned to</>
      </span>
      <Select value={value} onValueChange={(v) => onAssign(v === NOBODY ? null : v)}>
        <SelectTrigger size="sm" className="w-48" aria-label={t("Assigned to")}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NOBODY}>{t("Nobody", "no member holds the question")}</SelectItem>
          {!listed && held.id && <SelectItem value={held.id}>{held.name ?? held.id}</SelectItem>}
          {members.map((m) => (
            <SelectItem key={m.id} value={m.id}>
              <span translate="no">{m.name}</span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {failure !== undefined && (
        <div data-slot="decide-assign-refused" className="basis-full">
          <ErrorNotice error={failure} title={t("The assignment was not recorded")} />
        </div>
      )}
    </div>
  );
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
