// The Reader: one content file as a person reads it (page-patterns "Reader").
// The file's blocks run in file order at reading width, each finding drawn on
// the words it is about with its row under the block, and the rules that hold
// at the file run down the margin in the order `kapi context <path>` prints
// them. On a narrow window the margin moves below the blocks.
//
// The page renders from one read, ReaderFile: the blocks and findings of a
// checks run over this file alone, and the context answer for it. A fix is
// the finding's own change operation, which the backend applies through the
// change service as the person at the keyboard; the page then draws the file
// as the service's result left it. Nothing on the page edits a block in place.

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, AudioLines, FileText, ListChecks, Lock, Wand2 } from "lucide-react";
import {
  ActorMark,
  Button,
  EmptyState,
  ErrorNotice,
  FindingSnippet,
  KindIcon,
  LoadingSpinner,
  PageHeader,
  ScrollArea,
  SectionHeading,
  SimpleTooltip,
  When,
  findingOutcomeTone,
  findingToneTextClass,
  type DigestKind,
  type FindingMark,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import type {
  ReaderBlock,
  ReaderFile,
  ReaderFinding,
  ReaderRefusal,
  ReaderRule,
} from "../types/api";

/**
 * The view that opens the Reader on a file, at one block when one is named:
 * "reader:<path>" or "reader:<path>:<block>", each part URI-encoded so a path
 * holding a colon stays one part.
 */
export function readerView(path: string, blockID?: string): string {
  const view = `reader:${encodeURIComponent(path)}`;
  return blockID ? `${view}:${encodeURIComponent(blockID)}` : view;
}

/** The file and block a "reader:" view names, or null for any other view. */
export function readerTargetOf(view: string): { path: string; blockID?: string } | null {
  const [base, path, block] = view.split(":");
  if (base !== "reader" || !path) return null;
  try {
    return {
      path: decodeURIComponent(path),
      ...(block ? { blockID: decodeURIComponent(block) } : {}),
    };
  } catch {
    return null;
  }
}

export interface ReaderPageProps {
  tabID: string;
  /** The file, project-relative (or absolute inside the project). */
  path: string;
  /** Scroll to this block once the file is drawn, and mark it. */
  focusBlockID?: string;
  /** Pre-loaded file for Storybook and tests, which reach no backend. */
  reader?: ReaderFile;
  /** Return to the page the Reader was opened from. */
  onBack: () => void;
  /** The back button's label, naming that page. */
  backLabel?: string;
  /** Go to another view of the project: "context", … */
  onNavigate?: (view: string) => void;
}

export function ReaderPage({
  tabID,
  path,
  focusBlockID,
  reader: given,
  onBack,
  backLabel,
  onNavigate,
}: ReaderPageProps) {
  const qc = useQueryClient();
  const key = qk.readerFile(tabID, path);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.readerFile(tabID, path),
    enabled: !given,
    placeholderData: (previous) => previous,
  });
  // A rule kept anywhere, or the file changing on disk, changes the answer.
  useInvalidateOnEvent(["project-files-changed", "workspace:changed"], [key]);

  // A fix answers with the file read again, which replaces the page's read.
  // Its refusals are kept apart from the read: the file changing on disk
  // reads it again, and that read carries no refusal of its own.
  const [refused, setRefused] = useState<ReaderRefusal[] | null>(null);
  const settle = (next: ReaderFile | null) => {
    if (!next) return;
    qc.setQueryData(key, next);
    setRefused(next.refused);
  };
  const fixOne = useMutation({
    mutationFn: (findingID: string) => api.fixFinding(tabID, path, findingID),
    onSuccess: settle,
  });
  const fixAll = useMutation({
    mutationFn: () => api.fixFile(tabID, path),
    onSuccess: settle,
  });

  const reader = given ?? query.data ?? null;
  const busy = fixOne.isPending || fixAll.isPending;

  if (!reader) {
    return (
      <div className="p-6">
        {query.error ? (
          <div className="space-y-3">
            <ErrorNotice error={query.error} title={t("The file could not be read")} />
            <Button size="sm" variant="ghost" onClick={onBack}>
              <ArrowLeft size={14} />
              {backLabel ?? t("Back")}
            </Button>
          </div>
        ) : (
          <LoadingSpinner />
        )}
      </div>
    );
  }

  return (
    <ScrollArea className="h-full">
      <ReaderView
        reader={reader}
        refused={refused ?? reader.refused}
        focusBlockID={focusBlockID}
        busy={busy}
        fixingID={fixOne.isPending ? fixOne.variables : undefined}
        fixingAll={fixAll.isPending}
        fixError={fixOne.error ?? fixAll.error}
        onFix={(id) => fixOne.mutate(id)}
        onFixAll={() => fixAll.mutate()}
        onBack={onBack}
        backLabel={backLabel}
        onNavigate={onNavigate}
      />
    </ScrollArea>
  );
}

