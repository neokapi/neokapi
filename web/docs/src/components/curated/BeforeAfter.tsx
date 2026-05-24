import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import type { InlineSample } from "./seed";
import "./curated.css";

// BeforeAfter — source → transformed result curated view.
//
// It boots the shared kapi runtime, writes the pristine source into the cwd,
// runs a tool/command (pseudo-translate, search-replace, redaction, …) via
// KapiRuntime.run(argv), then reads the output file back out of the in-memory
// volume and shows input vs output side by side. The command line is displayed
// in a Catppuccin strip above the panes so the reader sees exactly what ran.
//
// The output path is whatever the command writes to: pass `outputPath`
// explicitly (matching the command's -o/--output flag). If the command edits
// the input in place, set outputPath to the same name as the source.

export interface BeforeAfterProps {
  /**
   * The pristine source: a bundled fixture name or an inline {name, content}.
   * Written fresh before each run so a re-run always starts from the original.
   */
  sample: string | InlineSample;
  /**
   * The command to run, e.g. "kapi pseudo-translate messages.json -o out.json".
   * The leading "kapi" is optional. Quoted arguments are supported.
   */
  command: string;
  /**
   * The file the command produces, read back for the "after" pane. Defaults to
   * the source file's name (use this for in-place transforms). For commands
   * with an explicit -o flag, set this to that output name.
   */
  outputPath?: string;
  /** Optional label for the source pane. Defaults to "Source". */
  beforeLabel?: string;
  /** Optional label for the result pane. Defaults to "Result". */
  afterLabel?: string;
  /** Optional caption shown below the command strip. */
  caption?: string;
  /**
   * Auto-run on mount (default true). When false, the reader clicks "Run" to
   * execute — useful for heavier transforms.
   */
  autoRun?: boolean;
}

const LazyBeforeAfter = React.lazy(async () => {
  const { useCuratedRuntime } = await import("./useCuratedRuntime");
  const { writeSample, readText, parseCommand } = await import("./seed");

  /** Derive a sensible default output name: the source file's own name. */
  function defaultOutput(sample: string | InlineSample): string {
    return typeof sample === "string" ? sample : sample.name;
  }

  function BeforeAfterInner({
    sample,
    command,
    outputPath,
    beforeLabel = "Source",
    afterLabel = "Result",
    caption,
    autoRun = true,
  }: BeforeAfterProps): React.ReactElement {
    const { runtime, error, cold } = useCuratedRuntime();
    const [before, setBefore] = React.useState<string>("");
    const [after, setAfter] = React.useState<string>("");
    const [running, setRunning] = React.useState<boolean>(false);
    const [runError, setRunError] = React.useState<string>("");
    const [exitCode, setExitCode] = React.useState<number | null>(null);
    const sourceName = typeof sample === "string" ? sample : sample.name;
    const outName = outputPath ?? defaultOutput(sample);

    const run = React.useCallback(async () => {
      if (!runtime) return;
      setRunning(true);
      setRunError("");
      try {
        // Always start from pristine source so re-runs are deterministic.
        const src = writeSample(runtime, sample);
        setBefore(readText(runtime, src));
        const argv = parseCommand(command);
        const code = await runtime.run(argv);
        setExitCode(code);
        setAfter(readText(runtime, outName));
      } catch (e) {
        setRunError(e instanceof Error ? e.message : String(e));
      } finally {
        setRunning(false);
      }
    }, [runtime, sample, command, outName]);

    React.useEffect(() => {
      if (!runtime) return;
      // Show the source immediately; auto-run the transform if requested.
      try {
        const src = writeSample(runtime, sample);
        setBefore(readText(runtime, src));
      } catch (e) {
        setRunError(e instanceof Error ? e.message : String(e));
      }
      if (autoRun) void run();
      // Run once the runtime is ready.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [runtime]);

    const promptLine = command.replace(/^kapi\s+/, "kapi ");

    return (
      <div className="kapi-cur">
        <div className="kapi-cur-cmd">
          <span className="kapi-cur-cmd-prompt" aria-hidden="true">
            $
          </span>
          <span className="kapi-cur-cmd-text">{promptLine}</span>
          <button
            type="button"
            className="kapi-cur-btn kapi-cur-btn--primary"
            onClick={() => void run()}
            disabled={!runtime || running}
          >
            {running ? "Running…" : "Run"}
          </button>
        </div>

        {caption && <p className="kapi-cur-meta">{caption}</p>}

        {(error || runError) && <p className="kapi-cur-error">{error || runError}</p>}

        {!error && !runtime && (
          <div className="kapi-cur-loading">
            <span className="kapi-cur-spinner" aria-hidden="true" />
            <span>{cold ? "Starting kapi for the first time…" : "Getting kapi ready…"}</span>
          </div>
        )}

        {runtime && (
          <div className="kapi-cur-split">
            <div className="kapi-cur-pane">
              <div className="kapi-cur-pane-head">
                <span>{beforeLabel}</span>
                <span className="kapi-cur-pane-name">{sourceName}</span>
              </div>
              <pre className="kapi-cur-code">{before}</pre>
            </div>
            <div className="kapi-cur-pane">
              <div className="kapi-cur-pane-head">
                <span>{afterLabel}</span>
                <span className="kapi-cur-pane-name">{outName}</span>
              </div>
              <pre className="kapi-cur-code">
                {running
                  ? "…"
                  : after ||
                    (exitCode !== null && exitCode !== 0
                      ? `(command exited with code ${exitCode})`
                      : "(no output yet — click Run)")}
              </pre>
            </div>
          </div>
        )}
      </div>
    );
  }

  return { default: BeforeAfterInner };
});

/**
 * BeforeAfter — run a tool/command in the warm runtime and show input vs output
 * side by side. Lazy + client-only.
 */
export default function BeforeAfter(props: BeforeAfterProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<div className="kapi-cur" />}>
      {() => (
        <Suspense fallback={<div className="kapi-cur" />}>
          <LazyBeforeAfter {...props} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}
