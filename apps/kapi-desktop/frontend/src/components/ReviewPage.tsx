import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  BookOpen,
  Check,
  CheckCheck,
  CheckCircle2,
  Circle,
  FileText,
  Loader2,
  RefreshCw,
  Sparkles,
  Undo2,
  X,
} from "lucide-react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  LocalePill,
  LocaleSelect,
  localeLabel,
  resolveLocaleName,
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
import { SourceLane } from "./SourceLane";
import { AIPreReview } from "./review/AIPreReview";
import { HistoryCard } from "./review/HistoryCard";
import { NeighbourhoodCard } from "./review/NeighbourhoodCard";
import { PointRail } from "./review/PointRail";
import { ProvenanceCard } from "./review/ProvenanceCard";
import { useActiveFilter } from "../context/ActiveFilterContext";
import type {
  DesktopFinding,
  PreReviewPolicy,
  PreReviewResult,
  PreReviewScope,
  ReviewAIActionKind,
  ReviewAIActionResult,
  ReviewItem,
  ReviewUnitDetail,
  AIActivityEntry,
  SourceQueueItem,
} from "../types/api";

/** Initial queue narrowing handed in by an entry point (a ship-gate cell or a
 *  timeline tag on the Collections surface). */
export interface ReviewScope {
  collection?: string;
  locale?: string;
}

export type ReviewDecision = "approved" | "rejected" | "signed-off";

export interface ReviewPageProps {
  tabID: string;
  /** Narrow the queue to a (collection, locale) on entry. */
  scope?: ReviewScope;
  /** Pre-loaded queue for Storybook/tests — skips api.getReviewQueue(). */
  items?: ReviewItem[];
  /** Override the unit loader (Storybook/tests); defaults to api.getReviewUnit. */
  loadUnit?: (item: ReviewItem) => Promise<ReviewUnitDetail | null>;
  /** Override the decision recorder (Storybook/tests); defaults to the Wails calls. */
  onDecide?: (item: ReviewItem, decision: ReviewDecision, note?: string) => Promise<void>;
  /** Override the target-save handler (Storybook/tests); defaults to api.updateReviewTarget. */
  onSaveTarget?: (item: ReviewItem, text: string) => Promise<void>;
  /** Override the per-unit AI action (Storybook/tests); defaults to api.reviewAIAction. */
  onAIAction?: (
    item: ReviewItem,
    action: ReviewAIActionKind,
    instruction: string,
  ) => Promise<ReviewAIActionResult | null>;
  /** Override the pre-review runner (Storybook/tests); defaults to api.runAIPreReview. */
  onPreReview?: (
    locale: string,
    scope: PreReviewScope,
    policy: PreReviewPolicy,
  ) => Promise<PreReviewResult | null>;
}

type Chip = "all" | "findings" | "clean";
type Lane = "target" | "source";

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

/** Queue ordering: findings first, then file, then key (Phase 2). */
function orderItems(items: ReviewItem[]): ReviewItem[] {
  return [...items].sort((a, b) => {
    const fa = a.hasFindings ? 0 : 1;
    const fb = b.hasFindings ? 0 : 1;
    if (fa !== fb) return fa - fb;
    if (a.file !== b.file) return a.file.localeCompare(b.file);
    if (a.key !== b.key) return a.key.localeCompare(b.key);
    return a.locale.localeCompare(b.locale);
  });
}

function severityBadgeClass(severity: string): string {
  switch (severity) {
    case "critical":
      return "border-destructive/40 bg-destructive/10 text-destructive";
    case "major":
      return "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400";
    case "minor":
      return "border-amber-500/30 bg-amber-500/5 text-amber-600/90 dark:text-amber-400/90";
    default:
      return "text-muted-foreground";
  }
}

/**
 * The translation review surface (issue #1077, phases 1–3): a keyboard-first
 * three-pane page — the queue on the left (findings-first, filterable by
 * findings/locale/collection), the unit in the center (read-only SOURCE,
 * editable TARGET), and the unit's CHECKS findings + CONTEXT (state, note,
 * provenance, content-memory match, AI review score) below. Every decision (a approve /
 * r reject / s sign off) records through cli.ApplyReviewDecision, hash-bound
 * to the translation it judged; editing the target re-runs the unit's checks
 * and re-bases the next approval on the new text.
 *
 * Phase 3 adds AI: per-unit actions (Fix with AI / Retranslate… / Explain)
 * that yield a proposal diff — nothing is written until Accept, which routes
 * through the same save path as a manual edit — and the AI pre-review modal
 * (annotate-only by default; optional auto-approve recorded as ai/<model>,
 * which human-required gates deliberately ignore). Provider calls happen only
 * on these explicit clicks, never while listing or loading the queue.
 */