export interface ReaderViewProps {
  reader: ReaderFile;
  /** The last fix's refusals; the read's own when absent. */
  refused?: ReaderRefusal[];
  focusBlockID?: string;
  /** A fix is being applied. */
  busy?: boolean;
  /** The finding whose fix is being applied. */
  fixingID?: string;
  fixingAll?: boolean;
  fixError?: unknown;
  onFix: (findingID: string) => void;
  onFixAll: () => void;
  onBack: () => void;
  backLabel?: string;
  onNavigate?: (view: string) => void;
}

/** The Reader as it is drawn, from a file already read. */
export function ReaderView({
  reader,
  refused: given,
  focusBlockID,
  busy,
  fixingID,
  fixingAll,
  fixError,
  onFix,
  onFixAll,
  onBack,
  backLabel,
  onNavigate,
}: ReaderViewProps) {
  const refused = given ?? reader.refused;
  const refusals = useMemo(() => {
    const out = new Map<string, string>();
    for (const r of refused) out.set(r.finding_id, r.reason);
    return out;
  }, [refused]);
  // A refusal whose finding is gone from the file read again is drawn at the
  // top, since there is no row left to draw it under.
  const placed = useMemo(() => {
    const ids = new Set<string>();
    for (const b of reader.blocks) for (const f of b.findings) ids.add(f.id);
    for (const f of reader.document) ids.add(f.id);
    return ids;
  }, [reader.blocks, reader.document]);
  const unplaced = refused.filter((r) => !placed.has(r.finding_id));

  // The page scrolls to the block it opened at once, when that block is first
  // drawn. A fix reads the file again, and the reader stays where they are.
  const blocksRef = useRef<HTMLDivElement>(null);
  const scrolledTo = useRef<string | null>(null);
  useEffect(() => {
    if (!focusBlockID || scrolledTo.current === focusBlockID) return;
    const el = blocksRef.current?.querySelector<HTMLElement>(
      `[data-block-id="${CSS.escape(focusBlockID)}"]`,
    );
    if (!el) return;
    scrolledTo.current = focusBlockID;
    el.scrollIntoView?.({ block: "center" });
  }, [focusBlockID, reader]);

  return (
    <div data-slot="reader" className="mx-auto max-w-6xl space-y-6 p-6">
      <PageHeader
        className="mb-0"
        title={reader.path}
        subtitle={<ReaderCounts reader={reader} />}
        backButton={
          <Button size="sm" variant="ghost" data-slot="reader-back" onClick={onBack}>
            <ArrowLeft size={14} />
            {backLabel ?? t("Back")}
          </Button>
        }
        actions={
          reader.fixable > 0 && (
            <Button size="sm" data-slot="reader-fix-file" disabled={busy} onClick={onFixAll}>
              {fixingAll ? <LoadingSpinner size="sm" /> : <Wand2 size={14} />}
              {t("Fix {count} in this file", "a button: apply every fix in the file", {
                count: reader.fixable,
              })}
            </Button>
          )
        }
      />
      {!!fixError && <ErrorNotice error={fixError} title={t("The fix did not apply")} />}
      {unplaced.length > 0 && (
        <ul data-slot="reader-refused" className="space-y-1">
          {unplaced.map((r) => (
            <li key={r.finding_id} className={`text-xs ${findingToneTextClass("warning")}`}>
              <NotFixed reason={r.reason} />
            </li>
          ))}
        </ul>
      )}

      <div className="flex flex-col gap-8 lg:grid lg:grid-cols-[minmax(0,1fr)_18rem] lg:items-start">
        <div ref={blocksRef} data-slot="reader-blocks" className="min-w-0 max-w-[70ch] space-y-5">
          {reader.document.length > 0 && (
            <FindingRows
              findings={reader.document}
              refusals={refusals}
              busy={busy}
              fixingID={fixingID}
              onFix={onFix}
            />
          )}
          {reader.blocks.length === 0 ? (
            <div data-slot="reader-no-blocks">
              <EmptyState
                icon={<FileText size={20} className="mx-auto" />}
                title={t("No blocks in this file")}
              />
            </div>
          ) : (
            reader.blocks.map((block) => (
              <BlockView
                key={block.id}
                block={block}
                focused={block.id === focusBlockID}
                refusals={refusals}
                busy={busy}
                fixingID={fixingID}
                onFix={onFix}
              />
            ))
          )}
        </div>
        <RulesMargin reader={reader} onNavigate={onNavigate} />
      </div>
    </div>
  );
}

