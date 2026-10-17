import { useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button } from "@neokapi/ui-primitives";
import { ArrowRight, ChevronRight } from "lucide-react";
import { stateFill } from "../colors";
import { formatCount, formatDay, formatPoints, questionLabel, stateLabel } from "../labels";
import { type Atlas, DERIVED_AXES, preparedStateAt } from "../model/atlas";
import type { EvalContext } from "../model/filter";
import {
  type AtlasQuery,
  QUESTION_IDS,
  type QuestionId,
  applyQuestion,
  withRetained,
} from "../model/query";
import { ABSENT, NEEDS_PERSON } from "../model/state";
import { dayOf } from "../model/time";
import { transitions } from "../model/transitions";

const SHOWN = 6;

export interface SelectionPaneProps {
  atlas: Atlas;
  query: AtlasQuery;
  ctx: EvalContext;
  foreground: readonly number[];
  onQuery: (next: AtlasQuery) => void;
  onSelect: (id: string) => void;
}

function Dot({ state }: { state: string }) {
  const fill = stateFill(state);
  return (
    <span
      aria-hidden
      className="inline-block size-2 shrink-0 rounded-full"
      style={fill === "transparent" ? { border: "1px dashed var(--border)" } : { background: fill }}
    />
  );
}

/** The selection at one instant: how much waits for a person, of how much was recorded. */
function Moment({ day, needs, observed }: { day: string; needs: number; observed: number }) {
  return (
    <div className="rounded-lg border p-3">
      <span className="text-[11px] text-muted-foreground tabular-nums">{formatDay(day)}</span>
      <div className="mt-1 flex items-baseline gap-1">
        <span className="text-xl font-semibold text-warning-ink tabular-nums">
          {formatCount(needs)}
        </span>
        <span className="text-[11px] text-muted-foreground tabular-nums">
          / {formatCount(observed)}
        </span>
      </div>
      <span className="text-[11px] text-muted-foreground">need a person</span>
    </div>
  );
}

/**
 * The supporting pane while no record is open: the selection at the window's
 * start against the snapshot, every state transition in it, what changed in
 * the window, and the questions to start from.
 */
