// The detail pane of What's new: the selected row's rule, its evidence, who
// noticed it and when, and the decisions open on it in the toolbar.
//
// A suggestion offers Keep, Change and Drop, and Apply more widely once it is
// a rule in force. A disagreement lays its sides beside each other, each with
// its evidence and its own Choose this side; the one sentence that explains
// the disagreement is the toolbar's caption, shown here and nowhere else. A
// rule in force offers Drop and Apply more widely. Change puts a field in the
// toolbar's place, or in the side's, holding the form the rule asks for.

import { useEffect, useRef } from "react";
import { FileText } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ActorMark,
  Button,
  Input,
  ListDetailEmpty,
  ListDetailPane,
  ListDetailToolbar,
  When,
  cn,
} from "@neokapi/ui-primitives";
import type { DigestItem, DigestTermRule } from "../types/api";
import type { DigestRow } from "../lib/digestRows";
import { ConflictQuestion, RuleSentence, themeLabel } from "./DigestRule";
import { WidenPicker } from "./ContextFeed";

/** A rule's form being changed before it is kept: the row, the side, and the form typed so far. */
export interface DigestChange {
  key: string;
  /** The item whose form is changed: the suggestion, or one side of a disagreement. */
  itemId: string;
  form: string;
}

export interface DigestDetailProps {
  row: DigestRow;
  /** Decisions can be written: a checkout of the project is on this machine. */
  canDecide: boolean;
  /** The change in progress, when it is on this row. */
  change: DigestChange | null;
  /** Keep a suggestion, in another form when one is given. */
  onKeep: (replacement?: string) => void;
  /** Choose one side of a disagreement, in another form when one is given. */
  onChoose: (side: DigestItem, replacement?: string) => void;
  /** Drop the row's item, or one side of a disagreement the evidence alone contests. */
  onDrop: (item: DigestItem) => void;
  /** Apply a rule in force more widely. Absent hides the picker. */
  onWiden?: (to: string) => void;
  /** Open a file the evidence names. Absent shows the path without a link. */
  onOpenFile?: (path: string) => void;
  onStartChange: (item: DigestItem) => void;
  onChangeForm: (form: string) => void;
  onCancelChange: () => void;
}

/** The detail pane for one row of the list. */
export function DigestDetail(props: DigestDetailProps) {
  switch (props.row.group) {
    case "conflict":
      return <ConflictDetail {...props} />;
    case "suggested":
      return <SuggestionDetail {...props} />;
    default:
      return <RuleDetail {...props} />;
  }
}

/** What the detail pane shows when no row needs a person. */
export function DigestNothingSelected() {
  return (
    <ListDetailEmpty
      title={t("Nothing needs you")}
      description={t(
        "Every suggestion here is decided. The list holds the rules in force: choose one to see where it came from.",
      )}
    />
  );
}

// ── A suggestion ─────────────────────────────────────────────────────────────

function SuggestionDetail({
  row,
  canDecide,
  change,
  onKeep,
  onDrop,
  onOpenFile,
  onStartChange,
  onChangeForm,
  onCancelChange,
}: DigestDetailProps) {
  const { item } = row;
  const changing = change?.itemId === item.id ? change : null;
  const toolbar = changing ? (
    <ChangeForm
      form={changing.form}
      onChange={onChangeForm}
      onKeep={() => onKeep(changing.form)}
      onCancel={onCancelChange}
      className="border-b border-border pb-2.5"
    />
  ) : (
    <ListDetailToolbar
      title={<RuleSentence item={item} />}
      actions={
        canDecide && (
          <>
            {item.keepable && (
              <Button
                size="sm"
                variant="success"
                data-slot="keep-item"
                aria-keyshortcuts="a"
                onClick={() => onKeep()}
              >
                Keep
                <Shortcut k="a" />
              </Button>
            )}
            {item.keepable && item.subject.term && (
              <Button
                size="sm"
                variant="outline"
                data-slot="change-item"
                aria-keyshortcuts="c"
                onClick={() => onStartChange(item)}
              >
                Change
                <Shortcut k="c" />
              </Button>
            )}
            {item.droppable && (
              <Button
                size="sm"
                variant="ghost"
                data-slot="drop-item"
                aria-keyshortcuts="d"
                onClick={() => onDrop(item)}
              >
                Drop
                <Shortcut k="d" />
              </Button>
            )}
          </>
        )
      }
    />
  );
  return (
    <ListDetailPane toolbar={toolbar} className="max-w-3xl">
      <ItemBody item={item} onOpenFile={onOpenFile} />
      <ItemFacts item={item} />
      <GroupShortcut row={row} canDecide={canDecide} />
    </ListDetailPane>
  );
}

