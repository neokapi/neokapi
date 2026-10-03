import { useCallback, useEffect, useMemo, useState } from "react";
import {
  BookOpen,
  Check,
  CheckCheck,
  CheckCircle2,
  Circle,
  FileText,
  Loader2,
  PauseCircle,
  RefreshCw,
  Sparkles,
  X,
} from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  HistoryCard,
  JudgementCard,
  LocaleLabel,
  NeighbourhoodCard,
  PointCard,
  ProvenanceCard,
  ReviewLanguageSelect,
  ALL_LANGUAGES,
  checkFindingViews,
  StatusBadge,
  localeLabel,
  ScrollArea,
  SimpleTooltip,
  directionAttrs,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { VirtualList } from "@neokapi/editor-grid";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { AIExchangeDisclosure } from "./AIExchangeView";
import { FilePreview } from "./FilePreview";
import { SourceUnitPane } from "./review/SourceUnitPane";
import { ChangesCard } from "./edit/ChangesCard";
import { EditionEditPanel } from "./edit/EditionEditPanel";
import { EditTextDisplay } from "./edit/EditTextDisplay";
import { StalePrompt } from "./edit/StalePrompt";
import { useChangeSender } from "./edit/useChangeSender";
import { useActiveFilter } from "../context/ActiveFilterContext";
import {
  type ChangeClient,
  decideOp,
  editionContent,
  readBlock,
  refusalMessage,
  setContentOps,
  tabChanges,
} from "../lib/changes";
import type { BlockRead, ChangeResult, EditionHistory } from "@neokapi/contract-types";
import type {
  CheckWarning,
  PreReviewResult,
  PreReviewScope,
  ReviewAIActionKind,
  ReviewAIActionResult,
  ReviewContext,
  ReviewItem,
  ReviewLanguage,
  ReviewUnitDetail,
  AIActivityEntry,
} from "../types/api";

/** Initial queue narrowing handed in by an entry point (a ship-gate cell or a
 *  timeline tag on the Collections surface). */
export interface ReviewScope {
  collection?: string;
  locale?: string;
}

export type ReviewDecision = "approved" | "rejected";

export interface ReviewPageProps {
  tabID: string;
  /** Narrow the queue to a (collection, locale) on entry. */
  scope?: ReviewScope;
  /** Pre-loaded queue for Storybook/tests, in place of api.reviewQueue(). */
  items?: ReviewItem[];
  /** Pre-loaded per-language counts (Storybook/tests). Derived from `items`
   *  when absent. */
  languages?: ReviewLanguage[];
  /** Pre-loaded queue warnings (Storybook/tests), in place of the ones
   *  api.reviewQueue() returns. */
  warnings?: CheckWarning[];
  /** Override the unit loader (Storybook/tests); defaults to api.getReviewUnit. */
  loadUnit?: (item: ReviewItem) => Promise<ReviewUnitDetail | null>;
  /**
   * The change service the page reads each unit from and sends every edit and
   * decision to, as change sets (Storybook/tests pass an in-memory one);
   * defaults to the tab's.
   */
  changes?: ChangeClient;
  /** Override the source review-model loader (Storybook/tests); defaults to
   *  api.getSourceUnitContext. */
  loadSourceContext?: (item: ReviewItem) => Promise<ReviewContext | null>;
  /** Override the per-unit AI action (Storybook/tests); defaults to api.reviewAIAction. */
  onAIAction?: (
    item: ReviewItem,
    action: ReviewAIActionKind,
    instruction: string,
  ) => Promise<ReviewAIActionResult | null>;
  /** Override the pre-review runner (Storybook/tests); defaults to api.runAIPreReview. */
  onPreReview?: (locale: string, scope: PreReviewScope) => Promise<PreReviewResult | null>;
}

type Chip = "all" | "findings" | "clean";

/** The language this row belongs to: a target locale, or the source language. */
const rowLanguage = (it: ReviewItem) => it.language || it.locale;

/**
 * Count the pending units per language, source language first.
 *
 * The backend sends this with the queue; this is the same summary for a
 * pre-loaded queue (Storybook, tests), so a selector reads one shape either way.
 */
function summarizeLanguages(items: ReviewItem[]): ReviewLanguage[] {
  const byLanguage = new Map<string, ReviewLanguage>();
  for (const it of items) {
    const language = rowLanguage(it);
    const at = byLanguage.get(language);
    if (at) {
      at.pending += 1;
      if (it.isSource) at.source = true;
      continue;
    }
    byLanguage.set(language, { language, pending: 1, source: it.isSource || undefined });
  }
  return [...byLanguage.values()].sort((a, b) => {
    if (!!a.source !== !!b.source) return a.source ? -1 : 1;
    return a.language.localeCompare(b.language);
  });
}

/** A one-field question the page asks before acting. */
interface AskPrompt {
  kind: "reject" | "retranslate";
  title: string;
  label: string;
  placeholder: string;
  confirm: string;
  /** Reject accepts an empty note; retranslate needs an instruction. */
  allowEmpty?: boolean;
}

const itemId = (it: ReviewItem) => `${it.locale}:${it.file}:${it.key}`;

function shortFile(p: string): string {
  const parts = p.split(/[\\/]/);
  return parts.slice(-2).join("/") || p;
}

/**
 * Queue ordering: source rows first, then findings-first among the targets,
 * then file, then key.
 *
 * Source first is where convergence.SortReviewQueue puts them, so this list and
 * `kapi status --review` read in the same order. Findings-first applies to the
 * target rows: a source row carries no findings enrichment to sort on.
 */
function orderItems(items: ReviewItem[]): ReviewItem[] {
  return [...items].sort((a, b) => {
    if (!!a.isSource !== !!b.isSource) return a.isSource ? -1 : 1;
    if (!a.isSource) {
      const fa = a.hasFindings ? 0 : 1;
      const fb = b.hasFindings ? 0 : 1;
      if (fa !== fb) return fa - fb;
    }
    if (a.file !== b.file) return a.file.localeCompare(b.file);
    if (a.key !== b.key) return a.key.localeCompare(b.key);
    return a.locale.localeCompare(b.locale);
  });
}

/**
 * Whether a unit's text is still what its queue row lists. A row shows the
 * text with its whitespace collapsed and, past a length, cut short with an
 * ellipsis, so a long text is still listed when it begins with what the row
 * shows.
 */
function stillListed(text: string, listed: string | undefined): boolean {
  if (listed === undefined) return false;
  const collapse = (s: string) => s.split(/\s+/).filter(Boolean).join(" ");
  const now = collapse(text);
  const row = collapse(listed);
  if (now === row) return true;
  if (!row.endsWith("…")) return false;
  // The row is cut by bytes, which can split a character; what is left of it
  // reads as a replacement character.
  const head = row.slice(0, -1).replace(/\uFFFD+$/, "");
  return head.length > 0 && now.startsWith(head);
}

