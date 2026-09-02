// The trial: what the checks say on one stream, before and after.
//
// The blast radius counts and the reach panel prices; this one NAMES. For the
// stream a pilot binds the draft to, it lists the findings the draft would raise
// and the ones it would clear, each carrying the rule that fired and the text it
// fired on — so a reviewer can see whether the draft says what they meant it to
// say before it reaches the live graph.
//
// The panel states which half of the answer is live and which is computed. The
// voice half really does resolve through the draft on a bound stream; the terms
// half is applied for this report, because no check resolves terms per stream. A
// trial that claimed both would invite trust in a mechanism that is not there.
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import { Badge, Button, Card, CardContent, Skeleton, cn } from "@neokapi/ui-primitives";
import { AlertTriangle, CircleCheck, FlaskConical } from "../../components/icons";
import { ErrorNotice } from "../../errors";
import type { TrialFinding, TrialReport } from "../../types/brand-graph";
import { useTrialFindings } from "../../hooks/useChangesetsApi";

export interface TrialPanelProps {
  changesetId: string;
  projectId: string;
  stream: string;
  /** The project's display name, when the caller knows it. */
  projectName?: string;
  className?: string;
}

export function TrialPanel({
  changesetId,
  projectId,
  stream,
  projectName,
  className,
}: TrialPanelProps) {
  const { data, isLoading, error, refetch } = useTrialFindings(changesetId, projectId, stream);

  if (error) {
    return (
      <ErrorNotice
        error={error}
        title="Couldn't run the trial"
        hint="The stream may be too large to scan in one request. Try again, or read the blast radius instead."
        variant="panel"
        onRetry={() => void refetch()}
        className={className}
      />
    );
  }
  if (isLoading) {
    return <Skeleton className={cn("h-40 w-full", className)} />;
  }
  if (!data) return null;

  return (
    <Card className={className}>
      <CardContent className="space-y-4 p-4">
        <Header report={data} projectName={projectName ?? projectId} />
        {data.partial && (
          <p className="flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-xs text-muted-foreground">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" />
            <span>
              <span className="font-medium text-foreground">Partial.</span> The scan stopped before
              it had covered the stream, so anything it did not reach is absent rather than clean.
              {data.partial_reason ? ` (${data.partial_reason})` : null}
            </span>
          </p>
        )}

        {data.changed_blocks === 0 ? (
          <p className="rounded-lg border border-dashed bg-muted/20 px-4 py-6 text-center text-sm text-muted-foreground">
            Nothing changes on this stream. The draft matched none of its{" "}
            {data.total_blocks.toLocaleString()} scanned rows.
          </p>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            <FindingList
              tone="raised"
              title="The draft would raise"
              shown={data.raised}
              total={data.raised_total}
            />
            <FindingList
              tone="cleared"
              title="The draft would clear"
              shown={data.cleared}
              total={data.cleared_total}
            />
          </div>
        )}

        <Provenance report={data} />
      </CardContent>
    </Card>
  );
}

function Header({ report, projectName }: { report: TrialReport; projectName: string }) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
      <div className="flex items-center gap-1.5">
        <FlaskConical className="size-4 text-muted-foreground" />
        <h3 className="text-sm font-medium">Trial on {projectName}</h3>
      </div>
      <Badge variant="outline" className="font-mono text-[10px]">
        {report.stream}
      </Badge>
      <span className="text-xs text-muted-foreground">
        <Plural count={report.changed_blocks}>
          <One>
            {report.changed_blocks} of {report.total_blocks.toLocaleString()} row changes
          </One>
          <Other>
            {report.changed_blocks.toLocaleString()} of {report.total_blocks.toLocaleString()} rows
            change
          </Other>
        </Plural>
      </span>
    </div>
  );
}

/**
 * How much of this answer is a resolution and how much is a computation. The
 * distinction decides what a reviewer may conclude from a quiet trial, so it is
 * stated on the panel rather than left to the reader.
 */
function Provenance({ report }: { report: TrialReport }) {
  return (
    <p className="text-[11px] leading-relaxed text-muted-foreground">
      {report.voice_bound ? (
        <>
          Voice resolves through this draft on this stream. The pilot bound a candidate profile, so
          a check running here reads it.{" "}
        </>
      ) : (
        <>Voice is computed here: no candidate profile is bound to this stream. </>
      )}
      {report.terms_computed && (
        <>
          Terms are computed for this report. Checks read terms without naming a stream, so a
          draft&rsquo;s terms never govern a branch on their own.
        </>
      )}
    </p>
  );
}

function FindingList({
  tone,
  title,
  shown,
  total,
}: {
  tone: "raised" | "cleared";
  title: string;
  shown: TrialFinding[];
  total: number;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        {tone === "raised" ? (
          <AlertTriangle className="size-3.5 text-destructive" />
        ) : (
          <CircleCheck className="size-3.5 text-success" />
        )}
        <h4 className="text-xs font-medium text-foreground">{title}</h4>
        <span className="text-xs tabular-nums text-muted-foreground">{total.toLocaleString()}</span>
      </div>
      {shown.length === 0 ? (
        <p className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
          {tone === "raised" ? "No new findings." : "No findings go away."}
        </p>
      ) : (
        <ul className="space-y-1.5">
          {shown.slice(0, 6).map((f, i) => (
            <FindingRow key={`${f.block_id}-${f.kind}-${f.rule}-${i}`} finding={f} tone={tone} />
          ))}
        </ul>
      )}
      {total > Math.min(shown.length, 6) && (
        <p className="text-[11px] text-muted-foreground">
          <Plural count={total - Math.min(shown.length, 6)}>
            <One>and {total - Math.min(shown.length, 6)} more.</One>
            <Other>and {total - Math.min(shown.length, 6)} more.</Other>
          </Plural>
        </p>
      )}
    </div>
  );
}

function FindingRow({ finding, tone }: { finding: TrialFinding; tone: "raised" | "cleared" }) {
  return (
    <li className="rounded-md border bg-card px-3 py-2">
      <div className="flex flex-wrap items-baseline gap-x-1.5 text-xs">
        <span
          className={cn("font-medium", tone === "raised" ? "text-destructive" : "text-success")}
        >
          {finding.rule}
        </span>
        {finding.replacement && (
          <span className="text-muted-foreground">&rarr; {finding.replacement}</span>
        )}
        <Badge variant="outline" className="text-[10px]">
          {finding.kind === "term" ? "term" : "voice"}
        </Badge>
        {finding.severity && (
          <span className="text-[10px] text-muted-foreground">{finding.severity}</span>
        )}
      </div>
      <p className="mt-1 line-clamp-2 text-xs text-foreground">{finding.text}</p>
      <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10px] text-muted-foreground">
        <span className="truncate">{finding.item_name}</span>
        {finding.collection_name && <span>· {finding.collection_name}</span>}
        <Badge variant="outline" className="font-mono text-[9px]">
          {finding.locale}
        </Badge>
      </div>
    </li>
  );
}

/**
 * The trial's entry point on a pilot row: a button that reveals the diff in
 * place. It is not fetched until asked for — a trial is a walk, and a detail
 * page that ran one per pilot on load would pay for answers nobody looked at.
 */
export function TrialToggle({
  open,
  onToggle,
  disabled,
}: {
  open: boolean;
  onToggle: () => void;
  disabled?: boolean;
}) {
  return (
    <Button
      size="sm"
      variant={open ? "secondary" : "outline"}
      className="h-7 px-2 text-[11px]"
      onClick={onToggle}
      disabled={disabled}
    >
      {open ? "Hide findings" : "Compare findings"}
    </Button>
  );
}