// ── A rule in force, or one the content drifts from ──────────────────────────

function RuleDetail({ row, canDecide, onDrop, onWiden, onOpenFile }: DigestDetailProps) {
  const { item, drift } = row;
  const widenTo = item.widenable ? (item.widen_to ?? []) : [];
  return (
    <ListDetailPane
      className="max-w-3xl"
      toolbar={
        <ListDetailToolbar
          title={<RuleSentence item={item} />}
          actions={
            canDecide && (
              <>
                {onWiden && widenTo.length > 0 && (
                  <WidenPicker options={widenTo} onWiden={onWiden} label={t("Apply more widely")} />
                )}
                {item.droppable && (
                  <Button
                    size="sm"
                    variant="ghost"
                    data-slot="drop-item"
                    aria-keyshortcuts="d"
                    onClick={() => onDrop(item)}
                  >
                    Drop
                    <Shortcut k="d" />
                  </Button>
                )}
              </>
            )
          }
        />
      }
    >
      {drift && (
        <p data-slot="digest-drift-line" className="text-sm">
          {drift.line}
        </p>
      )}
      <ItemBody item={item} onOpenFile={onOpenFile} withUsage={item.usage?.line !== drift?.line} />
      <ItemFacts item={item} established />
      {item.how && item.how.length > 0 && (
        <div data-slot="digest-how" className="flex flex-col gap-1">
          <p className="text-[11px] text-muted-foreground">How it was established</p>
          <ul className="flex flex-col gap-0.5 text-sm">
            {item.how.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </div>
      )}
    </ListDetailPane>
  );
}

// ── A disagreement ───────────────────────────────────────────────────────────

function ConflictDetail({
  row,
  canDecide,
  change,
  onChoose,
  onDrop,
  onOpenFile,
  onStartChange,
  onChangeForm,
  onCancelChange,
}: DigestDetailProps) {
  const conflict = row.conflict!;
  const single = conflict.sides.length === 1;
  return (
    <ListDetailPane
      toolbar={
        <ListDetailToolbar
          title={<ConflictQuestion conflict={conflict} />}
          actions={
            <span data-slot="digest-conflict-reason" className="text-[11px] text-muted-foreground">
              {conflict.reason}
            </span>
          }
        />
      }
    >
      <div className="@container">
        <div data-slot="digest-conflict-sides" className="grid gap-3 @xl:grid-cols-2">
          {conflict.sides.map((side, i) => {
            const changing = change?.itemId === side.id ? change : null;
            return (
              <div
                key={side.id}
                data-slot="digest-side"
                data-id={side.id}
                className="flex min-w-0 flex-col gap-3 rounded-lg border border-border p-3"
              >
                <p className="text-sm font-semibold">
                  <RuleSentence item={side} />
                </p>
                <ItemBody item={side} onOpenFile={onOpenFile} withKey={i === 0} />
                <dl className={FACTS_CLASS}>
                  <Fact label={<>Noticed by</>}>
                    <NoticedBy who={side.noticed_by} />
                  </Fact>
                  {side.standing && <Fact label={<>Standing</>}>{side.standing}</Fact>}
                </dl>
                {canDecide && changing && (
                  <ChangeForm
                    form={changing.form}
                    onChange={onChangeForm}
                    onKeep={() => onChoose(side, changing.form)}
                    onCancel={onCancelChange}
                  />
                )}
                {canDecide && !changing && (
                  <div data-slot="digest-actions" className="flex flex-wrap items-center gap-1.5">
                    {side.keepable && (
                      <Button
                        size="sm"
                        variant={i === 0 ? "success" : "outline"}
                        data-slot="keep-item"
                        aria-keyshortcuts={i === 0 ? "a" : undefined}
                        onClick={() => onChoose(side)}
                      >
                        {single ? <>Keep</> : <>Choose this side</>}
                        {i === 0 && <Shortcut k="a" />}
                      </Button>
                    )}
                    {side.keepable && side.subject.term && (
                      <Button
                        size="sm"
                        variant="ghost"
                        data-slot="change-item"
                        aria-keyshortcuts={i === 0 ? "c" : undefined}
                        onClick={() => onStartChange(side)}
                      >
                        Change
                        {i === 0 && <Shortcut k="c" />}
                      </Button>
                    )}
                    {single && side.droppable && (
                      <Button
                        size="sm"
                        variant="ghost"
                        data-slot="drop-item"
                        aria-keyshortcuts="d"
                        onClick={() => onDrop(side)}
                      >
                        Drop
                        <Shortcut k="d" />
                      </Button>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </ListDetailPane>
  );
}

// ── The parts ────────────────────────────────────────────────────────────────

/** A term rule as the host sends it: the digest carries the note the assistant left on it. */
type NotedTermRule = DigestTermRule & { note?: string };

/** What the assistant said about the item, when it said anything beyond the rule. */
function itemNote(item: DigestItem): string | undefined {
  const s = item.subject;
  if (s.kind !== "term") return undefined;
  return s.text || (s.term as NotedTermRule | undefined)?.note || undefined;
}

/**
 * The evidence: the assistant's note, the sentence the item was seen in as a
 * quotation with its file beneath, and the project's own words back to it.
 * `onOpenFile` makes the file a link; `withKey` shows that `o` follows it too,
 * which holds for every item but a disagreement's second side. `withUsage`
 * leaves the usage line out where the pane already says it, as Drift does.
 */
function ItemBody({
  item,
  onOpenFile: open,
  withKey = true,
  withUsage = true,
}: {
  item: DigestItem;
  onOpenFile?: (path: string) => void;
  withKey?: boolean;
  withUsage?: boolean;
}) {
  const note = itemNote(item);
  const quote = item.quote;
  return (
    <>
      {note && <p className="text-sm text-muted-foreground">{note}</p>}
      {quote && (quote.quote || quote.path) && (
        <figure className="flex flex-col gap-1 border-l-2 border-border pl-3">
          {quote.quote && (
            <blockquote className="text-sm italic" translate="no">
              “{quote.quote}”
            </blockquote>
          )}
          {quote.path && (
            <figcaption>
              {open ? (
                <button
                  type="button"
                  data-slot="open-file"
                  aria-keyshortcuts={withKey ? "o" : undefined}
                  className="group/button inline-flex items-center gap-1 font-mono text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline focus-visible:text-foreground focus-visible:underline focus-visible:outline-none"
                  onClick={() => open(quote.path!)}
                >
                  <FileText size={11} aria-hidden />
                  <span translate="no">{quote.path}</span>
                  {withKey && <Shortcut k="o" />}
                </button>
              ) : (
                <span className="font-mono text-[11px] text-muted-foreground" translate="no">
                  {quote.path}
                </span>
              )}
            </figcaption>
          )}
        </figure>
      )}
      {withUsage && item.usage && (
        <p data-slot="digest-usage" className="text-sm">
          {item.usage.line}
        </p>
      )}
    </>
  );
}

const FACTS_CLASS =
  "grid grid-cols-[minmax(6rem,auto)_minmax(0,1fr)] items-baseline gap-x-4 gap-y-1.5";

/** The facts about an item, one per line. */
function ItemFacts({ item, established }: { item: DigestItem; established?: boolean }) {
  return (
    <dl data-slot="digest-facts" className={FACTS_CLASS}>
      <Fact label={<>Noticed by</>}>
        <NoticedBy who={item.noticed_by} />
      </Fact>
      <Fact label={<>When</>}>
        <When iso={item.at} relative />
      </Fact>
      {established && item.established_at && (
        <Fact label={<>Established</>}>
          <When iso={item.established_at} relative />
        </Fact>
      )}
      {item.theme && <Fact label={<>Theme</>}>{themeLabel(item.theme)}</Fact>}
      {item.collection && (
        <Fact label={<>Collection</>}>
          <span translate="no">{item.collection}</span>
        </Fact>
      )}
      {item.scope && (
        <Fact label={<>Scope</>}>
          <span className="font-mono text-[11px]" translate="no">
            {item.scope}
          </span>
        </Fact>
      )}
      {item.standing && <Fact label={<>Standing</>}>{item.standing}</Fact>}
    </dl>
  );
}

function Fact({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-sm break-words">{children}</dd>
    </>
  );
}

type Actor = DigestItem["noticed_by"];

/**
 * The mark for who recorded an item: a person by their initials, an assistant
 * by its host's letters and hue, a kapi tool by its own name.
 */
export function NoticedMark({ who, decorative }: { who: Actor; decorative?: boolean }) {
  if (who.kind === "person") {
    return <ActorMark actor="person" name={who.name} size="sm" decorative={decorative} />;
  }
  const actor = who.kind === "tool" ? "kapi" : who.name || "agent";
  return <ActorMark actor={actor} size="sm" decorative={decorative} />;
}

/** Who recorded an item: their mark, their name, and the session it came from. */
export function NoticedBy({ who }: { who: Actor }) {
  const mark = <NoticedMark who={who} decorative />;
  let name: React.ReactNode;
  if (who.kind === "agent") name = who.name || t("an assistant");
  else if (who.kind === "tool") name = `kapi ${who.name ?? ""}`.trim();
  else name = who.name || t("you");
  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
      {mark}
      <span translate="no">{name}</span>
      {who.session && (
        <span className="font-mono text-[10px] text-muted-foreground" translate="no">
          {who.session}
        </span>
      )}
    </span>
  );
}

/** The field a rule's form is changed in, with Keep with this form, and Esc to close it. */
function ChangeForm({
  form,
  onChange,
  onKeep,
  onCancel,
  className,
}: {
  form: string;
  onChange: (form: string) => void;
  onKeep: () => void;
  onCancel: () => void;
  className?: string;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, []);
  return (
    <div data-slot="digest-change" className={cn("flex flex-wrap items-center gap-2", className)}>
      <Input
        ref={ref}
        value={form}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onKeep();
          } else if (e.key === "Escape") {
            e.preventDefault();
            onCancel();
          }
        }}
        className="h-8 max-w-64 font-mono"
        translate="no"
        aria-label={t("What to write instead")}
      />
      <Button size="sm" variant="success" data-slot="keep-changed" onClick={onKeep}>
        Keep with this form
      </Button>
      <Button size="sm" variant="ghost" onClick={onCancel} aria-keyshortcuts="Escape">
        Cancel
      </Button>
    </div>
  );
}

/**
 * `g` keeps every suggestion that shares this one's theme and collection. It
 * has no button of its own, so the pane says it is there.
 */
function GroupShortcut({ row, canDecide }: { row: DigestRow; canDecide: boolean }) {
  const count = (row.siblings ?? []).filter((s) => s.keepable).length;
  if (!canDecide || count < 2) return null;
  return (
    <p data-slot="digest-shortcuts" className="text-[11px] text-muted-foreground">
      <kbd className={KBD_CLASS}>g</kbd>{" "}
      {t("keeps all {count} suggestions in this theme and collection", { count })}
    </p>
  );
}

const KBD_CLASS = "rounded border border-border px-1 font-mono text-[10px]";

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