/**
 * The review surface: one queue, one language selector.
 *
 * A project has work in several languages, the source language among them, and
 * the reviewer picks the one they are working in. Picking the source language
 * puts the author's own wording in front of them; picking a target puts
 * translations of it there; "All languages" lists everything with each row
 * saying which language it belongs to and the source rows leading. One list
 * holds both kinds, so a count in the selector and the rows under it are the
 * same answer.
 *
 * A keyboard-first three-pane page: the queue on the left (source first, then
 * findings-first, filterable by findings and collection), the unit in the
 * center, and the five layers of the review model below, each headed by its own
 * verdict. A target row shows its source read-only with its translation
 * editable; a source row shows the author's wording editable, in SourceUnitPane.
 * Every edit and decision is a change set sent to the change service with the
 * revision the page read for the unit: an edit is a set_content (formatted text
 * in the inline-code editor, a plural a form at a time), and a decision (a
 * approve / r reject) is a decide bound to the text it judged. When the text
 * changed since the page read it, nothing is written: the page shows the text
 * as it stands and asks before applying the change over it. A source unit has
 * one decision, approve, so `a` is the only decision key a source row answers.
 * The unit's recorded changes are listed under its provenance.
 *
 * The AI paths are explicit clicks: per-unit actions (Fix with AI, Retranslate,
 * Explain) yield a proposal diff that Accept sends as the same set_content a
 * manual edit sends, and the pre-review modal annotates by default (an optional
 * auto-approve is recorded as ai/<model>, which human-required gates ignore).
 * Listing or loading the queue calls no provider.
 */
