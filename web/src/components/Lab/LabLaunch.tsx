import React, { Suspense, useId, useState } from "react";

interface LabLaunchProps {
  children: React.ReactNode;
  label?: string;
  description?: string;
}

class ExperimentBoundary extends React.Component<
  { children: React.ReactNode; onRetry: () => void },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true };
  }

  render(): React.ReactNode {
    if (!this.state.failed) return this.props.children;
    return (
      <div role="alert" className="rounded-lg border p-4">
        <p>The experiment could not load. Check your connection and try again.</p>
        <button type="button" className="button button--secondary" onClick={this.props.onRetry}>
          Try again
        </button>{" "}
        <button
          type="button"
          className="button button--secondary"
          onClick={() => window.location.reload()}
        >
          Reload page
        </button>
      </div>
    );
  }
}

/** Mount browser experiments only after an explicit launch, including their lazy imports. */
export function LabLaunch({
  children,
  label = "Open experiment",
  description = "Open the experiment to load its interface and sample. Where offered, use Run to start processing. Engine and model assets download on first use.",
}: LabLaunchProps): React.ReactElement {
  const [open, setOpen] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const id = useId();
  return (
    <div className="kapi-reference min-w-0">
      <div className="mb-4 rounded-lg border bg-muted/20 p-4">
        <p id={id} className="mb-3 text-sm leading-relaxed text-muted-foreground">
          {description}
        </p>
        <button
          type="button"
          className={`button button--${open ? "secondary" : "primary"}`}
          aria-expanded={open}
          aria-controls={`${id}-experiment`}
          aria-describedby={id}
          onClick={() => setOpen((value) => !value)}
        >
          {open ? "Close experiment" : label}
        </button>
        {open && (
          <span className="ml-3 text-sm text-muted-foreground">
            Closing discards unsaved experiment controls and results.
          </span>
        )}
      </div>
      <div id={`${id}-experiment`}>
        {open && (
          <ExperimentBoundary key={attempt} onRetry={() => setAttempt((value) => value + 1)}>
            <Suspense
              fallback={
                <p role="status" aria-live="polite">
                  Loading the experiment…
                </p>
              }
            >
              {children}
            </Suspense>
          </ExperimentBoundary>
        )}
      </div>
    </div>
  );
}

export default LabLaunch;
