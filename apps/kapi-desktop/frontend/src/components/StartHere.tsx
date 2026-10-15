// Start here: the three steps of the learn loop, on the home until the person
// dismisses them. Settings brings them back.

import { X } from "lucide-react";
import { Button, SectionHeading, SimpleTooltip } from "@neokapi/ui-primitives";

/** The desktop overview in the kapi docs; external links open in the browser. */
export const DESKTOP_OVERVIEW_URL = "https://neokapi.github.io/kapi/desktop/overview";

export interface StartHereProps {
  /** Hide the steps. */
  onDismiss: () => void;
  /** Create a sample project by name. */
  onCreateSampleProject: (name: string) => void;
  /** Open the most recent project tab on its Assistant section. */
  onConnectAssistant: () => void;
  /** A project tab is open, so there is a project to connect. */
  hasOpenProject: boolean;
}

export function StartHere({
  onDismiss,
  onCreateSampleProject,
  onConnectAssistant,
  hasOpenProject,
}: StartHereProps) {
  return (
    <section data-slot="start-here" aria-labelledby="start-here-heading">
      <div className="flex items-center justify-between gap-2">
        <div id="start-here-heading">
          <SectionHeading>Start here</SectionHeading>
        </div>
        <SimpleTooltip content="Dismiss">
          <Button
            variant="ghost"
            size="icon-xs"
            data-slot="start-here-dismiss"
            onClick={onDismiss}
            className="text-muted-foreground/60"
            aria-label="Dismiss Start here"
          >
            <X size={14} />
          </Button>
        </SimpleTooltip>
      </div>
      <ol className="mt-2 divide-y divide-border/60">
        <Step n={1}>
          <p>
            Open a project, or try the sample. The sample arrives with an assistant&apos;s notes to
            review.
          </p>
          <Button
            size="sm"
            variant="outline"
            data-testid="sample-kapimart"
            className="shrink-0"
            onClick={() => onCreateSampleProject("kapimart")}
          >
            Try the sample
          </Button>
        </Step>
        <Step n={2}>
          <p>
            Connect an assistant. kapi writes the project&apos;s rules where it reads them, and the
            assistant records what it notices.
          </p>
          {hasOpenProject ? (
            <Button
              size="sm"
              variant="outline"
              data-slot="start-here-connect"
              className="shrink-0"
              onClick={onConnectAssistant}
            >
              Connect an assistant
            </Button>
          ) : (
            <SimpleTooltip content="Open a project first">
              {/* A disabled button takes no focus, so the wrapper carries it and the tooltip. */}
              <span className="inline-flex shrink-0" tabIndex={0}>
                <Button size="sm" variant="outline" data-slot="start-here-connect" disabled>
                  Connect an assistant
                </Button>
              </span>
            </SimpleTooltip>
          )}
        </Step>
        <Step n={3}>
          <p>Come back. What it noticed appears under What&apos;s new; keep what is right.</p>
          <Button size="sm" variant="outline" className="shrink-0" asChild>
            <a href={DESKTOP_OVERVIEW_URL} data-slot="start-here-how">
              How it works
            </a>
          </Button>
        </Step>
      </ol>
    </section>
  );
}

function Step({ n, children }: { n: number; children: React.ReactNode }) {
  return (
    <li className="flex items-start gap-3 py-2.5 text-sm">
      <span aria-hidden className="w-3 shrink-0 text-[11px] leading-5 text-muted-foreground">
        {n}
      </span>
      <div className="flex min-w-0 flex-1 items-start justify-between gap-4">{children}</div>
    </li>
  );
}