/** The counts under the path: blocks, and findings or that none were checked. */
function ReaderCounts({ reader }: { reader: ReaderFile }) {
  const blocks = t("{count, plural, one {# block} other {# blocks}}", "a count in the Reader", {
    count: reader.blocks.length,
  });
  const findings =
    reader.verdict === "did_not_run" && reader.findings === 0
      ? t("Not checked")
      : t("{count, plural, one {# finding} other {# findings}}", "a count in the Reader", {
          count: reader.findings,
        });
  return (
    <span data-slot="reader-counts" className="tabular-nums">
      {blocks}
      {" · "}
      {findings}
    </span>
  );
}

/** One block: its id as the check names it, its text with the findings marked, and their rows. */
function BlockView({
  block,
  focused,
  refusals,
  busy,
  fixingID,
  onFix,
}: {
  block: ReaderBlock;
  focused: boolean;
  refusals: Map<string, string>;
  busy?: boolean;
  fixingID?: string;
  onFix: (findingID: string) => void;
}) {
  const marks = useMemo<FindingMark[]>(
    () =>
      block.findings
        .filter((f) => !!f.position)
        .map((f) => ({ anchor: f.position!, tone: findingOutcomeTone(f), label: f.label })),
    [block.findings],
  );
  return (
    <section
      data-slot="reader-block"
      data-block-id={block.id}
      data-focused={focused || undefined}
      className={`space-y-1.5 rounded-md ${focused ? "bg-accent/40 -mx-2 px-2 py-1.5" : ""}`}
    >
      <div translate="no" className="font-mono text-[11px] text-muted-foreground">
        {block.id}
      </div>
      <div className="text-sm leading-relaxed">
        <FindingSnippet runs={block.runs} marks={marks} tone="muted" data-testid="reader-text" />
      </div>
      {block.findings.length > 0 && (
        <FindingRows
          findings={block.findings}
          refusals={refusals}
          busy={busy}
          fixingID={fixingID}
          onFix={onFix}
        />
      )}
    </section>
  );
}

/** The rows under a block, one per finding. */
function FindingRows({
  findings,
  refusals,
  busy,
  fixingID,
  onFix,
}: {
  findings: ReaderFinding[];
  refusals: Map<string, string>;
  busy?: boolean;
  fixingID?: string;
  onFix: (findingID: string) => void;
}) {
  return (
    <ul role="list" className="space-y-1">
      {findings.map((f) => (
        <FindingRow
          key={f.id}
          finding={f}
          refused={refusals.get(f.id)}
          busy={busy}
          fixing={fixingID === f.id}
          onFix={() => onFix(f.id)}
        />
      ))}
    </ul>
  );
}

/** One finding: its kind, its label, and Fix when it has one. */
function FindingRow({
  finding,
  refused,
  busy,
  fixing,
  onFix,
}: {
  finding: ReaderFinding;
  refused?: string;
  busy?: boolean;
  fixing: boolean;
  onFix: () => void;
}) {
  const tone = findingOutcomeTone(finding);
  return (
    <li
      data-slot="reader-finding"
      data-id={finding.id}
      data-fails={finding.fails || undefined}
      className="flex min-h-8 flex-wrap items-center gap-x-3 gap-y-1"
    >
      <KindMark kind={finding.kind} />
      <span className={`min-w-0 flex-1 text-sm ${findingToneTextClass(tone)}`}>
        {/* The label is the check's own wording, read from the backend: the
            rule's terms or the check's message, never a string to translate. */}
        <span translate="no" data-slot="reader-finding-label">
          {finding.label}
        </span>
        {finding.locale && (
          <span translate="no" className="ml-1.5 font-mono text-[11px] text-muted-foreground">
            {finding.locale}
          </span>
        )}
      </span>
      {finding.fixable ? (
        <Button size="xs" variant="outline" data-slot="reader-fix" disabled={busy} onClick={onFix}>
          {fixing ? <LoadingSpinner size="sm" /> : <Wand2 size={12} />}
          Fix
        </Button>
      ) : (
        <span data-slot="reader-by-hand" className="text-[11px] text-muted-foreground">
          {t("Edit by hand")}
        </span>
      )}
      {refused && (
        <div
          data-slot="reader-finding-refused"
          role="alert"
          className={`basis-full pl-24 text-xs ${findingToneTextClass("warning")}`}
        >
          <NotFixed reason={refused} />
        </div>
      )}
    </li>
  );
}

/** A fix the change service refused, with its reason. */
function NotFixed({ reason }: { reason: string }) {
  return <span>{t("Not fixed: {reason}", "a fix the change service refused", { reason })}</span>;
}

/** The digest kind a Reader kind is drawn with, where one draws it. */
const DIGEST_KIND: Partial<Record<string, DigestKind>> = {
  name: "name",
  word: "word",
  note: "note",
};

