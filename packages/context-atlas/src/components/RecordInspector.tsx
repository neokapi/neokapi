import { useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { ActorMark, Button, CoordinateChip, SimpleTooltip } from "@neokapi/ui-primitives";
import { Check, Copy, ExternalLink, X } from "lucide-react";
import { stateFill } from "../colors";
import { axisLabel, formatCount, formatDay, stateLabel, valueLabel } from "../labels";
import { type Atlas, coordinateOf, preparedStateAt } from "../model/atlas";
import { type EvalContext, lifetimeAt } from "../model/filter";
import { dayOf, dayStart } from "../model/time";
import { StateBadge } from "./StateBadge";

const CHIP_AXIS: Record<string, string> = { locale: "language" };
const HIDDEN_AXES = new Set(["family", "actor"]);

export interface RecordInspectorProps {
  atlas: Atlas;
  recordId: string;
  ctx: EvalContext;
  onClose: () => void;
  onOpenEvidence?: (path: string) => void;
}

// webURL returns the evidence URL when it is an http(s) page, and undefined for
// any other scheme. Evidence is recorded by agents, so a `javascript:` or
// `data:` URL must never reach an href.
function webURL(raw: string): string | undefined {
  try {
    const url = new URL(raw);
    return url.protocol === "http:" || url.protocol === "https:" ? url.href : undefined;
  } catch {
    return undefined;
  }
}

function lastSegment(path: string): string {
  const parts = path.split("/").filter(Boolean);
  return parts[parts.length - 1] ?? path;
}

function actorId(kind: string | undefined, name: string | undefined): string {
  if (kind === "agent" && name) return name;
  if (kind === "tool") return name || "tool";
  return "person";
}

/** One record in full: what it says, where it applies, when, and its history. */
export function RecordInspector({
  atlas,
  recordId,
  ctx,
  onClose,
  onOpenEvidence,
}: RecordInspectorProps) {
  const p = atlas.byId.get(recordId);
  const [copied, setCopied] = useState(false);
  if (!p) return null;
  const r = p.record;
  const state = preparedStateAt(p, ctx.snapshotMs);
  const coords = atlas.axes
    .filter((a) => a.origin !== "view" && !HIDDEN_AXES.has(a.id))
    .map((a) => [a.id, coordinateOf(p, a.id)] as const)
    .filter(([, v]) => v !== "");
  const lifetime = lifetimeAt(p, ctx.snapshotMs);
  // Each end of the window is resolved before it enters the sentence: the
  // i18n transform cannot carry a t() call nested in another's arguments.
  const windowFrom = r.valid_from
    ? formatDay(dayOf(Date.parse(r.valid_from)), true)
    : t("always", "window start");
  const windowTo = r.valid_to
    ? formatDay(dayOf(Date.parse(r.valid_to)), true)
    : t("no end", "window end");
  const command = r.short ? `kapi context log ${r.short}` : null;

  const copy = async () => {
    if (!command) return;
    try {
      await navigator.clipboard?.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard access can be refused; the command stays visible to select.
    }
  };

  return (
    <section
      className="flex flex-col gap-4"
      aria-label={t("Record", "atlas detail pane")}
      data-testid="atlas-inspector"
    >
      <header className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <h3 className="text-sm leading-snug font-semibold break-words">{r.label}</h3>
          <div className="mt-1.5 flex flex-wrap items-center gap-2">
            <StateBadge state={state} />
            <span className="text-[11px] text-muted-foreground">
              {valueLabel(atlas, "family", r.family)}
            </span>
            {r.weight > 1 && (
              <span className="text-[11px] text-muted-foreground tabular-nums">
                {t("{count} units", "content cell size", { count: formatCount(r.weight) })}
              </span>
            )}
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t("Close record", "atlas detail pane")}
          onClick={onClose}
        >
          <X aria-hidden />
        </Button>
      </header>

      {coords.length > 0 && (
        <div
          className="flex flex-wrap gap-1"
          aria-label={t("Where it applies", "record coordinates")}
        >
          {coords.map(([axis, value]) => (
            <CoordinateChip
              key={axis}
              axis={CHIP_AXIS[axis] ?? axis}
              label={axisLabel(atlas, axis)}
              value={valueLabel(atlas, axis, value)}
            />
          ))}
        </div>
      )}
      {/* The chips above are the point; the scope line says only what they
          cannot, a rule held across the workspace. */}
      {r.scope?.startsWith("workspace") && (
        <p className="text-[11px] text-muted-foreground">Holds in every project</p>
      )}

      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
        <dt className="text-[11px] leading-5 text-muted-foreground">Recorded</dt>
        <dd className="tabular-nums">
          {Number.isNaN(p.recordedMs) ? (
            <span className="text-muted-foreground">Unknown</span>
          ) : (
            // A day on the atlas's own clock, as the history and the timeline
            // write it, so one record never reads as two dates.
            <span title={r.recorded}>{formatDay(dayOf(p.recordedMs), true)}</span>
          )}
        </dd>
        <dt className="text-[11px] leading-5 text-muted-foreground">In force</dt>
        <dd className="flex flex-col gap-1">
          <span className="tabular-nums">
            {r.valid_from || r.valid_to
              ? t("{from} to {to}", "a lifetime window", { from: windowFrom, to: windowTo })
              : t("No window", "record carries no lifetime window")}
          </span>
          {lifetime !== "open" && (
            <WindowBar
              atlas={atlas}
              from={p.validFromMs}
              to={p.validToMs}
              snapshot={ctx.snapshotMs}
              lifetime={lifetime}
            />
          )}
        </dd>
        {r.actor && (
          <>
            <dt className="text-[11px] leading-5 text-muted-foreground">By</dt>
            <dd className="flex items-center gap-1.5">
              <ActorMark
                actor={actorId(r.actor.kind, r.actor.name)}
                name={r.actor.kind === "person" ? r.actor.name : undefined}
                size="sm"
                decorative
              />
              <span className="truncate">
                {r.actor.name || valueLabel(atlas, "actor", r.actor.kind)}
              </span>
            </dd>
          </>
        )}
      </dl>

      {r.history.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <h4 className="text-xs font-medium text-muted-foreground uppercase">History</h4>
          <ol className="flex flex-col">
            {p.historyAt.map((at, i) => (
              <li key={`${at}-${i}`} className="relative flex items-center gap-2 py-1 pl-4">
                <span
                  aria-hidden
                  className="absolute left-0 size-2.5 rounded-full border border-background"
                  style={{
                    background:
                      stateFill(p.historyState[i]) === "transparent"
                        ? "var(--border)"
                        : stateFill(p.historyState[i]),
                  }}
                />
                {i < p.historyAt.length - 1 && (
                  <span
                    aria-hidden
                    className="absolute top-1/2 left-[4.5px] h-full w-px bg-border"
                  />
                )}
                <span className="text-sm">{stateLabel(p.historyState[i])}</span>
                <span className="ml-auto text-[11px] text-muted-foreground tabular-nums">
                  {formatDay(dayOf(at), true)}
                </span>
              </li>
            ))}
          </ol>
        </div>
      )}

      {r.text && r.text !== r.label && <p className="text-sm leading-relaxed">{r.text}</p>}

      {r.evidence && r.evidence.length > 0 && (
        <div className="flex flex-col gap-2">
          <h4 className="text-xs font-medium text-muted-foreground uppercase">Seen in</h4>
          {r.evidence.map((ev, i) => (
            <figure key={i} className="flex flex-col gap-1 border-l-2 border-border pl-3">
              {ev.quote && <blockquote className="text-sm leading-relaxed">{ev.quote}</blockquote>}
              <figcaption className="flex items-center gap-2 text-[11px] text-muted-foreground">
                {ev.path &&
                  (onOpenEvidence ? (
                    <SimpleTooltip content={ev.path}>
                      <button
                        type="button"
                        className="cursor-pointer truncate font-mono underline-offset-2 hover:text-foreground hover:underline"
                        onClick={() => onOpenEvidence(ev.path!)}
                        aria-label={t("Open {path}", "evidence path", { path: ev.path })}
                      >
                        {lastSegment(ev.path)}
                      </button>
                    </SimpleTooltip>
                  ) : (
                    <span className="truncate font-mono" title={ev.path}>
                      {lastSegment(ev.path)}
                    </span>
                  ))}
                {ev.unit && <span className="font-mono">{ev.unit}</span>}
                {ev.url &&
                  (webURL(ev.url) ? (
                    <a
                      href={webURL(ev.url)}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 hover:text-foreground"
                      title={ev.url}
                    >
                      <ExternalLink aria-hidden className="size-3" />
                      {lastSegment(ev.url.replace(/^https?:\/\//u, ""))}
                    </a>
                  ) : (
                    <span className="truncate font-mono" title={ev.url}>
                      {lastSegment(ev.url)}
                    </span>
                  ))}
              </figcaption>
            </figure>
          ))}
        </div>
      )}

      {r.contested_by && r.contested_by.length > 0 && (
        <div className="flex flex-col gap-1">
          <h4 className="text-xs font-medium text-muted-foreground uppercase">Contested by</h4>
          <ul className="flex flex-wrap gap-1">
            {r.contested_by.map((id) => (
              <li key={id} className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-[11px]">
                {id}
              </li>
            ))}
          </ul>
        </div>
      )}

      {command && (
        <div className="flex items-center gap-1 self-start rounded-md border bg-muted/50 py-0.5 pr-0.5 pl-2">
          <code className="font-mono text-[11px]">{command}</code>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t("Copy command", "cli chip")}
            onClick={copy}
          >
            {copied ? <Check aria-hidden /> : <Copy aria-hidden />}
          </Button>
        </div>
      )}
    </section>
  );
}

/** The lifetime window on the span, with the snapshot marked. */
function WindowBar({
  atlas,
  from,
  to,
  snapshot,
  lifetime,
}: {
  atlas: Atlas;
  from: number;
  to: number;
  snapshot: number;
  lifetime: string;
}) {
  const lo = dayStart(atlas.firstDay);
  const hi = dayStart(atlas.lastDay) + 86_400_000;
  const W = 150;
  const x = (ms: number) => Math.max(0, Math.min(W, ((ms - lo) / (hi - lo)) * W));
  const x0 = Number.isNaN(from) ? 0 : x(from);
  const x1 = Number.isNaN(to) ? W : x(to);
  return (
    <span className="flex items-center gap-2">
      <svg width={W} height={10} aria-hidden className="shrink-0">
        <rect x={0} y={4} width={W} height={2} rx={1} fill="var(--border)" />
        {x1 > x0 && (
          <rect
            x={x0}
            y={2}
            width={Math.max(2, x1 - x0)}
            height={6}
            rx={3}
            fill="var(--primary)"
            opacity={0.7}
          />
        )}
        <rect x={x(snapshot) - 1} y={0} width={2} height={10} fill="var(--foreground)" />
      </svg>
      <span className="text-[11px] text-muted-foreground">
        {valueLabel(atlas, "lifetime", lifetime)}
      </span>
    </span>
  );
}
