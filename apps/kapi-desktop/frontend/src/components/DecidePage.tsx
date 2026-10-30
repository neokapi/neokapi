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
// option numbers pick an option. An answer is recorded at once through the
// call `kapi rules` makes, and the next question is selected.

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  EmptyState,
  ErrorNotice,
  Input,
  KindIcon,
  ListDetail,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
  LoadingSpinner,
  PageHeader,
  SimpleTooltip,
  cn,
  useListNavigation,
  type DigestKind,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import type { RuleAnswer, RuleDecision, RuleOption, RulesDecideView } from "../types/api";
import { NoticedBy } from "./ContextDigestDetail";

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
 * The arguments DecideRule takes for an answer: the id it names and the form
 * a kept name writes. A keep on a disagreement names the side it chooses; every
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

export interface DecideViewProps {
  /** The questions, null while the first read is in flight. */
  view: RulesDecideView | null;
  loading?: boolean;
  error?: unknown;
  /** The Map row to open on (`suggested:<id>`, `conflict:<id>`). */
  defaultSelected?: string;
  /** Take the keyboard. */
  keyboard?: boolean;
  /** Record an answer. A rejection puts the question back and says so. */
  onAnswer: (d: RuleDecision, answer: RuleAnswer, pick: DecidePick) => Promise<unknown>;
  /** What keeping a name with a typed form does. Absent states nothing for Other…. */
  otherConsequence?: (d: RuleDecision, form: string) => string | undefined;
  /** Report the form typed for Other…, so its consequence can be read. */
  onOtherForm?: (d: RuleDecision, form: string) => void;
  /** Open the file the suggestion was seen in. Absent shows the file without a link. */
  onOpenFile?: (path: string) => void;
  /** Open the project's Map, from the empty state. */
  onOpenMap?: () => void;
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
  onOpenFile,
  onOpenMap,
}: DecideViewProps) {
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

  const opened = decisionForRow(decisions, defaultSelected);
  const [chosen, setChosen] = useState<string | null>(opened ? decisionKey(opened) : null);
  const selected =
    decisions.find((d) => decisionKey(d) === chosen) ??
    (chosen === null ? (opened ?? decisions[0]) : decisions[0]);
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

  const [failure, setFailure] = useState<unknown>(null);
  const answer = (a: RuleAnswer) => {
    if (!selected) return;
    if (a === "keep" && "other" in pick && !pick.other.trim()) return;
    const key = decisionKey(selected);
    const at = decisions.indexOf(selected);
    const next = decisions[at + 1] ?? decisions[at - 1];
    if (a === "later") setLater((s) => new Set([...s, key]));
    else setAnswered({ of: view, keys: new Set([...gone, key]) });
    setChosen(next ? decisionKey(next) : null);
    setFailure(null);
    onAnswer(selected, a, pick).catch((err: unknown) => {
      setFailure(err);
      const without = (s: ReadonlySet<string>) => new Set([...s].filter((k) => k !== key));
      setLater(without);
      setAnswered((s) => ({ ...s, keys: without(s.keys) }));
      setChosen(key);
    });
  };

  const nav = useListNavigation({
    ids: decisions.map(decisionKey),
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
        answer("keep");
        break;
      case "n":
        answer("drop");
        break;
      case "l":
        answer("later");
        break;
      case "ArrowDown":
      case "ArrowUp":
      case "j":
        acted = nav.handleKey(e);
        break;
      default: {
        const n = /^[1-9]$/.test(e.key) ? Number(e.key) : 0;
        if (n >= 1 && n <= selected.options.length) setPick({ option: n - 1 });
        else if (n === selected.options.length + 1 && selected.other) {
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
      subtitle={
        <span data-slot="decide-counts" className="tabular-nums">
          {t("{open} to decide · {kept} kept", "the Decide page's counts", {
            open: decisions.length,
            kept: view.rules,
          })}
        </span>
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
      {failure !== null && (
        <div className="shrink-0 px-6 pb-3">
          <ErrorNotice error={failure} title={t("The answer was not recorded")} />
        </div>
      )}
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
        ) : (
          <ListDetail
            list={
              <ListDetailList
                selectedId={selectedKey}
                onSelect={setChosen}
                aria-label={t("Suggestions to decide")}
              >
                {decisions.map((d) => (
                  <DecideRow key={decisionKey(d)} decision={d} />
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
                  onAnswer={answer}
                  otherConsequence={otherConsequence}
                  otherField={otherField}
                  onOpenFile={onOpenFile}
                />
              )
            }
          />
        )}
      </div>
    </div>
  );
}

/** One row of the list: the kind, the wording, and how much it covers. */
function DecideRow({ decision: d }: { decision: RuleDecision }) {
  return (
    <ListDetailItem
      id={decisionKey(d)}
      data-slot="decide-item"
      data-id={d.id ?? d.options[0]?.id}
      data-kind={d.kind}
      leading={<KindIcon kind={decisionKind(d)} />}
      trailing={<RowMeta decision={d} />}
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
            {t("{lead} of {total}", "a Decide row: blocks", { lead, total })}
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
  onAnswer: (answer: RuleAnswer) => void;
  otherConsequence?: (d: RuleDecision, form: string) => string | undefined;
  otherField: React.RefObject<HTMLInputElement | null>;
  onOpenFile?: (path: string) => void;
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
}: QuestionProps) {
  const disagreement = d.kind === "disagreement";
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
      {(d.options.length > 0 || d.other) && (
        <div role="radiogroup" aria-label={d.question} className="flex flex-col gap-1">
          {d.options.map((o, i) => (
            <OptionRow
              key={o.id ?? o.form ?? i}
              option={o}
              index={i}
              disagreement={disagreement}
              picked={"option" in pick && pick.option === i}
              onPick={() => onPick({ option: i })}
            />
          ))}
          {d.other && (
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
                    onAnswer("keep");
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
      <Evidence decision={d} onOpenFile={onOpenFile} />
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
}: {
  option: RuleOption;
  index: number;
  disagreement: boolean;
  picked: boolean;
  onPick: () => void;
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
        {o.noticed_by && <NoticedBy who={o.noticed_by} />}
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
}: {
  decision: RuleDecision;
  onOpenFile?: (path: string) => void;
}) {
  const seen = d.seen;
  const path = seen?.path;
  const file = path ? path.slice(path.lastIndexOf("/") + 1) : "";
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
        (onOpenFile ? (
          <button
            type="button"
            data-slot="decide-open-file"
            title={path}
            onClick={() => onOpenFile(path)}
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
          <NoticedBy who={d.noticed_by} />
        </span>
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

// ── Wired to the backend ─────────────────────────────────────────────────────

export interface DecidePageProps {
  tabID: string;
  /** The Map row to open on (`suggested:<id>`, `conflict:<id>`). */
  defaultSelected?: string;
  onOpenFile?: (path: string) => void;
  onOpenMap?: () => void;
}

/** How long a typed form rests before what keeping it does is read. */
const OTHER_DEBOUNCE_MS = 250;

/** Decide for the project a tab holds. */
export function DecidePage({ tabID, defaultSelected, onOpenFile, onOpenMap }: DecidePageProps) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: qk.rulesDecide(tabID),
    queryFn: () => api.rulesDecideView(tabID),
    placeholderData: (previous) => previous,
  });
  useInvalidateOnEvent("workspace:changed", [qk.rulesDecide(tabID)]);

  // Opening Decide is looking at what kapi learned, as the digest counts it:
  // the home's "new" standings are read from the marker this moves.
  useEffect(() => {
    void api.markProjectContextDigestSeen(tabID).then(() => {
      void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
    });
  }, [tabID, qc]);

  const decide = useMutation({
    mutationFn: async ({
      d,
      answer,
      pick,
    }: {
      d: RuleDecision;
      answer: RuleAnswer;
      pick: DecidePick;
    }) => {
      const { id, form } = answerArgs(d, answer, pick);
      return api.decideRule(tabID, id, answer, form);
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: qk.rulesDecide(tabID) });
      void qc.invalidateQueries({ queryKey: ["context-digest", tabID] });
      void qc.invalidateQueries({ queryKey: ["context-feed"] });
      void qc.invalidateQueries({ queryKey: qk.mapSummary(tabID) });
      void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
    },
  });

  // What keeping a typed form does, read once the person stops typing.
  const [typed, setTyped] = useState<{ id: string; form: string } | null>(null);
  const [settled, setSettled] = useState<{ id: string; form: string } | null>(null);
  useEffect(() => {
    const handle = window.setTimeout(() => setSettled(typed), OTHER_DEBOUNCE_MS);
    return () => window.clearTimeout(handle);
  }, [typed]);
  const consequence = useQuery({
    queryKey: qk.rulesDecideConsequence(tabID, settled?.id ?? "", settled?.form ?? ""),
    queryFn: () => api.rulesDecideConsequence(tabID, settled!.id, settled!.form),
    enabled: !!settled?.id && !!settled.form.trim(),
  });

  return (
    <DecideView
      view={query.data ?? null}
      loading={query.isLoading}
      error={query.error}
      defaultSelected={defaultSelected}
      onAnswer={(d, answer, pick) => decide.mutateAsync({ d, answer, pick })}
      onOtherForm={(d, form) => setTyped({ id: d.id ?? "", form: form.trim() })}
      otherConsequence={(d, form) =>
        settled && settled.id === d.id && settled.form === form
          ? (consequence.data ?? undefined)
          : undefined
      }
      onOpenFile={onOpenFile}
      onOpenMap={onOpenMap}
    />
  );
}