export function SelectionPane({
  atlas,
  query,
  ctx,
  foreground,
  onQuery,
  onSelect,
}: SelectionPaneProps) {
  const [all, setAll] = useState(false);
  const summary = useMemo(() => {
    let startObs = 0;
    let startNeeds = 0;
    let snapObs = 0;
    let snapNeeds = 0;
    for (const i of foreground) {
      const p = atlas.records[i];
      const s0 = preparedStateAt(p, ctx.startMs);
      const s1 = preparedStateAt(p, ctx.snapshotMs);
      if (s0 !== ABSENT) {
        startObs += p.weight;
        if (NEEDS_PERSON.has(s0)) startNeeds += p.weight;
      }
      if (s1 !== ABSENT) {
        snapObs += p.weight;
        if (NEEDS_PERSON.has(s1)) snapNeeds += p.weight;
      }
    }
    const delta = startObs > 0 && snapObs > 0 ? snapNeeds / snapObs - startNeeds / startObs : null;
    return {
      startObs,
      startNeeds,
      snapObs,
      snapNeeds,
      delta,
      t: transitions(atlas, foreground, ctx),
    };
  }, [atlas, foreground, ctx]);

  const changes = useMemo(() => {
    const inSel = new Set(foreground.map((i) => atlas.records[i].record.id));
    return atlas.events
      .filter((e) => {
        if (!e.record || !inSel.has(e.record)) return false;
        const at = query.clock === "effective" ? e.effectiveMs : e.recordedMs;
        return at >= ctx.startMs && at <= ctx.endMs;
      })
      .sort((a, b) =>
        query.clock === "effective" ? b.effectiveMs - a.effectiveMs : b.recordedMs - a.recordedMs,
      );
  }, [atlas, foreground, query.clock, ctx]);

  // One row per record, at its newest change, so a rule recorded, kept and
  // given a window in one week reads once with a count rather than three times.
  const changedRecords = useMemo(() => {
    const rows = new Map<string, { event: (typeof changes)[number]; count: number }>();
    for (const e of changes) {
      if (!e.record) continue;
      const row = rows.get(e.record);
      if (row) row.count += 1;
      else rows.set(e.record, { event: e, count: 1 });
    }
    return [...rows.values()];
  }, [changes]);

  const list = all ? summary.t.transitions : summary.t.transitions.slice(0, SHOWN);

  return (
    <div className="flex flex-col gap-5" data-testid="atlas-selection-pane">
      <div className="grid grid-cols-[1fr_auto_1fr] items-stretch gap-2">
        <Moment day={query.from} needs={summary.startNeeds} observed={summary.startObs} />
        <ArrowRight aria-hidden className="size-4 self-center text-muted-foreground" />
        <Moment day={query.asOf} needs={summary.snapNeeds} observed={summary.snapObs} />
      </div>
      <dl className="flex gap-6">
        <div>
          <dt className="sr-only">Change in share</dt>
          <dd className="text-sm font-semibold tabular-nums">{formatPoints(summary.delta)}</dd>
          <span className="text-[11px] text-muted-foreground">share needing a person</span>
        </div>
        <div>
          <dt className="sr-only">Changed state</dt>
          <dd className="text-sm font-semibold tabular-nums">
            {formatCount(summary.t.changedWeight)}
          </dd>
          <span className="text-[11px] text-muted-foreground">changed state</span>
        </div>
      </dl>

      <section className="flex flex-col gap-1.5" aria-label={t("Transitions", "selection pane")}>
        <h4 className="text-xs font-medium text-muted-foreground uppercase">Transitions</h4>
        {list.length === 0 && <p className="text-sm text-muted-foreground">No records selected</p>}
        <ul className="flex flex-col">
          {list.map((tr) => (
            <li key={`${tr.from}-${tr.to}`}>
              <button
                type="button"
                className="flex w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-left text-sm hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                onClick={() =>
                  onQuery(
                    withRetained(query, [
                      { axis: DERIVED_AXES.statusStart, value: tr.from },
                      { axis: DERIVED_AXES.statusSnapshot, value: tr.to },
                    ]),
                  )
                }
              >
                <Dot state={tr.from} />
                <span className="truncate">{stateLabel(tr.from)}</span>
                <ArrowRight aria-hidden className="size-3 shrink-0 text-muted-foreground" />
                <Dot state={tr.to} />
                <span className="truncate">{stateLabel(tr.to)}</span>
                <span className="ml-auto pl-2 tabular-nums">{formatCount(tr.weight)}</span>
              </button>
            </li>
          ))}
        </ul>
        {summary.t.transitions.length > SHOWN && (
          <Button
            variant="ghost"
            size="xs"
            className="self-start"
            onClick={() => setAll((v) => !v)}
          >
            {all
              ? t("Show fewer", "transitions list")
              : t("Show all {count}", "transitions list", { count: summary.t.transitions.length })}
          </Button>
        )}
      </section>

      <section
        className="flex flex-col gap-1.5"
        aria-label={t("Changes in this window", "selection pane")}
      >
        <h4 className="flex items-baseline gap-2 text-xs font-medium text-muted-foreground uppercase">
          Changes in this window
          <span className="font-normal tabular-nums normal-case">
            {formatCount(changes.length)}
          </span>
        </h4>
        <ul className="flex flex-col">
          {changedRecords.slice(0, 5).map(({ event: e, count }) => {
            const rec = e.record ? atlas.byId.get(e.record) : undefined;
            return (
              <li key={e.index}>
                <button
                  type="button"
                  className="flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm hover:bg-muted"
                  onClick={() => e.record && onSelect(e.record)}
                >
                  <span className="min-w-0 flex-1 truncate">{rec?.record.label ?? e.record}</span>
                  {count > 1 && (
                    <span
                      className="shrink-0 rounded-sm bg-muted px-1 text-[11px] text-muted-foreground tabular-nums"
                      title={t("{count} changes to this record in the window", "selection pane", {
                        count: formatCount(count),
                      })}
                    >
                      {formatCount(count)}
                    </span>
                  )}
                  <span className="shrink-0 text-[11px] text-muted-foreground tabular-nums">
                    {formatDay(dayOf(query.clock === "effective" ? e.effectiveMs : e.recordedMs))}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      </section>

      <section className="flex flex-col gap-1.5" aria-label={t("Try a question", "selection pane")}>
        <h4 className="text-xs font-medium text-muted-foreground uppercase">Try a question</h4>
        <ul className="flex flex-col">
          {QUESTION_IDS.map((id: QuestionId) => (
            <li key={id}>
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-sm hover:bg-muted"
                onClick={() => onQuery(applyQuestion(query, atlas, id))}
              >
                <span className="flex-1">{questionLabel(id)}</span>
                <ChevronRight aria-hidden className="size-3.5 text-muted-foreground" />
              </button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