export function ReviewPage({
  tabID,
  scope,
  items: propItems,
  languages: propLanguages,
  warnings: propWarnings,
  loadUnit,
  changes,
  loadSourceContext,
  onAIAction,
  onPreReview,
}: ReviewPageProps) {
  const { showError } = useError();
  const [queue, setQueue] = useState<ReviewItem[] | null>(propItems ?? null);
  const [queueLanguages, setQueueLanguages] = useState<ReviewLanguage[] | null>(
    propLanguages ?? null,
  );
  // Declared files the queue could not read, such as a collection in a format
  // no installed plugin supplies. They are named above the queue, and an empty
  // queue with any of them is not a finished one.
  const [queueWarnings, setQueueWarnings] = useState<CheckWarning[]>(propWarnings ?? []);
  const [loadingQueue, setLoadingQueue] = useState(!propItems);
  const [chip, setChip] = useState<Chip>("all");
  // The one language control: "" is every language, and the project's source
  // language selects source review.
  const [language, setLanguage] = useState(scope?.locale ?? "");
  const [collectionFilter, setCollectionFilter] = useState(scope?.collection ?? "");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [unit, setUnit] = useState<ReviewUnitDetail | null>(null);
  const [unitLoading, setUnitLoading] = useState(false);
  // The selected unit as the change service reads it: the reference and the
  // revision every edit and decision on it names, and its content with codes.
  const client = useMemo(() => changes ?? tabChanges(tabID), [changes, tabID]);
  const [read, setRead] = useState<BlockRead | null>(null);
  const [readError, setReadError] = useState<string | null>(null);
  const [history, setHistory] = useState<EditionHistory | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  // The last edit that landed, for the source pane's "re-drafted on the next
  // run" line.
  const [lastSaved, setLastSaved] = useState<ChangeResult | null>(null);
  const [deciding, setDeciding] = useState(false);
  const [batch, setBatch] = useState<{ done: number; total: number } | null>(null);
  // Per-unit AI actions (phase 3): a running action, the proposed replacement
  // shown as a diff (Accept routes through the save-target path), and the
  // explain text.
  const [aiBusy, setAIBusy] = useState<ReviewAIActionKind | null>(null);
  // The queue is scoped by the project's Active Filter, the same control the
  // Checks panel reads. The in-page dropdowns below narrow within that scope
  // rather than replacing it, so a reviewer can jump between languages without
  // editing the project-wide filter.
  const { active: activeFilter } = useActiveFilter();
  // window.prompt is a no-op in the app's webview: it returns null without ever
  // showing anything, and both callers read null as "cancelled". Retranslate and
  // Reject therefore did nothing at all, silently, with no error to explain it.
  // These drive an in-app dialog instead, which is also the only version a test
  // can drive.
  const [askText, setAskText] = useState<AskPrompt | null>(null);
  const [askValue, setAskValue] = useState("");
  const [aiProposal, setAIProposal] = useState<{ text: string; edit?: string } | null>(null);
  const [aiExplanation, setAIExplanation] = useState<string | null>(null);
  // The calls the last AI action made. Held beside its result so the disclosure
  // under a proposal shows the prompt that produced THAT proposal.
  const [aiExchanges, setAIExchanges] = useState<AIActivityEntry[]>([]);
  // AI pre-review modal state.
  const [preReviewOpen, setPreReviewOpen] = useState(false);
  const [preReviewRunning, setPreReviewRunning] = useState(false);
  const [preReviewResult, setPreReviewResult] = useState<PreReviewResult | null>(null);
  const [reviewerModel, setReviewerModel] = useState<string>("");
  // The read-only document view, opened at the selected unit. It reads the file
  // and never commits: approve, reject and retranslate stay here on the queue,
  // and closing it returns to the queue with this unit still selected and the
  // list where it was.
  const [documentOpen, setDocumentOpen] = useState(false);

  const refreshQueue = useCallback(async () => {
    if (propItems) {
      setQueue(propItems);
      setQueueLanguages(propLanguages ?? null);
      setQueueWarnings(propWarnings ?? []);
      setLoadingQueue(false);
      return;
    }
    setLoadingQueue(true);
    try {
      const result = await api.reviewQueue(tabID, activeFilter ?? { id: "", name: "" });
      setQueue(result?.pending ?? []);
      setQueueLanguages(result?.languages ?? null);
      setQueueWarnings(result?.warnings ?? []);
    } catch (err) {
      showError("Failed to load the review queue", err);
      setQueue([]);
      setQueueLanguages(null);
      setQueueWarnings([]);
    } finally {
      setLoadingQueue(false);
    }
  }, [tabID, propItems, propLanguages, propWarnings, activeFilter, showError]);

  useEffect(() => {
    void refreshQueue();
  }, [refreshQueue]);

  // The languages the selector offers, with the count behind each. The engine
  // sends them with the queue; a pre-loaded queue is summarized the same way.
  const languages = useMemo(
    () => queueLanguages ?? summarizeLanguages(queue ?? []),
    [queueLanguages, queue],
  );
  const collections = useMemo(
    () =>
      Array.from(new Set((queue ?? []).map((it) => it.collection ?? "").filter(Boolean))).sort(),
    [queue],
  );

  // The visible queue: the chosen language, the collection, the findings chip,
  // source-first then findings-first order. Both kinds of row list here, which
  // is what makes the selector's count and the rows under it agree.
  //
  // The chips read the findings enrichment, which the queue computes for
  // translations only. A source row is therefore neither "with findings" nor
  // "clean", and lists under All alone.
  const visible = useMemo(() => {
    let items = queue ?? [];
    if (language) items = items.filter((it) => rowLanguage(it) === language);
    if (collectionFilter) items = items.filter((it) => (it.collection ?? "") === collectionFilter);
    if (chip === "findings") items = items.filter((it) => it.hasFindings === true);
    if (chip === "clean") items = items.filter((it) => it.hasFindings === false);
    return orderItems(items);
  }, [queue, language, collectionFilter, chip]);

  const selectedIndex = visible.findIndex((it) => itemId(it) === selectedId);
  const selected = selectedIndex >= 0 ? visible[selectedIndex] : null;

  // Keep a valid selection as the visible queue changes.
  useEffect(() => {
    if (visible.length === 0) {
      if (selectedId !== null) setSelectedId(null);
      return;
    }
    if (!visible.some((it) => itemId(it) === selectedId)) {
      setSelectedId(itemId(visible[0]));
    }
  }, [visible, selectedId]);

  // Load the unit detail when the selection changes. Any pending AI proposal
  // or explanation belongs to the previous unit — drop it.
  //
  // The detail carries the whole review model (the point, the neighbourhood,
  // the history), which costs more than the queue row it was picked from. The
  // two strings the queue already holds render straight away and the model
  // fills in behind them, so j/k stays as fast as the list.
  useEffect(() => {
    setAIProposal(null);
    setAIExplanation(null);
    setAIExchanges([]);
    // A source row has no translation to load: SourceUnitPane loads its own
    // model from GetSourceUnitContext.
    if (!selected || selected.isSource) {
      setUnit(null);
      return;
    }
    setUnit(null);
    let cancelled = false;
    setUnitLoading(true);
    const load =
      loadUnit ?? ((it: ReviewItem) => api.getReviewUnit(tabID, it.locale, it.file, it.key));
    load(selected)
      .then((d) => {
        if (cancelled) return;
        setUnit(d ?? null);
      })
      .catch((err) => {
        if (!cancelled) showError("Failed to load the review unit", err);
      })
      .finally(() => {
        if (!cancelled) setUnitLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, tabID, loadUnit]);

  // Read the selected unit through the change service. A target row's file is
  // the translation's own file, whose read names that edition; a source row's
  // is the source. The read and the history it names are what an edit or a
  // decision binds to.
  const loadRead = useCallback(
    (item: ReviewItem) => readBlock(client, item.file, item.key),
    [client],
  );
  useEffect(() => {
    setRead(null);
    setReadError(null);
    setLastSaved(null);
    if (!selected) return;
    let cancelled = false;
    loadRead(selected)
      .then((r) => {
        if (!cancelled) setRead(r);
      })
      .catch((err) => {
        if (!cancelled) setReadError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, loadRead]);

  const readRef = read?.ref;
  const readRev = read?.rev;
  useEffect(() => {
    setHistory(null);
    if (!readRef) return;
    let cancelled = false;
    setHistoryLoading(true);
    client
      .history({ ref: readRef })
      .then((h) => {
        if (!cancelled) setHistory(h);
      })
      .catch(() => {
        // The history is context beside the decision; failing to read it
        // leaves the unit reviewable.
        if (!cancelled) setHistory(null);
      })
      .finally(() => {
        if (!cancelled) setHistoryLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client, readRef?.doc, readRef?.block, readRef?.edition, readRev]);

  // After an edit lands: read the unit again (its new revision), and for a
  // translation reload the detail so the checks re-run against the new text and
  // the queue row shows it.
  const afterEdit = useCallback(
    async (res: ChangeResult) => {
      if (!selected) return;
      setLastSaved(res);
      setRead(await loadRead(selected));
      if (selected.isSource) {
        // The queue's counts follow the source's rung, which an edit moves.
        await refreshQueue();
        return;
      }
      const load =
        loadUnit ?? ((it: ReviewItem) => api.getReviewUnit(tabID, it.locale, it.file, it.key));
      const d = await load(selected);
      setUnit(d ?? null);
      if (d) {
        const has = d.findings.length > 0;
        setQueue((q) =>
          (q ?? []).map((it) =>
            itemId(it) === itemId(selected) ? { ...it, target: d.target, hasFindings: has } : it,
          ),
        );
      }
    },
    [selected, loadRead, loadUnit, tabID, refreshQueue],
  );
  const reread = useCallback(async () => {
    if (selected) setRead(await loadRead(selected));
  }, [selected, loadRead]);

  // Edits (the editor's save, an accepted AI proposal) and decisions each hold
  // their own refusal, so a stale decision is asked about beside the decision
  // keys and a stale edit beside the editor.
  const editSender = useChangeSender(client, { onApplied: afterEdit, onReload: reread });
  const decisionSender = useChangeSender(client, { onReload: reread });
  const clearEdit = editSender.clear;
  const clearDecision = decisionSender.clear;
  useEffect(() => {
    clearEdit();
    clearDecision();
  }, [selectedId, clearEdit, clearDecision]);

  // The review model for the selected unit: the point governing its file, the
  // blocks either side of it, what it said before, what the checks found, and
  // who decided last. The host assembles it, so the queue, an agent and the CLI
  // read one answer.
  const model = unit?.context;

  // The document to open the unit in is its SOURCE file, which carries the
  // source↔target toggle. The point names it project-relative; the queue's own
  // file is the target rendering, and answers until the model arrives.
  const documentFile = model?.point.path || selected?.file || "";

  // What the queue knows about the rest of this file: every unit listed here is
  // awaiting a decision, and the marked ones trip a check.
  const unitStates = useMemo(() => {
    if (!selected) return {};
    const out: Record<string, string> = {};
    for (const it of queue ?? []) {
      if (it.file !== selected.file || it.locale !== selected.locale) continue;
      out[it.key] = it.hasFindings ? t("findings") : t("awaiting review");
    }
    if (unit?.review_state) out[selected.key] = unit.review_state;
    return out;
  }, [queue, selected, unit?.review_state]);

  const move = useCallback(
    (delta: number) => {
      if (visible.length === 0) return;
      const next = Math.min(
        visible.length - 1,
        Math.max(0, (selectedIndex < 0 ? 0 : selectedIndex) + delta),
      );
      setSelectedId(itemId(visible[next]));
    },
    [visible, selectedIndex],
  );

  // A decision is a decide operation on the revision the page read, so it
  // binds to the text the reviewer had in front of them. A source unit has one
  // decision, approve: the source's establish. The decided unit leaves the
  // queue; the selection effect advances.
  const decide = useCallback(
    async (item: ReviewItem, decision: ReviewDecision, note?: string) => {
      if (!read || itemId(item) !== selectedId) return;
      setDeciding(true);
      try {
        const outcome = decision === "approved" ? "establish" : "reject";
        const res = await decisionSender.send(
          [decideOp(read.ref, read.rev, outcome)],
          decision === "rejected" ? note : undefined,
        );
        if (res?.status === "applied") {
          setQueue((q) => (q ?? []).filter((it) => itemId(it) !== itemId(item)));
        }
      } finally {
        setDeciding(false);
      }
    },
    [read, selectedId, decisionSender],
  );

  // Approve the text as it now stands, after the reviewer has seen it in the
  // stale prompt.
  const decideAgain = useCallback(async () => {
    if (!selected) return;
    setDeciding(true);
    try {
      const res = await decisionSender.reapply();
      if (res?.status === "applied") {
        setQueue((q) => (q ?? []).filter((it) => itemId(it) !== itemId(selected)));
      }
    } finally {
      setDeciding(false);
    }
  }, [selected, decisionSender]);

  const approve = useCallback(() => {
    if (!selected || deciding || !read) return;
    void decide(selected, "approved");
  }, [selected, deciding, read, decide]);

  const reject = useCallback(() => {
    if (!selected || selected.isSource || deciding || !read) return;
    setAskValue("");
    setAskText({
      kind: "reject",
      title: t("Send back to draft"),
      label: t("Why? The note travels with the unit."),
      placeholder: t("e.g. the tone is too formal for this surface"),
      confirm: t("Send back"),
      allowEmpty: true,
    });
  }, [selected, deciding, read]);

  // Per-unit AI actions — the only review paths that call a provider, and only
  // on explicit click. Fix/retranslate yield a PROPOSAL (diff + Accept/Discard);
  // explain yields text. Nothing is written until Accept, which sends the same
  // set_content a manual edit sends.
  const runAIAction = useCallback(
    async (action: ReviewAIActionKind, instructionOverride?: string) => {
      if (!selected || aiBusy) return;
      let instruction = instructionOverride ?? "";
      if (action === "retranslate" && instruction.trim() === "") {
        setAskValue("");
        setAskText({
          kind: "retranslate",
          title: t("Retranslate with AI"),
          label: t("What should change?"),
          placeholder: t("e.g. more informal, or keep it under 40 characters"),
          confirm: t("Retranslate"),
        });
        return;
      }
      setAIBusy(action);
      setAIExplanation(null);
      setAIExchanges([]);
      if (action !== "explain") setAIProposal(null);
      try {
        const run =
          onAIAction ??
          ((it: ReviewItem, act: ReviewAIActionKind, ins: string) =>
            api.reviewAIAction(tabID, it.locale, it.file, it.key, act, ins));
        const res = await run(selected, action, instruction);
        if (!res) return;
        setAIExchanges(res.exchanges ?? []);
        if (action === "explain") {
          setAIExplanation(res.explanation ?? "");
        } else if (res.proposed_target) {
          setAIProposal({ text: res.proposed_target, edit: res.proposed_edit });
        }
      } catch (err) {
        showError("AI action failed", err);
      } finally {
        setAIBusy(null);
      }
    },
    [selected, aiBusy, tabID, onAIAction, showError],
  );

  // Answer the pending question and run what it was asked for.
  const submitAsk = useCallback(() => {
    const ask = askText;
    if (!ask || !selected) return;
    const value = askValue;
    setAskText(null);
    if (ask.kind === "reject") {
      void decide(selected, "rejected", value);
      return;
    }
    void runAIAction("retranslate", value);
  }, [askText, askValue, selected, decide, runAIAction]);

  // Accept the AI proposal: a set_content of its edit text over the revision the
  // page read, the change a manual edit sends, so a translation that moved
  // meanwhile is asked about rather than overwritten.
  const acceptProposal = useCallback(async () => {
    if (!read || aiProposal === null) return;
    const content = editionContent(read);
    if (!content) return;
    const res = await editSender.send(
      setContentOps(content, [{ text: aiProposal.edit ?? aiProposal.text }]),
    );
    if (res?.status === "applied") setAIProposal(null);
  }, [read, aiProposal, editSender]);

  // AI pre-review modal: load the reviewer model for display when it opens.
  useEffect(() => {
    if (!preReviewOpen) return;
    void api.getDefaultModel().then((info) => {
      if (info) setReviewerModel(info.model || info.provider || "");
    });
  }, [preReviewOpen]);

  // The pre-review reads translations, so it never counts a source row.
  const preReviewPending = useMemo(() => {
    let items = (queue ?? []).filter((it) => !it.isSource);
    if (language) items = items.filter((it) => rowLanguage(it) === language);
    if (collectionFilter) items = items.filter((it) => (it.collection ?? "") === collectionFilter);
    return items;
  }, [queue, language, collectionFilter]);

  const runPreReview = useCallback(async () => {
    setPreReviewRunning(true);
    setPreReviewResult(null);
    try {
      const run =
        onPreReview ??
        ((locale: string, sc: PreReviewScope) => api.runAIPreReview(tabID, locale, sc));
      const res = await run(language, { collection: collectionFilter || undefined });
      setPreReviewResult(res ?? null);
      await refreshQueue();
    } catch (err) {
      showError("AI pre-review failed", err);
    } finally {
      setPreReviewRunning(false);
    }
  }, [tabID, onPreReview, language, collectionFilter, refreshQueue, showError]);

  // Keyboard-first: j/k navigate, a approve, r reject, space skip, e focuses
  // the target editor. Typing in the editor is left alone (Escape returns to
  // the queue).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName ?? "";
      // The editor is a contenteditable as well as the plural forms' textareas.
      const editable = !!el?.closest?.("[contenteditable='true']");
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT" || editable) {
        if (e.key === "Escape") el?.blur();
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      // The document view reads; it decides nothing. While it is open the
      // decision keys belong to it (Escape closes it) rather than to the queue
      // behind it.
      if (documentOpen) return;
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          move(1);
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          move(-1);
          break;
        case "a":
          e.preventDefault();
          approve();
          break;
        case "r":
          e.preventDefault();
          reject();
          break;
        case " ":
          if (tag === "BUTTON") return; // native activation
          e.preventDefault();
          move(1);
          break;
        case "e":
          e.preventDefault();
          document
            .querySelector<HTMLElement>(
              "[data-slot='review-target-editor'] [contenteditable='true'], [data-slot='review-target-editor'] textarea",
            )
            ?.focus();
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [move, approve, reject, documentOpen]);

  // Phase 2 batch: approve every clean unit in the current filter.
  const cleanVisible = useMemo(() => visible.filter((it) => it.hasFindings === false), [visible]);
  // Each unit is read as it is approved, so every approval binds to the text in
  // the file at that moment, and its checks run again over that text. A unit
  // whose text is no longer what its row lists, or that now trips a check,
  // stops the batch with its row brought up to date, so nothing is approved
  // that the reviewer has not seen; a unit whose approval is refused stops the
  // batch too, and each says why.
  const approveClean = useCallback(async () => {
    const targets = cleanVisible;
    if (targets.length === 0) return;
    const load =
      loadUnit ?? ((it: ReviewItem) => api.getReviewUnit(tabID, it.locale, it.file, it.key));
    setBatch({ done: 0, total: targets.length });
    try {
      for (let i = 0; i < targets.length; i++) {
        const item = targets[i];
        const r = await loadRead(item);
        if (!r) break;
        const d = await load(item);
        if (!d) break;
        const trips = d.findings.length > 0;
        if (trips || !stillListed(d.target, item.target)) {
          setQueue((q) =>
            (q ?? []).map((it) =>
              itemId(it) === itemId(item) ? { ...it, target: d.target, hasFindings: trips } : it,
            ),
          );
          showError(
            t("Batch approval stopped at {key}", { key: item.key }),
            new Error(
              trips
                ? t("The unit now trips a check. Review it before approving it.")
                : t("The unit changed since the queue listed it. Review the text as it stands."),
            ),
          );
          break;
        }
        const res = await client.apply({ ops: [decideOp(r.ref, r.rev, "establish")] });
        if (!res) break;
        if (res.status !== "applied") {
          showError(
            t("Batch approval stopped at {key}", { key: item.key }),
            new Error(refusalMessage(res)),
          );
          break;
        }
        setQueue((q) => (q ?? []).filter((it) => itemId(it) !== itemId(item)));
        setBatch({ done: i + 1, total: targets.length });
      }
    } catch (err) {
      showError("Batch approval stopped", err);
    } finally {
      setBatch(null);
    }
  }, [cleanVisible, loadRead, loadUnit, tabID, client, showError]);

  // Group the visible queue by file, then flatten to a single row stream
  // (a file header, then its units) so the left pane can be virtualized: a
  // review queue can reach thousands of units, and mounting every one is what
  // turns the pane into a multi-thousand-node scroll. The flat stream keeps the
  // grouped-by-file presentation while letting the virtualizer mount only the
  // rows near the viewport (it falls back to a full render for small queues and
  // in jsdom, where a viewport can't be measured).
  const rows = useMemo(() => {
    const m = new Map<string, ReviewItem[]>();
    for (const it of visible) {
      const g = m.get(it.file);
      if (g) g.push(it);
      else m.set(it.file, [it]);
    }
    const out: Array<
      { kind: "header"; file: string; count: number } | { kind: "item"; item: ReviewItem }
    > = [];
    for (const [file, items] of m.entries()) {
      out.push({ kind: "header", file, count: items.length });
      for (const item of items) out.push({ kind: "item", item });
    }
    return out;
  }, [visible]);

  // What the Active Filter is holding back, said in the reviewer's terms. A
  // queue that is quietly smaller than the project reads as a shorter queue,
  // not a narrowed one, and the control that narrowed it is on another menu.
  const filterNarrowing = useMemo(() => {
    if (!activeFilter) return "";
    const parts: string[] = [];
    if (activeFilter.languages?.length) {
      // Wrapped rather than passed point-free: `localeLabel` takes the UI
      // language second, and `map` would hand it the array index.
      parts.push(activeFilter.languages.map((l) => localeLabel(l)).join(", "));
    }
    if (activeFilter.collections?.length) {
      parts.push(activeFilter.collections.join(", "));
    }
    if (activeFilter.glob?.trim()) parts.push(activeFilter.glob.trim());
    if (parts.length === 0) return "";
    return `${activeFilter.name || t("the active filter")}: ${parts.join(" · ")}`;
  }, [activeFilter]);

  // Whether the translation can be edited here, and the source it answers to:
  // its words and codes for the editor's tag palette.
  const canEdit = !!read && read.ops.includes("set_content");
  const sourceReference = useMemo(() => {
    if (!read?.editions) return undefined;
    const src = (unit?.source_locale ?? selected?.sourceLocale ?? "").toLowerCase();
    const key = Object.keys(read.editions).find((k) => k.toLowerCase() === src);
    return key ? { text: read.editions[key].text, codes: read.codes } : undefined;
  }, [read, unit?.source_locale, selected?.sourceLocale]);

  const chips: Array<{ id: Chip; label: string }> = [
    { id: "all", label: t("All") },
    { id: "findings", label: t("With findings") },
    { id: "clean", label: t("Clean") },
  ];

  return (
    <div className="flex h-full flex-col p-6" data-slot="review-page">
      {/* Header: title + queue filters */}
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <div>
          <h1 className="text-lg font-semibold">{t("Review")}</h1>
          <p className="text-xs text-muted-foreground">
            {t(
              "Approve, edit, or send translations back. A decision binds to the exact text it judged.",
            )}
          </p>
          {filterNarrowing && (
            <p className="mt-1 text-xs text-muted-foreground" data-slot="review-filter-notice">
              {t("Showing {scope}.", { scope: filterNarrowing })}{" "}
              <span className="opacity-70">{t("Change it in the filter menu, top right.")}</span>
            </p>
          )}
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <ReviewLanguageSelect
            value={language || ALL_LANGUAGES}
            onChange={(next) => setLanguage(next === ALL_LANGUAGES ? "" : next)}
            lanes={languages}
            allowAll
            allPending={(queue ?? []).length}
            size="xs"
            data-slot="review-language-select"
          />
          <div className="flex items-center gap-1" data-slot="review-chips">
            {chips.map((c) => (
              <Button
                key={c.id}
                variant={chip === c.id ? "default" : "outline"}
                size="xs"
                onClick={() => setChip(c.id)}
                aria-pressed={chip === c.id}
              >
                {c.label}
              </Button>
            ))}
          </div>
          {collections.length > 1 && (
            <select
              className="h-7 rounded-md border border-input bg-transparent px-2 text-xs"
              value={collectionFilter}
              onChange={(e) => setCollectionFilter(e.target.value)}
              aria-label={t("Filter by collection")}
              data-slot="review-collection-filter"
            >
              <option value="">{t("All collections")}</option>
              {collections.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          )}
          <Button
            variant="outline"
            size="xs"
            onClick={() => {
              setPreReviewResult(null);
              setPreReviewOpen(true);
            }}
            disabled={loadingQueue || preReviewPending.length === 0}
            data-slot="review-prereview-open"
          >
            <Sparkles size={12} />
            {t("AI pre-review…")}
          </Button>
          <Button
            variant="outline"
            size="xs"
            onClick={() => void refreshQueue()}
            disabled={loadingQueue}
            aria-label={t("Refresh the review queue")}
          >
            {loadingQueue ? (
              <Loader2 size={12} className="animate-spin" />
            ) : (
              <RefreshCw size={12} />
            )}
          </Button>
        </div>
      </div>

      {/* AI pre-review modal: reviewer model, policy (annotate-only default),
          scope summary, unit count; then progress and the result summary. */}
      <Dialog
        open={preReviewOpen}
        onOpenChange={(o) => {
          if (!o && !preReviewRunning) setPreReviewOpen(false);
        }}
      >
        <DialogContent
          className="w-[26rem] max-w-[90vw] sm:max-w-[26rem]"
          showCloseButton={false}
          data-slot="review-prereview-modal"
        >
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-sm font-semibold">
              <Sparkles size={14} className="text-primary" />
              {t("AI pre-review")}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3 text-sm">
            <div className="space-y-1 text-xs text-muted-foreground">
              <div>
                {t("Reviewer model")}:{" "}
                <span className="text-foreground" translate="no">
                  {reviewerModel || t("project default")}
                </span>
              </div>
              <div>
                {t("Scope")}:{" "}
                <span className="text-foreground">
                  {language ? localeLabel(language) : t("all languages")}
                  {collectionFilter ? ` · ${collectionFilter}` : ""}
                </span>{" "}
                · {t("{count} pending units", { count: preReviewPending.length })}
              </div>
            </div>
            <p className="text-xs text-muted-foreground" data-slot="review-prereview-policy">
              {t("The model stores a score and findings on each unit; every decision stays yours.")}
            </p>
            {preReviewResult && (
              <div
                className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs"
                data-slot="review-prereview-result"
              >
                {t("{count} units scored", { count: preReviewResult.reviewed })}
                {preReviewResult.skipped ? (
                  <span className="text-muted-foreground">
                    {" "}
                    ({t("{count} skipped", { count: preReviewResult.skipped })})
                  </span>
                ) : null}
              </div>
            )}
            <div className="flex items-center justify-end gap-2 pt-1">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPreReviewOpen(false)}
                disabled={preReviewRunning}
              >
                {preReviewResult ? t("Close") : t("Cancel")}
              </Button>
              {!preReviewResult && (
                <Button
                  size="sm"
                  onClick={() => void runPreReview()}
                  disabled={preReviewRunning || preReviewPending.length === 0}
                  data-slot="review-prereview-run"
                >
                  {preReviewRunning ? (
                    <>
                      <Loader2 size={12} className="animate-spin" />
                      {t("Reviewing…")}
                    </>
                  ) : (
                    t("Run pre-review")
                  )}
                </Button>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* The one-field question Reject and Retranslate ask. window.prompt does
          not work in the app's webview, so this is the only version that runs
          at all. */}
      <Dialog open={askText !== null} onOpenChange={(o) => !o && setAskText(null)}>
        <DialogContent
          className="w-[26rem] max-w-[90vw] sm:max-w-[26rem]"
          data-slot="review-ask-dialog"
        >
          <DialogHeader>
            <DialogTitle className="text-sm font-semibold">{askText?.title}</DialogTitle>
          </DialogHeader>
          <label className="space-y-1.5 text-xs">
            <span className="text-muted-foreground">{askText?.label}</span>
            <textarea
              autoFocus
              className="min-h-20 w-full resize-y rounded-md border bg-background p-2 text-sm"
              value={askValue}
              placeholder={askText?.placeholder}
              onChange={(e) => setAskValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault();
                  submitAsk();
                }
              }}
              data-slot="review-ask-input"
            />
          </label>
          <DialogFooter>
            <Button variant="outline" size="xs" onClick={() => setAskText(null)}>
              {t("Cancel")}
            </Button>
            <Button
              size="xs"
              onClick={submitAsk}
              disabled={!askText?.allowEmpty && askValue.trim() === ""}
              data-slot="review-ask-confirm"
            >
              {askText?.confirm}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Batch bar: approve every clean unit in the current view. Source rows
          carry no findings enrichment, so they never count here. */}
      {cleanVisible.length > 0 && (
        <div
          className="mb-3 flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-1.5 text-xs"
          data-slot="review-batch"
        >
          <CheckCheck size={13} className="shrink-0 text-muted-foreground" />
          <span className="text-muted-foreground">
            {t(
              "{count, plural, one {# clean unit (no findings) in this view} other {# clean units (no findings) in this view}}",
              {
                count: cleanVisible.length,
              },
            )}
          </span>
          <Button
            size="xs"
            className="ml-auto"
            disabled={batch !== null || deciding}
            onClick={() => void approveClean()}
            data-slot="review-batch-approve"
          >
            {batch ? (
              <>
                <Loader2 size={12} className="animate-spin" />
                {t("Approving {done} of {total}…", { done: batch.done, total: batch.total })}
              </>
            ) : (
              <>
                <Check size={12} />
                {t("{count, plural, one {Approve # clean unit} other {Approve # clean units}}", {
                  count: cleanVisible.length,
                })}
              </>
            )}
          </Button>
        </div>
      )}

      {queueWarnings.length > 0 && (
        <Card className="mb-3 border-amber-500/40" data-slot="review-unread">
          <CardContent className="p-3">
            <p className="mb-1 text-sm font-medium">{t("Some declared content was not read")}</p>
            <ul className="space-y-1 text-xs text-muted-foreground">
              {queueWarnings.map((warning) => (
                <li key={`${warning.source}#${warning.code}`}>{warning.message}</li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      {loadingQueue && !queue ? (
        <div className="p-4 text-sm text-muted-foreground">{t("Loading review queue…")}</div>
      ) : visible.length === 0 ? (
        <Card className="border-dashed" data-slot="review-empty">
          <CardContent className="p-10 text-center">
            {queueWarnings.length === 0 && (
              <CheckCircle2 size={24} className="mx-auto mb-2 text-success" />
            )}
            <p className="text-sm text-muted-foreground">
              {(queue ?? []).length === 0
                ? queueWarnings.length > 0
                  ? t("Nothing to review in the content this project could read.")
                  : t("Review queue empty. Every translated unit is established.")
                : t("Nothing matches this filter.")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-[280px_minmax(0,1fr)] gap-4">
          {/* Left: the queue, grouped by file. Virtualized once it grows past a
              handful of units (shared @neokapi/editor-grid VirtualList) so a
              thousands-strong queue never mounts a thousands-strong DOM; the
              viewport is bounded so the page never becomes the scroll surface.
              The primitive owns the positioned, measured row wrapper; the queue
              rows (file header or unit button) render unchanged inside it. */}
          <VirtualList
            items={rows}
            estimateSize={44}
            overscan={16}
            className="min-h-0 flex-1 rounded-md border p-2"
            dataSlot="review-queue"
            renderRow={(row, { key, rowProps }) => (
              <div key={key} {...rowProps}>
                {row.kind === "header" ? (
                  <div className="mb-1 mt-2 flex items-center gap-1.5 px-1 text-[11px] font-medium text-muted-foreground first:mt-0">
                    <FileText size={11} />
                    <SimpleTooltip content={row.file}>
                      <span className="truncate" translate="no">
                        {shortFile(row.file)}
                      </span>
                    </SimpleTooltip>
                    <span className="text-muted-foreground/60">· {row.count}</span>
                  </div>
                ) : (
                  (() => {
                    const it = row.item;
                    const id = itemId(it);
                    const active = id === selectedId;
                    return (
                      <button
                        className={`flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors ${
                          active ? "bg-primary/10" : "hover:bg-accent"
                        }`}
                        onClick={() => setSelectedId(id)}
                        data-slot="review-queue-item"
                        data-active={active || undefined}
                        data-key={it.key}
                        data-language={rowLanguage(it)}
                        data-source={it.isSource || undefined}
                      >
                        {/* A source row carries no findings enrichment, so it
                            takes the neutral marker and says what it is in the
                            language affix beside its key. */}
                        {it.isSource ? (
                          <Circle
                            size={8}
                            className="mt-1 shrink-0 text-muted-foreground/40"
                            aria-label={t("Source wording")}
                          />
                        ) : it.hasFindings ? (
                          <Circle
                            size={8}
                            className="mt-1 shrink-0 fill-warning text-warning"
                            aria-label={t("Has findings")}
                          />
                        ) : (
                          <Circle
                            size={8}
                            className={`mt-1 shrink-0 ${it.hasFindings === false ? "text-success" : "text-muted-foreground/40"}`}
                            aria-label={it.hasFindings === false ? t("Clean") : t("Not checked")}
                          />
                        )}
                        <span className="min-w-0 flex-1">
                          <span className="flex items-center gap-1.5">
                            {it.held && (
                              <PauseCircle
                                size={11}
                                className="shrink-0 text-warning"
                                aria-label={t("holding every language")}
                              />
                            )}
                            <span className="truncate font-medium" translate="no">
                              {it.key}
                            </span>
                            {/* Every row says which language it belongs to,
                                because the queue holds them all at once, and a
                                source row wears the same source marker the
                                selector and the detail pane use. */}
                            <LocaleLabel locale={rowLanguage(it)} compact source={it.isSource} />
                            {it.aiScore !== undefined && (
                              <SimpleTooltip
                                content={
                                  it.aiModel
                                    ? t("AI review score ({model})", { model: it.aiModel })
                                    : t("AI review score")
                                }
                              >
                                <span
                                  className="rounded bg-muted px-1 text-[10px] tabular-nums text-muted-foreground"
                                  data-slot="review-queue-ai-score"
                                >
                                  {t("ai {score}", { score: it.aiScore })}
                                </span>
                              </SimpleTooltip>
                            )}
                          </span>
                          <SimpleTooltip content={it.source}>
                            <span
                              className="block truncate text-muted-foreground"
                              {...directionAttrs(it.sourceLocale)}
                            >
                              {it.source}
                            </span>
                          </SimpleTooltip>
                        </span>
                      </button>
                    );
                  })()
                )}
              </div>
            )}
          />

          {/* Center: the unit. The point it sits at, the source wording and
              the translation under each language's own name, the document
              around it, what was approved before, the checks, the provenance.
              A source row takes the same header and its own pane below it. */}
          <div className="flex min-h-0 flex-col" data-slot="review-unit">
            <ScrollArea className="min-h-0 flex-1">
              <div className="space-y-3 pr-3">
                {selected && (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <SimpleTooltip content={`${selected.file}:${selected.key}`}>
                      <span className="truncate" translate="no">
                        {selected.file}:{selected.key}
                      </span>
                    </SimpleTooltip>
                    <LocaleLabel locale={selected.locale} compact source={selected.isSource} />
                    {selected.isSource
                      ? selected.status && (
                          <StatusBadge
                            ladder="source"
                            status={selected.status}
                            compact
                            data-slot="review-status"
                          />
                        )
                      : unit?.status && (
                          <StatusBadge
                            ladder="content"
                            status={unit.status}
                            compact
                            data-slot="review-status"
                          />
                        )}
                    {unitLoading && <Loader2 size={12} className="animate-spin" />}
                    <Button
                      variant="outline"
                      size="xs"
                      className="ml-auto"
                      onClick={() => setDocumentOpen(true)}
                      disabled={!documentFile}
                      data-slot="review-open-document"
                    >
                      <BookOpen size={12} />
                      {t("Open in document")}
                    </Button>
                  </div>
                )}

                {selected?.isSource ? (
                  <>
                    <SourceUnitPane
                      tabID={tabID}
                      item={selected}
                      read={read}
                      readError={readError}
                      sender={editSender}
                      saved={lastSaved}
                      loadContext={loadSourceContext}
                    />
                    <ChangesCard history={history} loading={historyLoading} />
                  </>
                ) : (
                  <>
                    <PointCard point={model?.point} loading={unitLoading} />

                    <Card>
                      <CardContent className="p-3">
                        {/* Headed by the language, named. A reviewer reads "French
                        (France)", never a code in capitals. */}
                        <div className="mb-1 text-[11px] font-medium text-muted-foreground">
                          <LocaleLabel
                            locale={unit?.source_locale ?? selected?.sourceLocale ?? ""}
                            source
                            data-slot="review-source-language"
                          />
                        </div>
                        <div
                          className="whitespace-pre-wrap text-sm"
                          data-slot="review-source"
                          translate="no"
                          {...directionAttrs(unit?.source_locale)}
                        >
                          {unit?.source ?? selected?.source ?? ""}
                        </div>
                      </CardContent>
                    </Card>

                    <Card>
                      <CardContent className="p-3">
                        <div className="mb-1 flex items-center gap-2 text-[11px] font-medium text-muted-foreground">
                          <LocaleLabel
                            locale={unit?.locale ?? selected?.locale ?? ""}
                            data-slot="review-target-language"
                          />
                        </div>
                        {readError ? (
                          <div className="space-y-1">
                            <div className="whitespace-pre-wrap text-sm" translate="no">
                              {unit?.target ?? selected?.target ?? ""}
                            </div>
                            <p className="text-xs text-destructive" data-slot="review-read-error">
                              {readError}
                            </p>
                          </div>
                        ) : (
                          <EditionEditPanel
                            sender={editSender}
                            content={read ? editionContent(read) : null}
                            locale={unit?.locale ?? selected?.locale}
                            reference={sourceReference}
                            readOnlyReason={
                              read && !read.ops.includes("set_content")
                                ? t("This format does not write a change to this text.")
                                : undefined
                            }
                            saveLabel={t("Save & re-check")}
                            compact
                            autoFocus={false}
                            data-slot="review-target"
                            actions={
                              /* Per-unit AI actions (phase 3): explicit clicks only. */
                              <div
                                className="flex flex-wrap items-center gap-2"
                                data-slot="review-ai-actions"
                              >
                                <Button
                                  variant="outline"
                                  size="xs"
                                  onClick={() => void runAIAction("fix-findings")}
                                  disabled={!canEdit || aiBusy !== null || editSender.busy}
                                  data-slot="review-ai-fix"
                                >
                                  {aiBusy === "fix-findings" ? (
                                    <Loader2 size={12} className="animate-spin" />
                                  ) : (
                                    <Sparkles size={12} />
                                  )}
                                  {t("Fix with AI")}
                                </Button>
                                <Button
                                  variant="outline"
                                  size="xs"
                                  onClick={() => void runAIAction("retranslate")}
                                  disabled={!canEdit || aiBusy !== null || editSender.busy}
                                  data-slot="review-ai-retranslate"
                                >
                                  {aiBusy === "retranslate" ? (
                                    <Loader2 size={12} className="animate-spin" />
                                  ) : (
                                    <Sparkles size={12} />
                                  )}
                                  {t("Retranslate…")}
                                </Button>
                                <Button
                                  variant="outline"
                                  size="xs"
                                  onClick={() => void runAIAction("explain")}
                                  disabled={!unit || aiBusy !== null}
                                  data-slot="review-ai-explain"
                                >
                                  {aiBusy === "explain" ? (
                                    <Loader2 size={12} className="animate-spin" />
                                  ) : (
                                    <Sparkles size={12} />
                                  )}
                                  {t("Explain")}
                                </Button>
                              </div>
                            }
                          />
                        )}
                      </CardContent>
                    </Card>

                    {/* AI proposal: current vs proposed, Accept / Discard. Accept
                    routes through the same save path as a manual edit. */}
                    {aiProposal !== null && unit && (
                      <Card data-slot="review-ai-proposal">
                        <CardContent className="space-y-2 p-3">
                          <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                            {t("AI proposal")}
                          </div>
                          <div className="space-y-1 text-sm">
                            {/* The wording in force, drawn neutrally: a proposal is
                            an offer, and painting the reviewer's own text red
                            says a check rejected it. */}
                            <div className="rounded-md border bg-muted/40 px-2 py-1">
                              <span className="mr-1 text-[10px] uppercase text-muted-foreground">
                                {t("Current")}
                              </span>
                              <span
                                className="whitespace-pre-wrap"
                                translate="no"
                                {...directionAttrs(unit.locale)}
                              >
                                {unit.target}
                              </span>
                            </div>
                            <div className="rounded-md border border-primary/30 bg-primary/5 px-2 py-1">
                              <span className="mr-1 text-[10px] uppercase text-muted-foreground">
                                {t("Proposed")}
                              </span>
                              {aiProposal.edit !== undefined ? (
                                <EditTextDisplay
                                  text={aiProposal.edit}
                                  codes={read?.codes}
                                  locale={unit.locale}
                                  className="whitespace-pre-wrap"
                                />
                              ) : (
                                <span
                                  className="whitespace-pre-wrap"
                                  translate="no"
                                  {...directionAttrs(unit.locale)}
                                >
                                  {aiProposal.text}
                                </span>
                              )}
                            </div>
                          </div>
                          <div className="flex items-center gap-2">
                            <Button
                              size="xs"
                              onClick={() => void acceptProposal()}
                              disabled={editSender.busy}
                              data-slot="review-ai-accept"
                            >
                              {editSender.busy ? (
                                <Loader2 size={12} className="animate-spin" />
                              ) : (
                                <Check size={12} />
                              )}
                              {t("Accept")}
                            </Button>
                            <Button
                              variant="outline"
                              size="xs"
                              onClick={() => setAIProposal(null)}
                              disabled={editSender.busy}
                              data-slot="review-ai-discard"
                            >
                              <X size={12} />
                              {t("Discard")}
                            </Button>
                          </div>
                          <AIExchangeDisclosure entries={aiExchanges} />
                        </CardContent>
                      </Card>
                    )}

                    {/* AI explanation (read-only). */}
                    {aiExplanation !== null && (
                      <Card data-slot="review-ai-explanation">
                        <CardContent className="space-y-1 p-3">
                          <div className="flex items-center justify-between">
                            <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                              {t("AI explanation")}
                            </div>
                            <Button
                              variant="ghost"
                              size="xs"
                              onClick={() => setAIExplanation(null)}
                            >
                              <X size={12} />
                            </Button>
                          </div>
                          <div className="whitespace-pre-wrap text-xs">{aiExplanation}</div>
                          <AIExchangeDisclosure entries={aiExchanges} />
                        </CardContent>
                      </Card>
                    )}

                    {/* The unit in its document: the blocks either side of it, in
                    document order, as the translate prompt read them. */}
                    <NeighbourhoodCard
                      neighbourhood={model?.neighbourhood}
                      unitKey={selected?.key}
                      unitSource={unit?.source ?? selected?.source}
                      unitTarget={unit?.target ?? selected?.target}
                      sourceLocale={unit?.source_locale ?? selected?.sourceLocale}
                      locale={unit?.locale ?? selected?.locale}
                      loading={unitLoading}
                    />

                    {/* What was approved for this unit before, and the wording the
                    content memory already holds for it. */}
                    <HistoryCard
                      history={model?.history}
                      emptyText={t(
                        "Nothing has been approved for this unit yet, and the content memory holds no close match.",
                      )}
                      sourceLocale={unit?.source_locale ?? selected?.sourceLocale}
                      locale={unit?.locale ?? selected?.locale}
                      loading={unitLoading}
                      fallbackMemoryScore={unit?.memory_score}
                    />

                    {/* What has already been said about this translation: the
                    checks, and the AI pre-review that scored it. */}
                    <JudgementCard
                      findings={unit ? checkFindingViews(unit.findings ?? []) : undefined}
                      aiScore={model?.judgement.ai_score ?? unit?.ai_review_score}
                      aiModel={model?.judgement.ai_model ?? unit?.ai_review_model}
                      aiFindings={model?.judgement.ai_findings}
                    />

                    {/* Where this translation came from, and the decision in force.
                    Until the model arrives the queue row's own flat fields stand in. */}
                    <ProvenanceCard
                      provenance={
                        model?.provenance ?? {
                          origin: unit?.origin,
                          review_state: unit?.review_state,
                          note: unit?.note,
                        }
                      }
                    />

                    {/* Every recorded change to this translation. */}
                    <ChangesCard history={history} loading={historyLoading} />
                  </>
                )}
              </div>
            </ScrollArea>

            {/* A decision on text that changed since the page read it: the text
                as it stands, and the reviewer's choice. */}
            {decisionSender.stale && (
              <div className="mt-3">
                <StalePrompt
                  current={decisionSender.stale.current}
                  codes={read?.codes}
                  locale={selected?.isSource ? selected.sourceLocale : selected?.locale}
                  busy={deciding}
                  reapplyLabel={
                    decisionSender.stale.ops.some(
                      (o) => o.op === "decide" && o.outcome === "reject",
                    )
                      ? t("Send back the text as it stands")
                      : t("Approve the text as it stands")
                  }
                  onReapply={() => void decideAgain()}
                  onDiscard={() => void decisionSender.discard()}
                />
              </div>
            )}
            {decisionSender.error && (
              <p
                className="mt-2 text-xs text-destructive"
                role="alert"
                data-slot="review-decision-refused"
              >
                {decisionSender.error}
              </p>
            )}

            {/* Action bar: the keyboard verbs, spelled out. A source unit has
                one decision, so Reject is absent on a source row and `r`
                answers nothing there. */}
            <div
              className="mt-3 flex flex-wrap items-center gap-2 rounded-md border bg-muted/30 px-3 py-2"
              data-slot="review-actions"
            >
              <Button
                variant="success"
                size="sm"
                onClick={approve}
                disabled={!selected || deciding || !read}
                data-slot="review-approve"
              >
                <Check size={13} />
                {selected?.isSource ? t("Approve source") : t("Approve")}
                <kbd className="ml-1 rounded bg-black/10 px-1 text-[10px]">a</kbd>
              </Button>
              {/* Reject takes destructive, which is where the shared scale puts
                  it (packages/ui/docs/judgement-colours.md): it undoes the
                  translation in force rather than accepting it. */}
              {!selected?.isSource && (
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={reject}
                  disabled={!selected || deciding || !read}
                  data-slot="review-reject"
                >
                  <X size={13} />
                  {t("Reject")}
                  <kbd className="ml-1 rounded bg-black/10 px-1 text-[10px]">r</kbd>
                </Button>
              )}
              <div className="ml-auto flex items-center gap-3 text-[11px] text-muted-foreground">
                <span>
                  <kbd className="rounded bg-muted px-1">j</kbd>/
                  <kbd className="rounded bg-muted px-1">k</kbd> {t("navigate")}
                </span>
                <span>
                  <kbd className="rounded bg-muted px-1">e</kbd> {t("edit")}
                </span>
                <span>
                  <kbd className="rounded bg-muted px-1">space</kbd> {t("skip")}
                </span>
                {deciding && <Loader2 size={12} className="animate-spin" />}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* The unit in its document. The queue stays mounted behind the sheet, so
          closing it lands back on this unit with the list where it was. */}
      <FilePreview
        tabID={tabID}
        filePath={documentOpen && selected ? documentFile : null}
        filename={documentFile}
        focusKey={selected?.key ?? null}
        unitStates={unitStates}
        side={selected?.isSource ? "source" : selected?.locale}
        backLabel={t("Back to the queue")}
        onClose={() => setDocumentOpen(false)}
      />
    </div>
  );
}