/** The word a Reader kind is named by. */
function kindWord(kind: string): string {
  switch (kind) {
    case "name":
      return t("Name", "the kind of a finding or a rule");
    case "word":
      return t("Word", "the kind of a finding or a rule");
    case "tone":
      return t("Tone", "the kind of a finding or a rule");
    case "note":
      return t("Note", "the kind of a finding or a rule");
    case "keep":
      return t("Keep", "the kind of a rule: a wording that stays as it is here");
    case "placeholder":
      return t("Placeholder", "the kind of a finding");
    case "dnt":
      return t("Do not translate", "the kind of a finding");
    default:
      return kind;
  }
}

/** The icon a Reader kind is drawn with. */
function KindGlyph({ kind }: { kind: string }) {
  const digest = DIGEST_KIND[kind];
  if (digest) return <KindIcon kind={digest} decorative />;
  if (kind === "tone") return <AudioLines aria-hidden className="size-3.5 shrink-0" />;
  if (kind === "keep") return <Lock aria-hidden className="size-3.5 shrink-0" />;
  return <ListChecks aria-hidden className="size-3.5 shrink-0" />;
}

/** A kind as a small mark and its word, in a column of its own. */
function KindMark({ kind }: { kind: string }) {
  return (
    <span
      data-slot="reader-kind"
      data-kind={kind}
      className="inline-flex w-24 shrink-0 items-center gap-1.5 text-[11px] text-muted-foreground"
    >
      <KindGlyph kind={kind} />
      <span className="truncate">{kindWord(kind)}</span>
    </span>
  );
}

/** The margin: the rules the context answer for the file holds, in its order. */
function RulesMargin({
  reader,
  onNavigate,
}: {
  reader: ReaderFile;
  onNavigate?: (view: string) => void;
}) {
  const more = reader.rules_total - reader.rules.length;
  return (
    <aside data-slot="reader-rules" className="min-w-0 space-y-2 lg:sticky lg:top-6">
      <SectionHeading>Rules here</SectionHeading>
      {reader.attention.length > 0 && (
        <ul data-slot="reader-attention" className="space-y-1">
          {reader.attention.map((a) => (
            <li key={a} className={`text-xs ${findingToneTextClass("warning")}`}>
              {a}
            </li>
          ))}
        </ul>
      )}
      {reader.rules.length === 0 ? (
        <div data-slot="reader-rules-empty">
          <EmptyState
            title={t("No rules hold here yet.")}
            action={
              onNavigate && (
                <Button size="sm" variant="outline" onClick={() => onNavigate("context")}>
                  Open Context
                </Button>
              )
            }
          />
        </div>
      ) : (
        <ul role="list" className="divide-y divide-border/60">
          {reader.rules.map((rule, i) => (
            <RuleRow key={`${rule.kind}:${rule.rule}:${i}`} rule={rule} />
          ))}
        </ul>
      )}
      {more > 0 && (
        <div data-slot="reader-rules-more" className="text-[11px] text-muted-foreground">
          {t("{count} more", "rules the margin does not list", { count: more })}
        </div>
      )}
    </aside>
  );
}

/** One rule: its kind, the rule, and where it comes from or who recorded it. */
function RuleRow({ rule }: { rule: ReaderRule }) {
  const status =
    rule.status === "suggested"
      ? t("suggested", "a rule nobody has established")
      : rule.status === "contested"
        ? t("contested", "a rule another rule disagrees with")
        : "";
  return (
    <li
      data-slot="reader-rule"
      data-kind={rule.kind}
      data-status={rule.status}
      className="flex items-start gap-2 py-2"
    >
      <SimpleTooltip content={kindWord(rule.kind)}>
        <span
          role="img"
          aria-label={kindWord(rule.kind)}
          className="mt-0.5 inline-flex text-muted-foreground"
        >
          <KindGlyph kind={rule.kind} />
        </span>
      </SimpleTooltip>
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="text-sm">{rule.rule}</div>
        {(status || rule.from || rule.by || rule.at) && <RuleMeta rule={rule} status={status} />}
      </div>
    </li>
  );
}

/** Where a rule comes from, or who recorded it and when, on one meta line. */
function RuleMeta({ rule, status }: { rule: ReaderRule; status: string }) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-1 text-[11px] text-muted-foreground">
      {status && <span>{status}</span>}
      {rule.from && (
        <span translate="no" className="truncate">
          {status ? " · " : ""}
          {rule.from}
        </span>
      )}
      {rule.by && (
        <span className="inline-flex items-center gap-1">
          {status || rule.from ? " · " : ""}
          <ActorMark actor={rule.by} name={rule.by} size="sm" decorative />
          <span translate="no">{rule.by}</span>
        </span>
      )}
      {rule.at && (
        <span>
          {" · "}
          <When iso={rule.at} relative />
        </span>
      )}
    </div>
  );
}
