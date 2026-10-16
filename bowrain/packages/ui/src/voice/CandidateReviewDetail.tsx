// The detail pane of a voice profile's suggested-rule review: the selected
// row's rule, the corrections behind it, who decided it and when, and the acts
// open on it in the toolbar.
//
// A suggestion offers Promote, Change, Reject and Preview impact. Promote
// writes the rule into the workspace's terms as a versioned check, Change
// promotes it with another form, Reject stops the phrasing from surfacing
// again, and Preview impact counts what the rule would newly flag in the
// project chosen in the page header. A disagreement lays its forms beside each
// other, each with its own Choose this form; Reject sets the phrasing aside
// whichever form it was corrected to. A rule already decided offers no act,
// because the platform keeps that decision as the record.
import { useEffect, useRef } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ActorMark,
  Button,
  Input,
  KindIcon,
  ListDetailEmpty,
  ListDetailPane,
  ListDetailToolbar,
  When,
  cn,
} from "@neokapi/ui-primitives";
import { ErrorNotice } from "../errors";
import type { CandidateRule } from "./types";
import { dimensionKind, type CandidateRow } from "./candidateRows";
import { CandidateSentence, ConflictQuestion } from "./CandidateRule";

/** A rule's form being changed before it is promoted: the row, the form it started from, and what is typed. */
export interface CandidateChange {
  key: string;
  /** The form the change started from, which names the side of a disagreement. */
  from: string;
  form: string;
}

/** Resolves a member's user id to the name the workspace knows them by. */
export type NameOf = (id: string | null | undefined) => string;

export interface CandidateDetailProps {
  row: CandidateRow;
  /** The reader may decide: false hides the acts. */
  canDecide: boolean;
  /** An act on this row is in flight: its buttons are disabled. */
  busy?: boolean;
  /** Why the last act on this row failed, when it did. */
  error?: unknown;
  /** The change in progress, when it is on this row. */
  change: CandidateChange | null;
  /** Promote a candidate, with another form when one is given. */
  onPromote: (candidate: CandidateRule, replacement?: string) => void;
  /** Reject a phrasing, every form of it. */
  onReject: (candidate: CandidateRule) => void;
  /** Preview the blast radius of promoting a form. Absent hides the act. */
  onEvaluate?: (candidate: CandidateRule) => void;
  /** Open the concept a phrasing denotes. Absent shows the fact without a link. */
  onOpenConcept?: (conceptId: string) => void;
  onStartChange: (candidate: CandidateRule) => void;
  onChangeForm: (form: string) => void;
  onCancelChange: () => void;
  nameOf?: NameOf;
  /** The signed-in member, whose own decisions read as "you". */
  currentUserId?: string;
}

/** The detail pane for one row of the list. */
export function CandidateDetail(props: CandidateDetailProps) {
  return props.row.group === "conflict" ? (
    <ConflictDetail {...props} />
  ) : (
    <CandidateRuleDetail {...props} />
  );
}

/** What the detail pane shows when no row waits for a decision. */
export function CandidateNothingSelected() {
  return (
    <ListDetailEmpty
      title={t("Nothing needs you")}
      description={t(
        "Every suggested rule here is decided. Open Established or Rejected to see who decided each one.",
      )}
    />
  );
}

// ── A suggestion, or a rule already decided ──────────────────────────────────

function CandidateRuleDetail({
  row,
  canDecide,
  busy,
  error,
  change,
  onPromote,
  onReject,
  onEvaluate,
  onOpenConcept,
  onStartChange,
  onChangeForm,
  onCancelChange,
  nameOf,
  currentUserId,
}: CandidateDetailProps) {
  const { candidate } = row;
  const open = row.group === "suggested" || row.group === "approved";
  const changing = change && change.from === candidate.replacement ? change : null;
  const toolbar = changing ? (
    <ChangeForm
      form={changing.form}
      busy={busy}
      onChange={onChangeForm}
      onPromote={() => onPromote(candidate, changing.form)}
      onCancel={onCancelChange}
      className="border-b border-border pb-2.5"
    />
  ) : (
    <ListDetailToolbar
      title={<CandidateSentence candidate={candidate} />}
      actions={
        canDecide &&
        open && (
          <>
            <Button
              size="sm"
              variant="success"
              data-slot="promote-candidate"
              aria-keyshortcuts="a"
              disabled={busy}
              onClick={() => onPromote(candidate)}
            >
              Promote
              <Shortcut k="a" />
            </Button>
            <Button
              size="sm"
              variant="outline"
              data-slot="change-candidate"
              aria-keyshortcuts="c"
              disabled={busy}
              onClick={() => onStartChange(candidate)}
            >
              Change
              <Shortcut k="c" />
            </Button>
            <Button
              size="sm"
              variant="ghost"
              data-slot="reject-candidate"
              aria-keyshortcuts="d"
              disabled={busy}
              onClick={() => onReject(candidate)}
            >
              Reject
              <Shortcut k="d" />
            </Button>
            {onEvaluate && (
              <Button
                size="sm"
                variant="outline"
                data-slot="evaluate-candidate"
                disabled={busy}
                onClick={() => onEvaluate(candidate)}
              >
                Preview impact
              </Button>
            )}
          </>
        )
      }
    />
  );
  return (
    <ListDetailPane data-slot="candidate-detail" toolbar={toolbar} className="max-w-3xl">
      {error ? <ActError error={error} /> : null}
      <dl data-slot="candidate-facts" className={FACTS_CLASS}>
        <Fact label={<>Corrections</>}>
          <span className="tabular-nums">{candidate.correction_count}</span>
        </Fact>
        <Fact label={<>Kind</>}>
          <span className="inline-flex items-center gap-1.5">
            <KindIcon kind={dimensionKind(candidate.dimension)} decorative />
            {dimensionLabel(candidate.dimension)}
          </span>
        </Fact>
        <ConceptFact candidate={candidate} onOpenConcept={onOpenConcept} />
        <DecisionFacts candidate={candidate} nameOf={nameOf} currentUserId={currentUserId} />
      </dl>
    </ListDetailPane>
  );
}

