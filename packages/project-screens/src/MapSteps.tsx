// A first project's steps on the Map: decide, check, fix, connect and share,
// each with what it has left and the place that does it. The host reads which
// are done from the project; the list ends once the project is set up, and a
// person can dismiss it sooner.

import { Check, ChevronRight, X } from "lucide-react";
import { Button, SectionHeading, SimpleTooltip } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { opens, type ScreenNavigation, type ScreenTarget } from "./port";
import type { MapStep } from "./types";

export interface MapStepsProps extends ScreenNavigation {
  steps: MapStep[];
  /** Run the checks over the whole project; absent, the check step leads nowhere. */
  onRun?: () => void;
  running?: boolean;
  /** Hide the steps for this project; absent, they cannot be dismissed. */
  onDismiss?: () => void;
  /** The guide that walks through the same steps, opened outside the app. */
  guideHref?: string;
}

const targets: Partial<Record<MapStep["id"], ScreenTarget>> = {
  decide: { kind: "rules" },
  fix: { kind: "checks" },
  connect: { kind: "connect-assistant" },
  share: { kind: "share" },
};

function stepLabel(id: MapStep["id"]): string {
  switch (id) {
    case "decide":
      return t("Decide what becomes a rule", "a first step on the Map");
    case "check":
      return t("Check the files", "a first step on the Map");
    case "fix":
      return t("Fix what fails", "a first step on the Map");
    case "connect":
      return t("Connect an assistant", "a first step on the Map");
    case "share":
      return t("Share the rules", "a first step on the Map");
  }
}

function stepLeft(step: MapStep): string | undefined {
  if (step.done || !step.count) return undefined;
  if (step.id === "decide") {
    return t("{count} waiting", "items awaiting a decision, beside a first step", {
      count: step.count,
    });
  }
  if (step.id === "fix") {
    return t("{count} failing", "failing findings, beside a first step", { count: step.count });
  }
  return undefined;
}

export function MapSteps({
  steps,
  onRun,
  running,
  onDismiss,
  guideHref,
  navigate,
  canNavigate,
}: MapStepsProps) {
  const nav: ScreenNavigation = { navigate, canNavigate };
  const done = steps.filter((s) => s.done).length;
  const actionOf = (step: MapStep): (() => void) | undefined => {
    if (step.done) return undefined;
    if (step.id === "check") return running ? undefined : onRun;
    const target = targets[step.id];
    return target && opens(nav, target) ? () => navigate(target) : undefined;
  };
  return (
    <section data-slot="map-steps" aria-labelledby="map-steps-heading" className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div id="map-steps-heading" className="flex items-baseline gap-2">
          <SectionHeading>First steps</SectionHeading>
          <span data-slot="map-steps-done" className="text-xs text-muted-foreground tabular-nums">
            {t("{done} of {total}", "first steps done, of all of them", {
              done,
              total: steps.length,
            })}
          </span>
        </div>
        <div className="flex items-center gap-1">
          {guideHref && (
            <Button variant="link" size="sm" className="h-auto px-1 text-xs" asChild>
              <a href={guideHref} data-slot="map-steps-guide">
                Guide
              </a>
            </Button>
          )}
          {onDismiss && (
            <SimpleTooltip content={t("Dismiss")}>
              <Button
                variant="ghost"
                size="icon-xs"
                data-slot="map-steps-dismiss"
                onClick={onDismiss}
                className="text-muted-foreground/60"
                aria-label={t("Dismiss first steps")}
              >
                <X size={14} />
              </Button>
            </SimpleTooltip>
          )}
        </div>
      </div>
      <ol className="divide-y divide-border/60 border-y border-border/60">
        {steps.map((step) => (
          <StepRow
            key={step.id}
            step={step}
            label={stepLabel(step.id)}
            left={stepLeft(step)}
            onAct={actionOf(step)}
          />
        ))}
      </ol>
    </section>
  );
}

function StepRow({
  step,
  label,
  left,
  onAct,
}: {
  step: MapStep;
  label: string;
  left?: string;
  onAct?: () => void;
}) {
  const body = (
    <>
      <span
        aria-hidden
        className={
          step.done
            ? "flex size-4 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
            : "size-4 shrink-0 rounded-full border border-muted-foreground/50"
        }
      >
        {step.done && <Check size={10} strokeWidth={3} />}
      </span>
      <span className={step.done ? "flex-1 text-sm text-muted-foreground" : "flex-1 text-sm"}>
        {label}
        {step.done && <span className="sr-only"> {t("(done)")}</span>}
      </span>
      {left && (
        <span
          data-slot="map-step-left"
          className="shrink-0 text-xs text-muted-foreground tabular-nums"
        >
          {left}
        </span>
      )}
    </>
  );
  return (
    <li data-slot="map-step" data-step={step.id} data-done={step.done || undefined}>
      {onAct ? (
        <button
          type="button"
          data-slot="map-step-act"
          onClick={onAct}
          className="flex min-h-11 w-full items-center gap-3 rounded-md py-1.5 text-left hover:bg-accent/50 focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
        >
          {body}
          <ChevronRight size={14} className="shrink-0 text-muted-foreground" aria-hidden />
        </button>
      ) : (
        <div className="flex min-h-11 items-center gap-3 py-1.5">{body}</div>
      )}
    </li>
  );
}