export function ReviewPage({
  tabID,
  scope,
  items: propItems,
  loadUnit,
  onDecide,
  onSaveTarget,
  onAIAction,
  onPreReview,
}: ReviewPageProps) {
  const { showError } = useError();
  const [queue, setQueue] = useState<ReviewItem[] | null>(propItems ?? null);
  const [loadingQueue, setLoadingQueue] = useState(!propItems);
  const [chip, setChip] = useState<Chip>("all");
  const [localeFilter, setLocaleFilter] = useState(scope?.locale ?? "");
  const [collectionFilter, setCollectionFilter] = useState(scope?.collection ?? "");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [unit, setUnit] = useState<ReviewUnitDetail | null>(null);
  const [unitLoading, setUnitLoading] = useState(false);
  const [editText, setEditText] = useState("");
  const [saving, setSaving] = useState(false);
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
  // The two lanes of one page. Target review asks whether a translation is right
  // for its source; source review asks the question underneath it, once for
  // every language rather than once per language.
  const [lane, setLane] = useState<Lane>("target");
  // Owned here rather than inside the lane, so the toggle can carry the count.
  // A source hold is the reason a whole project's translations are not moving,
  // and it was reaching the app already: SourceCoverage rides in every
  // convergence report and nothing rendered it, so the one number that explains
  // a stalled run sat in memory and off the screen.
  const [sourceQueue, setSourceQueue] = useState<SourceQueueItem[] | null>(null);
  // window.prompt is a no-op in the app's webview: it returns null without ever
  // showing anything, and both callers read null as "cancelled". Retranslate and
  // Reject therefore did nothing at all, silently, with no error to explain it.
  // These drive an in-app dialog instead, which is also the only version a test
  // can drive.
  const [askText, setAskText] = useState<AskPrompt | null>(null);
  const [askValue, setAskValue] = useState("");
  const [aiProposal, setAIProposal] = useState<string | null>(null);
  const [aiExplanation, setAIExplanation] = useState<string | null>(null);
  // The calls the last AI action made. Held beside its result so the disclosure
  // under a proposal shows the prompt that produced THAT proposal.
  const [aiExchanges, setAIExchanges] = useState<AIActivityEntry[]>([]);
  // AI pre-review modal state.
  const [preReviewOpen, setPreReviewOpen] = useState(false);
  const [preReviewAuto, setPreReviewAuto] = useState(false);
  const [preReviewMinScore, setPreReviewMinScore] = useState(90);
  const [preReviewRunning, setPreReviewRunning] = useState(false);
  const [preReviewResult, setPreReviewResult] = useState<PreReviewResult | null>(null);
  const [reviewerModel, setReviewerModel] = useState<string>("");
  // The read-only document view, opened at the selected unit. It reads the file
  // and never commits: approve, reject, retranslate and sign off stay here on
  // the queue, and closing it returns to the queue with this unit still
  // selected and the list where it was.
  const [documentOpen, setDocumentOpen] = useState(false);
  const editRef = useRef<HTMLTextAreaElement>(null);

  const refreshQueue = useCallback(async () => {
    if (propItems) {
      setQueue(propItems);
      setLoadingQueue(false);
      return;
    }
    setLoadingQueue(true);
    try {
      const items = await api.getReviewQueue(tabID, activeFilter ?? { id: "", name: "" });
      setQueue(items ?? []);
    } catch (err) {
      showError("Failed to load the review queue", err);
      setQueue([]);
    } finally {
      setLoadingQueue(false);
    }
  }, [tabID, propItems, activeFilter, showError]);

  useEffect(() => {
    void refreshQueue();
  }, [refreshQueue]);

  const refreshSourceQueue = useCallback(async () => {
    if (propItems) return; // Storybook/tests drive the lane directly.
    try {
      setSourceQueue((await api.getSourceQueue(tabID, activeFilter ?? { id: "", name: "" })) ?? []);
    } catch {
      // The source lane is a second view of the same project. A failure to read
      // it must not take down the target queue the reviewer came here for.
      setSourceQueue([]);
    }
  }, [tabID, activeFilter, propItems]);

  useEffect(() => {
    void refreshSourceQueue();
  }, [refreshSourceQueue]);

  // Filter options derived from the full queue.
  const locales = useMemo(
    () => Array.from(new Set((queue ?? []).map((it) => it.locale))).sort(),
    [queue],
  );
  // The picker takes names, not codes: a reviewer choosing between fr and ar
  // reads "French" and "Arabic".
  const localeOptions = useMemo(
    () => locales.map((code) => ({ code, displayName: resolveLocaleName(code) })),
    [locales],
  );
  const collections = useMemo(
    () =>
      Array.from(new Set((queue ?? []).map((it) => it.collection ?? "").filter(Boolean))).sort(),
    [queue],
  );

  // The visible queue: scope filters + findings chip, findings-first order.
  const visible = useMemo(() => {
    let items = queue ?? [];
    if (localeFilter) items = items.filter((it) => it.locale === localeFilter);
    if (collectionFilter) items = items.filter((it) => (it.collection ?? "") === collectionFilter);
    if (chip === "findings") items = items.filter((it) => it.hasFindings === true);
    if (chip === "clean") items = items.filter((it) => it.hasFindings === false);
    return orderItems(items);
  }, [queue, localeFilter, collectionFilter, chip]);

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
    if (!selected) {
      setUnit(null);
      setEditText("");
      return;
    }
    setUnit(null);
    setEditText(selected.target ?? "");
    let cancelled = false;
    setUnitLoading(true);
    const load =
      loadUnit ?? ((it: ReviewItem) => api.getReviewUnit(tabID, it.locale, it.file, it.key));
    load(selected)
      .then((d) => {
        if (cancelled) return;
        setUnit(d ?? null);
        setEditText(d?.target ?? "");
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

  const decide = useCallback(
    async (item: ReviewItem, decision: ReviewDecision, note?: string) => {
      setDeciding(true);
      try {
        if (onDecide) {
          await onDecide(item, decision, note);
        } else if (decision === "approved") {
          await api.approveReviewItem(tabID, item.locale, item.file, item.key);
        } else if (decision === "rejected") {
          await api.rejectReviewItem(tabID, item.locale, item.file, item.key, note ?? "");
        } else {
          await api.signOffReviewItem(tabID, item.locale, item.file, item.key);
        }
        // The decided unit leaves the queue; the selection effect advances.
        setQueue((q) => (q ?? []).filter((it) => itemId(it) !== itemId(item)));
      } catch (err) {
        showError("Failed to record the review decision", err);
      } finally {
        setDeciding(false);
      }
    },
    [tabID, onDecide, showError],
  );

  const approve = useCallback(() => {
    if (selected && !deciding) void decide(selected, "approved");
  }, [selected, deciding, decide]);

  const signOff = useCallback(() => {
    if (selected && !deciding) void decide(selected, "signed-off");
  }, [selected, deciding, decide]);

  const reject = useCallback(() => {
    if (!selected || deciding) return;
    setAskValue("");
    setAskText({
      kind: "reject",
      title: t("Send back to draft"),
      label: t("Why? The note travels with the unit."),
      placeholder: t("e.g. the tone is too formal for this surface"),
      confirm: t("Send back"),
      allowEmpty: true,
    });
  }, [selected, deciding]);

  const saveTarget = useCallback(async () => {
    if (!selected || !unit) return;
    setSaving(true);
    try {
      if (onSaveTarget) {
        await onSaveTarget(selected, editText);
      } else {
        await api.updateReviewTarget(tabID, selected.locale, selected.file, selected.key, editText);
      }
      // Re-load the unit: findings re-run against the edited text, and the
      // next approval binds to the new content hash.
      const load =
        loadUnit ?? ((it: ReviewItem) => api.getReviewUnit(tabID, it.locale, it.file, it.key));
      const d = await load(selected);
      setUnit(d ?? null);
      setEditText(d?.target ?? editText);
      if (d) {
        const has = d.findings.length > 0;
        setQueue((q) =>
          (q ?? []).map((it) =>
            itemId(it) === itemId(selected) ? { ...it, target: d.target, hasFindings: has } : it,
          ),
        );
      }
    } catch (err) {
      showError("Failed to save the translation", err);
    } finally {
      setSaving(false);
    }
  }, [selected, unit, editText, tabID, onSaveTarget, loadUnit, showError]);

  // Per-unit AI actions — the only review paths that call a provider, and only
  // on explicit click. Fix/retranslate yield a PROPOSAL (diff + Accept/Discard);
  // explain yields text. Nothing is written until Accept, which routes through
  // the same save-target path as a manual edit.
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
          setAIProposal(res.proposed_target);
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

  // Accept the AI proposal: write it through the same path a manual edit takes
  // (UpdateReviewTarget), then re-load the unit so checks re-run and the next
  // approval binds to the new text.
  const acceptProposal = useCallback(async () => {
    if (!selected || !unit || aiProposal === null) return;
    setSaving(true);
    try {
      if (onSaveTarget) {
        await onSaveTarget(selected, aiProposal);
      } else {
        await api.updateReviewTarget(
          tabID,
          selected.locale,
          selected.file,
          selected.key,
          aiProposal,
        );
      }
      setAIProposal(null);
      const load =
        loadUnit ?? ((it: ReviewItem) => api.getReviewUnit(tabID, it.locale, it.file, it.key));
      const d = await load(selected);
      setUnit(d ?? null);
      setEditText(d?.target ?? aiProposal);
      if (d) {
        const has = d.findings.length > 0;
        setQueue((q) =>
          (q ?? []).map((it) =>
            itemId(it) === itemId(selected) ? { ...it, target: d.target, hasFindings: has } : it,
          ),
        );
      }
    } catch (err) {
      showError("Failed to apply the AI proposal", err);
    } finally {
      setSaving(false);
    }
  }, [selected, unit, aiProposal, tabID, onSaveTarget, loadUnit, showError]);

  // AI pre-review modal: load the reviewer model for display when it opens.
  useEffect(() => {
    if (!preReviewOpen) return;
    void api.getDefaultModel().then((info) => {
      if (info) setReviewerModel(info.model || info.provider || "");
    });
  }, [preReviewOpen]);

  const preReviewPending = useMemo(() => {
    let items = queue ?? [];
    if (localeFilter) items = items.filter((it) => it.locale === localeFilter);
    if (collectionFilter) items = items.filter((it) => (it.collection ?? "") === collectionFilter);
    return items;
  }, [queue, localeFilter, collectionFilter]);

  const runPreReview = useCallback(async () => {
    setPreReviewRunning(true);
    setPreReviewResult(null);
    try {
      const run =
        onPreReview ??
        ((locale: string, sc: PreReviewScope, policy: PreReviewPolicy) =>
          api.runAIPreReview(tabID, locale, sc, policy));
      const res = await run(
        localeFilter,
        { collection: collectionFilter || undefined },
        { autoApprove: preReviewAuto, minScore: preReviewMinScore },
      );
      setPreReviewResult(res ?? null);
      await refreshQueue();
    } catch (err) {
      showError("AI pre-review failed", err);
    } finally {
      setPreReviewRunning(false);
    }
  }, [
    tabID,
    onPreReview,
    localeFilter,
    collectionFilter,
    preReviewAuto,
    preReviewMinScore,
    refreshQueue,
    showError,
  ]);

  // Keyboard-first: j/k navigate, a approve, r reject, s sign off, space skip,
  // e focuses the target editor. Typing in the editor is left alone (Escape
  // returns to the queue).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName ?? "";
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT") {
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
        case "s":
          e.preventDefault();
          signOff();
          break;
        case " ":
          if (tag === "BUTTON") return; // native activation
          e.preventDefault();
          move(1);
          break;
        case "e":
          e.preventDefault();
          editRef.current?.focus();
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [move, approve, reject, signOff, documentOpen]);

  // Phase 2 batch: approve every clean unit in the current filter.
  const cleanVisible = useMemo(() => visible.filter((it) => it.hasFindings === false), [visible]);
  const approveClean = useCallback(async () => {
    const targets = cleanVisible;
    if (targets.length === 0) return;
    setBatch({ done: 0, total: targets.length });
    try {
      for (let i = 0; i < targets.length; i++) {
        const item = targets[i];
        if (onDecide) {
          await onDecide(item, "approved");
        } else {
          await api.approveReviewItem(tabID, item.locale, item.file, item.key);
        }
        setQueue((q) => (q ?? []).filter((it) => itemId(it) !== itemId(item)));
        setBatch({ done: i + 1, total: targets.length });
      }
    } catch (err) {
      showError("Batch approval stopped", err);
    } finally {
      setBatch(null);
    }
  }, [cleanVisible, tabID, onDecide, showError]);

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
      parts.push(activeFilter.languages.map(localeLabel).join(", "));
    }
    if (activeFilter.collections?.length) {
      parts.push(activeFilter.collections.join(", "));
    }
    if (activeFilter.glob?.trim()) parts.push(activeFilter.glob.trim());
    if (parts.length === 0) return "";
    return `${activeFilter.name || t("the active filter")} — ${parts.join(" · ")}`;
  }, [activeFilter]);

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
          <div className="flex items-center gap-1" data-slot="review-lane-toggle">
            {(
              [
                { id: "target", label: t("Target") },
                {
                  id: "source",
                  label: sourceQueue?.length
                    ? t("Source ({count})", { count: sourceQueue.length })
                    : t("Source"),
                },
              ] as Array<{ id: Lane; label: string }>
            ).map((l) => (
              <Button
                key={l.id}
                variant={lane === l.id ? "default" : "outline"}
                size="xs"
                onClick={() => setLane(l.id)}
                aria-pressed={lane === l.id}
              >
                {l.label}
              </Button>
            ))}
          </div>
          <div
            className="flex items-center gap-1"
            data-slot="review-chips"
            hidden={lane !== "target"}
          >
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
          {lane === "target" && locales.length > 1 && (
            <LocaleSelect
              value={localeFilter}
              onChange={setLocaleFilter}
              locales={localeOptions}
              clearLabel={t("All languages")}
              compact
              className="h-7"
              aria-label={t("Filter by language")}
              data-slot="review-locale-filter"
            />
          )}
          {lane === "target" && collections.length > 1 && (
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
          {lane === "target" && (
            <Button
              variant="outline"
              size="xs"
              onClick={() => {
                setPreReviewResult(null);
                setPreReviewOpen(true);
              }}
              disabled={loadingQueue || (queue ?? []).length === 0}
              data-slot="review-prereview-open"
            >
              <Sparkles size={12} />
              {t("AI pre-review…")}
            </Button>
          )}
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
                  {localeFilter ? localeLabel(localeFilter) : t("all languages")}
                  {collectionFilter ? ` · ${collectionFilter}` : ""}
                </span>{" "}
                — {t("{count} pending units", { count: preReviewPending.length })}
              </div>
            </div>
            <div className="space-y-1.5 text-xs" role="radiogroup" aria-label={t("Policy")}>
              <label className="flex items-start gap-2">
                <input
                  type="radio"
                  name="prereview-policy"
                  checked={!preReviewAuto}
                  onChange={() => setPreReviewAuto(false)}
                  data-slot="review-prereview-annotate"
                />
                <span>
                  <span className="font-medium">{t("Annotate only")}</span>{" "}
                  <span className="text-muted-foreground">
                    {"· "}
                    {t("Store score and findings; every decision stays yours.")}
                  </span>
                </span>
              </label>
              <label className="flex items-start gap-2">
                <input
                  type="radio"
                  name="prereview-policy"
                  checked={preReviewAuto}
                  onChange={() => setPreReviewAuto(true)}
                  data-slot="review-prereview-auto"
                />
                <span>
                  <span className="font-medium">
                    {t("Auto-approve clean units scoring at least")}
                  </span>{" "}
                  <input
                    type="number"
                    min={0}
                    max={100}
                    value={preReviewMinScore}
                    onChange={(e) => setPreReviewMinScore(Number(e.target.value))}
                    className="w-14 rounded border border-input bg-transparent px-1 text-xs"
                    aria-label={t("Minimum score")}
                  />{" "}
                  <span className="text-muted-foreground">
                    {"· "}
                    {t("Approvals are recorded as ai/<model>; human-required gates ignore them.")}
                  </span>
                </span>
              </label>
            </div>
            {preReviewResult && (
              <div
                className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs"
                data-slot="review-prereview-result"
              >
                {t("{approved} auto-approved · {left} left for you", {
                  approved: preReviewResult.auto_approved,
                  left: preReviewResult.remaining,
                })}
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

      {lane === "source" && (
        <SourceLane
          tabID={tabID}
          filter={activeFilter ?? null}
          items={sourceQueue ?? undefined}
          onChanged={refreshSourceQueue}
        />
      )}

      {/* Batch bar (Phase 2): approve all clean units in the current filter. */}
      {lane === "target" && cleanVisible.length > 0 && (
        <div
          className="mb-3 flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-1.5 text-xs"
          data-slot="review-batch"
        >
          <CheckCheck size={13} className="shrink-0 text-muted-foreground" />
          <span className="text-muted-foreground">
            {t("{count} clean units (no findings) in this view", { count: cleanVisible.length })}
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
                {t("Approve {count} clean units", { count: cleanVisible.length })}
              </>
            )}
          </Button>
        </div>
      )}

      {lane !== "target" ? null : loadingQueue && !queue ? (
        <div className="p-4 text-sm text-muted-foreground">{t("Loading review queue…")}</div>
      ) : visible.length === 0 ? (
        <Card className="border-dashed" data-slot="review-empty">
          <CardContent className="p-10 text-center">
            <CheckCircle2 size={24} className="mx-auto mb-2 text-primary" />
            <p className="text-sm text-muted-foreground">
              {(queue ?? []).length === 0
                ? t("Review queue empty. Every translated unit is reviewed.")
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
                      >
                        {it.hasFindings ? (
                          <Circle
                            size={8}
                            className="mt-1 shrink-0 fill-amber-500 text-amber-500"
                            aria-label={t("Has findings")}
                          />
                        ) : (
                          <Circle
                            size={8}
                            className={`mt-1 shrink-0 ${it.hasFindings === false ? "text-primary" : "text-muted-foreground/40"}`}
                            aria-label={it.hasFindings === false ? t("Clean") : t("Not checked")}
                          />
                        )}
                        <span className="min-w-0 flex-1">
                          <span className="flex items-center gap-1.5">
                            <span className="truncate font-medium" translate="no">
                              {it.key}
                            </span>
                            <LocalePill locale={it.locale} />
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

          {/* Center: the unit — the point it sits at, SOURCE / TARGET, the
              document around it, what was approved before, the checks and the
              provenance. */}
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
                    <LocalePill locale={selected.locale} />
                    {unit?.status && (
                      <Badge
                        variant="outline"
                        className="text-muted-foreground"
                        data-slot="review-status"
                      >
                        {unit.status}
                      </Badge>
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

                <PointRail point={model?.point} loading={unitLoading} />

                <Card>
                  <CardContent className="p-3">
                    <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("Source")}
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
                    <div className="mb-1 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("Target")}
                      {unit && !unit.editable && (
                        <span className="normal-case font-normal">
                          {t("(formatted content, read-only)")}
                        </span>
                      )}
                    </div>
                    <textarea
                      ref={editRef}
                      className="min-h-20 w-full resize-y rounded-md border border-input bg-transparent p-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-60"
                      value={editText}
                      onChange={(e) => setEditText(e.target.value)}
                      disabled={!unit || !unit.editable || saving}
                      aria-label={t("Edit the translation")}
                      data-slot="review-target"
                      translate="no"
                      {...directionAttrs(unit?.locale ?? selected?.locale)}
                    />
                    {unit && unit.editable && editText !== unit.target && (
                      <div className="mt-2 flex items-center gap-2">
                        <Button
                          size="xs"
                          onClick={() => void saveTarget()}
                          disabled={saving}
                          data-slot="review-save"
                        >
                          {saving ? (
                            <Loader2 size={12} className="animate-spin" />
                          ) : (
                            <Check size={12} />
                          )}
                          {t("Save & re-check")}
                        </Button>
                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => setEditText(unit.target)}
                          disabled={saving}
                        >
                          <Undo2 size={12} />
                          {t("Revert")}
                        </Button>
                      </div>
                    )}
                    {/* Per-unit AI actions (phase 3): explicit clicks only. */}
                    <div
                      className="mt-2 flex flex-wrap items-center gap-2"
                      data-slot="review-ai-actions"
                    >
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() => void runAIAction("fix-findings")}
                        disabled={!unit || !unit.editable || aiBusy !== null || saving}
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
                        disabled={!unit || !unit.editable || aiBusy !== null || saving}
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
                        <div className="rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1">
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
                          <span
                            className="whitespace-pre-wrap"
                            translate="no"
                            {...directionAttrs(unit.locale)}
                          >
                            {aiProposal}
                          </span>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          size="xs"
                          onClick={() => void acceptProposal()}
                          disabled={saving}
                          data-slot="review-ai-accept"
                        >
                          {saving ? (
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
                          disabled={saving}
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
                        <Button variant="ghost" size="xs" onClick={() => setAIExplanation(null)}>
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
                  sourceLocale={unit?.source_locale ?? selected?.sourceLocale}
                  locale={unit?.locale ?? selected?.locale}
                  loading={unitLoading}
                  fallbackMemoryScore={unit?.tm_score}
                />

                {/* What has already been said about this translation: the
                    checks, and the AI pre-review that scored it. */}
                <Card data-slot="review-findings">
                  <CardContent className="p-3">
                    <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("Checks")}
                    </div>
                    {!unit || unit.findings.length === 0 ? (
                      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <CheckCircle2 size={12} className="text-primary" />
                        {t("No findings for this unit.")}
                      </div>
                    ) : (
                      <ul className="space-y-1.5">
                        {unit.findings.map((f: DesktopFinding, i: number) => (
                          <li
                            key={i}
                            className="flex items-start gap-2 text-xs"
                            data-slot="review-finding"
                          >
                            <Badge variant="outline" className={severityBadgeClass(f.severity)}>
                              {f.severity}
                            </Badge>
                            {/* Which side of the unit the finding is about. A
                                voice or terminology finding is judged on the
                                SOURCE, and unlabelled it reads as a defect in
                                the translation: a reviewer saw `Forbidden term
                                "cart"` on a Norwegian target containing no such
                                word, with nothing to act on and no way to tell
                                why. */}
                            {f.field === "source" && (
                              <Badge
                                variant="outline"
                                className="shrink-0 text-[10px] text-muted-foreground"
                                title={t(
                                  "This finding is about the source text, not the translation.",
                                )}
                              >
                                {t("source")}
                              </Badge>
                            )}
                            <span className="min-w-0">
                              <span>{f.message}</span>
                              {f.suggestion && (
                                <span className="block text-muted-foreground">
                                  ↳ {f.suggestion}
                                </span>
                              )}
                            </span>
                          </li>
                        ))}
                      </ul>
                    )}
                    <AIPreReview
                      score={model?.judgement.ai_score ?? unit?.ai_review_score}
                      model={model?.judgement.ai_model ?? unit?.ai_review_model}
                      findings={model?.judgement.ai_findings}
                    />
                  </CardContent>
                </Card>

                {/* Where this translation came from, and the decision in force. */}
                <ProvenanceCard
                  provenance={model?.provenance}
                  origin={unit?.origin}
                  reviewState={unit?.review_state}
                  note={unit?.note}
                />
              </div>
            </ScrollArea>

            {/* Action bar — the keyboard verbs, spelled out. */}
            <div
              className="mt-3 flex flex-wrap items-center gap-2 rounded-md border bg-muted/30 px-3 py-2"
              data-slot="review-actions"
            >
              <Button
                size="sm"
                onClick={approve}
                disabled={!selected || deciding}
                data-slot="review-approve"
              >
                <Check size={13} />
                {t("Approve")}
                <kbd className="ml-1 rounded bg-muted px-1 text-[10px] text-muted-foreground">
                  a
                </kbd>
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={reject}
                disabled={!selected || deciding}
                data-slot="review-reject"
              >
                <X size={13} />
                {t("Reject")}
                <kbd className="ml-1 rounded bg-muted px-1 text-[10px] text-muted-foreground">
                  r
                </kbd>
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={signOff}
                disabled={!selected || deciding}
                data-slot="review-signoff"
              >
                <CheckCheck size={13} />
                {t("Sign off")}
                <kbd className="ml-1 rounded bg-muted px-1 text-[10px] text-muted-foreground">
                  s
                </kbd>
              </Button>
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
        side={selected?.locale}
        backLabel={t("Back to the queue")}
        onClose={() => setDocumentOpen(false)}
      />
    </div>
  );
}
