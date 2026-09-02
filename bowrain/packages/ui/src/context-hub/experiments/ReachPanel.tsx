// The reach panel: what approving this draft costs, split by the act it asks
// for. The blast radius above already says how much content moves; this says
// what moving it means. An ANNOTATION changes what the checks flag — the next
// check fans it out and the affected translations come back into review. A
// TRANSFORM tells someone to rewrite the source, which invalidates every
// translation of that block and lands on whoever may edit source.
//
// The bar is the whole design: the two classes partition the affected blocks
// exactly, so their widths are the answer and the sentences underneath are the
// detail. Deliberately not a second row of stat tiles — the hero above owns the
// counting, and this owns the division.
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import { Card, CardContent, cn } from "@neokapi/ui-primitives";
import { Pencil, Tag, Users } from "../../components/icons";
import type { ChangeSetImpact, ReachClass } from "../../types/brand-graph";
import { formatLocales, hasReach, reachSegments, splitCoversAffected } from "./reach";

export interface ReachPanelProps {
  impact?: ChangeSetImpact;
  className?: string;
}

export function ReachPanel({ impact, className }: ReachPanelProps) {
  if (!hasReach(impact)) return null;

  const { reach } = impact;
  const segments = reachSegments(reach);
  const stored = impact.stored === true;

  return (
    <Card className={className}>
      <CardContent className="space-y-4 p-4">
        <div className="space-y-1">
          <h3 className="text-sm font-medium">What this asks for</h3>
          <p className="text-xs text-muted-foreground">
            Every affected block falls to one of two acts. They cost different work.
          </p>
        </div>

        <SplitBar segments={segments} />

        {reach.annotate.blocks > 0 && (
          <ReachRow
            icon={<Tag />}
            tone="annotate"
            title="Re-check fans out"
            cls={reach.annotate}
            detail={
              reach.annotate.targets > 0 ? (
                <>
                  <Plural count={reach.annotate.targets}>
                    <One>
                      {reach.annotate.targets} translation returns to review
                      {localeTail(reach.annotate)}
                    </One>
                    <Other>
                      {reach.annotate.targets} translations return to review
                      {localeTail(reach.annotate)}
                    </Other>
                  </Plural>
                  {reach.annotate.approved > 0 && (
                    <>
                      {" · "}
                      <span className="text-warning">
                        {reach.annotate.approved} already approved
                      </span>
                    </>
                  )}
                  .
                </>
              ) : (
                <>Nothing has been translated yet, so nothing comes back.</>
              )
            }
          />
        )}

        {reach.transform.blocks > 0 && (
          <ReachRow
            icon={<Pencil />}
            tone="transform"
            title="Source gets rewritten"
            cls={reach.transform}
            detail={
              <>
                The draft names what to write instead, so acting on these edits the source.
                {reach.transform.targets > 0 ? (
                  <>
                    {" "}
                    <Plural count={reach.transform.targets}>
                      <One>
                        That invalidates {reach.transform.targets} translation
                        {localeTail(reach.transform)}.
                      </One>
                      <Other>
                        That invalidates {reach.transform.targets} translations
                        {localeTail(reach.transform)}.
                      </Other>
                    </Plural>
                  </>
                ) : null}
              </>
            }
          />
        )}

        {reach.transform_projects.length > 0 && (
          <div className="flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-xs">
            <Users className="mt-0.5 size-3.5 shrink-0 text-warning" />
            <p className="text-muted-foreground">
              <span className="font-medium text-foreground">Not yours to finish.</span> A source
              rewrite is decided by whoever may edit source in{" "}
              {reach.transform_projects.map((p) => p.project_name || p.project_id).join(", ")}.
            </p>
          </div>
        )}

        {stored ? (
          <p className="text-[11px] text-muted-foreground">
            Measured when this change-set was submitted. Re-measure for the locales behind these
            numbers.
          </p>
        ) : (
          !splitCoversAffected(impact) && (
            <p className="text-[11px] text-muted-foreground">
              The split covers {reachSum(impact)} of {impact.affected_blocks.toLocaleString()}{" "}
              affected rows: a block reached in several locales is one block to act on.
            </p>
          )
        )}
      </CardContent>
    </Card>
  );
}

function reachSum(impact: ChangeSetImpact): string {
  const reach = impact.reach;
  if (!reach) return "0";
  return (reach.annotate.blocks + reach.transform.blocks).toLocaleString();
}

/** The trailing " in de, nb" of a sentence, or nothing when no locale is known. */
function localeTail(cls: ReachClass) {
  const locales = formatLocales(cls.locales);
  if (!locales) return null;
  return <> in {locales}</>;
}

// ── The split bar ────────────────────────────────────────────────────────────

/**
 * One bar, divided in proportion. The label sits inside a segment wide enough to
 * hold it and is dropped otherwise — a legend under a two-colour bar restates
 * what the rows below already say.
 */
function SplitBar({ segments }: { segments: ReturnType<typeof reachSegments> }) {
  return (
    <div className="flex h-8 w-full overflow-hidden rounded-md border">
      {segments.map((s) => (
        <div
          key={s.kind}
          className={cn(
            "flex items-center justify-center overflow-hidden whitespace-nowrap px-2 text-[11px] font-medium tabular-nums transition-[flex-grow]",
            s.kind === "annotate"
              ? "bg-primary/15 text-primary"
              : "bg-warning/25 text-warning-foreground",
          )}
          style={{ flexGrow: s.blocks, flexBasis: 0 }}
        >
          {s.share >= 0.2 && (
            <span>
              {s.blocks.toLocaleString()} {s.kind === "annotate" ? "re-checked" : "rewritten"}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

// ── One class ────────────────────────────────────────────────────────────────

function ReachRow({
  icon,
  tone,
  title,
  cls,
  detail,
}: {
  icon: React.ReactNode;
  tone: "annotate" | "transform";
  title: string;
  cls: ReachClass;
  detail: React.ReactNode;
}) {
  return (
    <div className="flex gap-3">
      <div
        className={cn(
          "mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md [&_svg]:size-3.5",
          tone === "annotate" ? "bg-primary/10 text-primary" : "bg-warning/15 text-warning",
        )}
      >
        {icon}
      </div>
      <div className="min-w-0 space-y-0.5">
        <div className="flex flex-wrap items-baseline gap-x-1.5 text-sm">
          <span className="font-medium text-foreground">{title}</span>
          <span className="text-muted-foreground">
            <Plural count={cls.blocks}>
              <One>{cls.blocks.toLocaleString()} block</One>
              <Other>{cls.blocks.toLocaleString()} blocks</Other>
            </Plural>
            {cls.collections > 0 && (
              <>
                {" in "}
                <Plural count={cls.collections}>
                  <One>{cls.collections} collection</One>
                  <Other>{cls.collections} collections</Other>
                </Plural>
              </>
            )}
            {cls.projects > 1 && <> across {cls.projects} projects</>}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">{detail}</p>
      </div>
    </div>
  );
}