// ── A disagreement ───────────────────────────────────────────────────────────

function ConflictDetail({
  row,
  canDecide,
  busy,
  error,
  change,
  onPromote,
  onReject,
  onEvaluate,
  onOpenConcept,
  onStartChange,
  onChangeForm,
  onCancelChange,
}: CandidateDetailProps) {
  const sides = row.sides ?? [row.candidate];
  return (
    <ListDetailPane
      data-slot="candidate-detail"
      toolbar={
        <ListDetailToolbar
          title={<ConflictQuestion row={row} />}
          actions={
            canDecide && (
              <Button
                size="sm"
                variant="ghost"
                data-slot="reject-candidate"
                aria-keyshortcuts="d"
                disabled={busy}
                onClick={() => onReject(row.candidate)}
              >
                Reject
                <Shortcut k="d" />
              </Button>
            )
          }
        />
      }
    >
      {error ? <ActError error={error} /> : null}
      <div className="@container">
        <div data-slot="candidate-sides" className="grid gap-3 @xl:grid-cols-2">
          {sides.map((side, i) => {
            const changing = change && change.from === side.replacement ? change : null;
            return (
              <div
                key={side.replacement}
                data-slot="candidate-side"
                className="flex min-w-0 flex-col gap-3 rounded-lg border border-border p-3"
              >
                <p className="text-sm font-semibold">
                  <CandidateSentence candidate={side} />
                </p>
                <dl className={FACTS_CLASS}>
                  <Fact label={<>Corrections</>}>
                    <span className="tabular-nums">{side.correction_count}</span>
                  </Fact>
                  <Fact label={<>Kind</>}>{dimensionLabel(side.dimension)}</Fact>
                </dl>
                {canDecide && changing && (
                  <ChangeForm
                    form={changing.form}
                    busy={busy}
                    onChange={onChangeForm}
                    onPromote={() => onPromote(side, changing.form)}
                    onCancel={onCancelChange}
                  />
                )}
                {canDecide && !changing && (
                  <div
                    data-slot="candidate-side-acts"
                    className="flex flex-wrap items-center gap-1.5"
                  >
                    <Button
                      size="sm"
                      variant={i === 0 ? "success" : "outline"}
                      data-slot="promote-candidate"
                      aria-keyshortcuts={i === 0 ? "a" : undefined}
                      disabled={busy}
                      onClick={() => onPromote(side)}
                    >
                      Choose this form
                      {i === 0 && <Shortcut k="a" />}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      data-slot="change-candidate"
                      aria-keyshortcuts={i === 0 ? "c" : undefined}
                      disabled={busy}
                      onClick={() => onStartChange(side)}
                    >
                      Change
                      {i === 0 && <Shortcut k="c" />}
                    </Button>
                    {onEvaluate && (
                      <Button
                        size="sm"
                        variant="ghost"
                        data-slot="evaluate-candidate"
                        disabled={busy}
                        onClick={() => onEvaluate(side)}
                      >
                        Preview impact
                      </Button>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
      {row.candidate.concept_id && (
        <dl className={FACTS_CLASS}>
          <ConceptFact candidate={row.candidate} onOpenConcept={onOpenConcept} />
        </dl>
      )}
    </ListDetailPane>
  );
}

// ── The parts ────────────────────────────────────────────────────────────────

const FACTS_CLASS =
  "grid grid-cols-[minmax(6rem,auto)_minmax(0,1fr)] items-baseline gap-x-4 gap-y-1.5";

function Fact({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-sm break-words">{children}</dd>
    </>
  );
}

/** The concept a phrasing denotes, a link when the page can open it. `o` follows it too. */
function ConceptFact({
  candidate,
  onOpenConcept,
}: {
  candidate: CandidateRule;
  onOpenConcept?: (conceptId: string) => void;
}) {
  const id = candidate.concept_id;
  if (!id) return null;
  return (
    <Fact label={<>Concept</>}>
      {onOpenConcept ? (
        <button
          type="button"
          data-slot="open-concept"
          aria-keyshortcuts="o"
          onClick={() => onOpenConcept(id)}
          className="group/button inline-flex items-center gap-1 underline-offset-2 hover:underline focus-visible:underline focus-visible:outline-none"
        >
          <span translate="no">{candidate.term}</span>
          <Shortcut k="o" />
        </button>
      ) : (
        <span translate="no">{candidate.term}</span>
      )}
    </Fact>
  );
}

/** Who decided a rule and when, the version it landed in, and whether a threshold decided it. */
function DecisionFacts({
  candidate,
  nameOf,
  currentUserId,
}: {
  candidate: CandidateRule;
  nameOf?: NameOf;
  currentUserId?: string;
}) {
  if (candidate.status === "pending") return null;
  return (
    <>
      <Fact label={<>Status</>}>{statusLabel(candidate)}</Fact>
      {(candidate.decided_by || candidate.auto) && (
        <Fact label={<>Decided by</>}>
          <DecidedBy candidate={candidate} nameOf={nameOf} currentUserId={currentUserId} />
        </Fact>
      )}
      {candidate.decided_at && (
        <Fact label={<>When</>}>
          <When iso={candidate.decided_at} relative />
        </Fact>
      )}
      {candidate.status === "promoted" && candidate.promoted_version ? (
        <Fact label={<>Version</>}>
          <span className="tabular-nums">
            {t("v{version}", "a voice profile version", {
              version: candidate.promoted_version,
            })}
          </span>
        </Fact>
      ) : null}
    </>
  );
}

/** A decision's author: a member by their mark and name, or the profile's threshold. */
export function DecidedBy({
  candidate,
  nameOf,
  currentUserId,
}: {
  candidate: CandidateRule;
  nameOf?: NameOf;
  currentUserId?: string;
}) {
  if (candidate.auto) return <>Automatically, at the profile&apos;s threshold</>;
  const id = candidate.decided_by;
  if (!id) return null;
  const name = nameOf?.(id) || id;
  const you = !!currentUserId && id === currentUserId;
  return (
    <span className="inline-flex min-w-0 items-center gap-1.5">
      <ActorMark actor="person" name={name} size="sm" decorative />
      <span translate="no">{you ? t("You") : name}</span>
    </span>
  );
}

/** The mark at the end of a decided row: who decided it, or "auto" for a threshold. */
export function DecidedMark({ candidate, nameOf }: { candidate: CandidateRule; nameOf?: NameOf }) {
  if (candidate.auto) {
    return <span className="text-[11px] text-muted-foreground">{t("auto")}</span>;
  }
  if (!candidate.decided_by) return null;
  return (
    <ActorMark
      actor="person"
      name={nameOf?.(candidate.decided_by) || candidate.decided_by}
      size="sm"
    />
  );
}

function statusLabel(candidate: CandidateRule): string {
  switch (candidate.status) {
    case "approved":
      return t("Approved, not yet promoted");
    case "promoted":
      return candidate.auto ? t("Promoted automatically") : t("Promoted");
    case "rejected":
      return t("Rejected");
    default:
      return t("Suggested");
  }
}

/** A voice dimension's name, written here so it is translated. */
export function dimensionLabel(dimension: string): string {
  switch (dimension) {
    case "vocabulary":
      return t("Vocabulary");
    case "tone":
      return t("Tone");
    case "style":
      return t("Style");
    case "clarity":
      return t("Clarity");
    case "compliance":
      return t("Compliance");
  }
  return dimension;
}

function ActError({ error }: { error: unknown }) {
  return (
    <ErrorNotice
      error={error}
      title={t("The decision was not recorded")}
      hint={t("Promoting and rejecting need the manage voice permission in this workspace.")}
      variant="inline"
    />
  );
}

/** The field a rule's form is changed in, with Promote with this form, and Esc to close it. */
function ChangeForm({
  form,
  busy,
  onChange,
  onPromote,
  onCancel,
  className,
}: {
  form: string;
  busy?: boolean;
  onChange: (form: string) => void;
  onPromote: () => void;
  onCancel: () => void;
  className?: string;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, []);
  return (
    <div
      data-slot="candidate-change"
      className={cn("flex flex-wrap items-center gap-2", className)}
    >
      <Input
        ref={ref}
        value={form}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onPromote();
          } else if (e.key === "Escape") {
            e.preventDefault();
            onCancel();
          }
        }}
        className="h-8 max-w-64 font-mono"
        translate="no"
        aria-label={t("What to write instead")}
      />
      <Button
        size="sm"
        variant="success"
        data-slot="promote-changed"
        disabled={busy}
        onClick={onPromote}
      >
        Promote with this form
      </Button>
      <Button size="sm" variant="ghost" onClick={onCancel} aria-keyshortcuts="Escape">
        Cancel
      </Button>
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
