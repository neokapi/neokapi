// Get started: three guided tours on the home, until the person hides the
// card. Help and App Settings bring it back. Each step starts its tour with a
// click; nothing here starts one on its own.

import { Check } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button, SectionHeading, SimpleTooltip, useOptionalGuide } from "@neokapi/ui-primitives";
import { GUIDE_IDS } from "../guide/guides";

export interface StartHereProps {
  /** Hide the card. */
  onDismiss: () => void;
  /** Create a sample project by name; resolves once its tab has opened. */
  onCreateSampleProject: (name: string) => void | Promise<void>;
  /** A project tab is open, so the project tours have a project to show. */
  hasOpenProject: boolean;
}

export function StartHere({ onDismiss, onCreateSampleProject, hasOpenProject }: StartHereProps) {
  const guide = useOptionalGuide();
  const done = (id: string) => guide?.completed.has(id) ?? false;
  const steps = [GUIDE_IDS.project, GUIDE_IDS.decide, GUIDE_IDS.assistant];
  const doneCount = steps.filter(done).length;

  // One click opens the sample and starts its tour; the tour's Skip ends it.
  const trySample = async () => {
    await onCreateSampleProject("kapimart");
    guide?.start(GUIDE_IDS.project);
  };
  const startTour = (id: string) => () => guide?.start(id);

  return (
    <section data-slot="start-here" aria-labelledby="start-here-heading">
      <div className="flex items-center justify-between gap-2">
        <div id="start-here-heading" className="flex items-baseline gap-2">
          <SectionHeading>Get started</SectionHeading>
          <span data-slot="start-here-done" className="text-xs text-muted-foreground tabular-nums">
            {t("{done} of {total}", "Get started tours done, of all of them", {
              done: doneCount,
              total: steps.length,
            })}
          </span>
        </div>
        <Button
          variant="ghost"
          size="sm"
          data-slot="start-here-dismiss"
          onClick={onDismiss}
          className="h-auto px-1 text-xs text-muted-foreground"
          aria-label={t("Hide Get started")}
        >
          Hide
        </Button>
      </div>
      <ol className="mt-2 divide-y divide-border/60">
        <Step n={1} done={done(GUIDE_IDS.project)}>
          <p>Try the sample project and take a tour of its Overview.</p>
          <Button
            size="sm"
            variant="outline"
            data-testid="sample-kapimart"
            className="shrink-0"
            onClick={() => void trySample()}
          >
            Try the sample
          </Button>
        </Step>
        <Step n={2} done={done(GUIDE_IDS.decide)}>
          <p>Decide which of an assistant&apos;s suggestions become rules.</p>
          <TourButton
            slot="start-here-decide"
            enabled={hasOpenProject && !!guide}
            onClick={startTour(GUIDE_IDS.decide)}
          />
        </Step>
        <Step n={3} done={done(GUIDE_IDS.assistant)}>
          <p>Connect an assistant, so it writes by the project&apos;s rules.</p>
          <TourButton
            slot="start-here-connect"
            enabled={hasOpenProject && !!guide}
            onClick={startTour(GUIDE_IDS.assistant)}
          />
        </Step>
      </ol>
    </section>
  );
}

function TourButton({
  slot,
  enabled,
  onClick,
}: {
  slot: string;
  enabled: boolean;
  onClick: () => void;
}) {
  if (enabled) {
    return (
      <Button size="sm" variant="outline" data-slot={slot} className="shrink-0" onClick={onClick}>
        Take the tour
      </Button>
    );
  }
  return (
    <SimpleTooltip content={t("Open a project first")}>
      {/* A disabled button takes no focus, so the wrapper carries it and the tooltip. */}
      <span className="inline-flex shrink-0" tabIndex={0}>
        <Button size="sm" variant="outline" data-slot={slot} disabled>
          Take the tour
        </Button>
      </span>
    </SimpleTooltip>
  );
}

function Step({ n, done, children }: { n: number; done: boolean; children: React.ReactNode }) {
  return (
    <li
      data-slot="start-here-step"
      data-done={done || undefined}
      className="flex items-start gap-3 py-2.5 text-sm"
    >
      <span
        aria-hidden
        className="flex w-3 shrink-0 justify-center text-[11px] leading-5 text-muted-foreground"
      >
        {done ? <Check size={12} strokeWidth={3} className="mt-1 text-primary" /> : n}
      </span>
      {done && <span className="sr-only">{t("(done)")}</span>}
      <div className="flex min-w-0 flex-1 items-start justify-between gap-4">{children}</div>
    </li>
  );
}
