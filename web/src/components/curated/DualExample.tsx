import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import type { InlineSample } from "./seed";
import BlockPreview from "./BlockPreview";
import BeforeAfter from "./BeforeAfter";
import "./curated.css";

// DualExample — the old playground's "file in → parsed blocks out" feel,
// generalized: a CLI command (+ its captured terminal output) shown beside the
// curated result the framework produced (a BlockPreview or a BeforeAfter).
//
// The reader sees BOTH the command ergonomics AND what the framework did. Two
// layouts:
//   • "tabs"  — a tabbed view: [Command] | [Result]   (compact, good in prose)
//   • "split" — side by side terminal ⇄ curated result (default, the dual feel)
//
// The terminal pane is not a live xterm (that would be a second session
// competing for the singleton runtime). Instead it captures the command's real
// stdout/stderr by pointing the runtime's sinks at a buffer around a run() — so
// it shows the actual framework output, in the Catppuccin palette, without a
// second wasm terminal. A "Open full terminal" affordance hands off to the kit's
// shared modal (openKapi) for readers who want the interactive xterm.

export type DualResult =
  | {
      /** A "kapi reader" block view of the sample. */
      kind: "blocks";
      sample: string | InlineSample;
      title?: string;
      caption?: string;
    }
  | {
      /** A before/after transform view. */
      kind: "before-after";
      sample: string | InlineSample;
      command: string;
      outputPath?: string;
      beforeLabel?: string;
      afterLabel?: string;
      caption?: string;
    };

export interface DualExampleProps {
  /**
   * The CLI command shown + run in the terminal pane (its stdout/stderr is
   * captured for display). Leading "kapi" optional.
   */
  command: string;
  /**
   * Bundled fixtures (or inline samples) the command needs seeded into the cwd
   * before it runs. For a "before-after" result the result's own sample is also
   * seeded automatically.
   */
  seed?: (string | InlineSample)[];
  /** The curated result rendered beside the terminal. */
  result: DualResult;
  /** Layout: "split" (side by side, default) or "tabs". */
  layout?: "split" | "tabs";
  /** Optional caption above the whole example. */
  caption?: string;
}

const LazyDual = React.lazy(async () => {
  const { useCuratedRuntime } = await import("./useCuratedRuntime");
  const { ensureSample, parseCommand } = await import("./seed");
  const { openKapi } = await import("@neokapi/kapi-playground/store");
  const { Maximize2 } = await import("lucide-react");

  // The terminal pane: seed, run the command capturing its output, render the
  // captured stdout/stderr in a Catppuccin strip. Reuses the singleton runtime.
  function CommandPane({
    command,
    seed,
  }: {
    command: string;
    seed?: (string | InlineSample)[];
  }): React.ReactElement {
    const { runtime, error, cold, armed, arm } = useCuratedRuntime();
    const [output, setOutput] = React.useState<string>("");
    const [running, setRunning] = React.useState<boolean>(false);
    const [ran, setRan] = React.useState<boolean>(false);

    const run = React.useCallback(async () => {
      if (!runtime) return;
      setRunning(true);
      let buf = "";
      runtime.setSinks(
        (s) => {
          buf += s;
        },
        (s) => {
          buf += s;
        },
      );
      try {
        for (const s of seed ?? []) ensureSample(runtime, s);
        await runtime.run(parseCommand(command));
      } catch (e) {
        buf += `\n${e instanceof Error ? e.message : String(e)}`;
      } finally {
        // Detach our capture sinks so a later live terminal isn't fed our buffer.
        runtime.setSinks(
          () => {},
          () => {},
        );
        setOutput(buf);
        setRunning(false);
        setRan(true);
      }
      // seed/command are stable per usage.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [runtime]);

    React.useEffect(() => {
      if (runtime && !ran) void run();
      // Auto-run once the runtime is ready.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [runtime]);

    const openFull = React.useCallback(() => {
      const fixtureSeed = (seed ?? []).filter((s): s is string => typeof s === "string");
      const inlineFiles = (seed ?? [])
        .filter((s): s is InlineSample => typeof s !== "string")
        .map((s) => ({ path: s.name, content: s.content }));
      openKapi({ cmd: command, seed: fixtureSeed, files: inlineFiles, autoRun: true });
    }, [command, seed]);

    return (
      <div className="kapi-cur-term-block">
        <div className="kapi-cur-cmd">
          <span className="kapi-cur-cmd-prompt" aria-hidden="true">
            $
          </span>
          <span className="kapi-cur-cmd-text">{command.replace(/^kapi\s+/, "kapi ")}</span>
          {!armed && (
            <button type="button" className="kapi-cur-btn kapi-cur-btn--primary" onClick={arm}>
              ▶ Run
            </button>
          )}
          <button
            type="button"
            className="kapi-cur-btn"
            onClick={openFull}
            title="Open the full interactive terminal"
          >
            <Maximize2 size={13} aria-hidden="true" />
            Terminal
          </button>
        </div>
        {error && <p className="kapi-cur-error">{error}</p>}
        {!error && !armed && (
          <p className="kapi-cur-meta">Press Run to execute this command in the real engine.</p>
        )}
        {!error && armed && !runtime && (
          <div className="kapi-cur-loading">
            <span className="kapi-cur-spinner" aria-hidden="true" />
            <span>{cold ? "Starting kapi for the first time…" : "Getting kapi ready…"}</span>
          </div>
        )}
        {runtime && (
          <pre className="kapi-cur-code kapi-cur-stdout">
            {running ? "…" : output || "(no output)"}
          </pre>
        )}
      </div>
    );
  }

  function ResultView({ result }: { result: DualResult }): React.ReactElement {
    if (result.kind === "blocks") {
      return <BlockPreview sample={result.sample} title={result.title} caption={result.caption} />;
    }
    return (
      <BeforeAfter
        sample={result.sample}
        command={result.command}
        outputPath={result.outputPath}
        beforeLabel={result.beforeLabel}
        afterLabel={result.afterLabel}
        caption={result.caption}
      />
    );
  }

  function DualInner({
    command,
    seed,
    result,
    layout = "split",
    caption,
  }: DualExampleProps): React.ReactElement {
    const [tab, setTab] = React.useState<"command" | "result">("command");

    const cmdPane = <CommandPane command={command} seed={seed} />;
    const resultPane = <ResultView result={result} />;

    if (layout === "tabs") {
      return (
        <div className="kapi-cur">
          {caption && <p className="kapi-cur-meta">{caption}</p>}
          <div className="kapi-cur-tabs" role="tablist" aria-label="CLI command and curated result">
            <button
              type="button"
              role="tab"
              aria-selected={tab === "command"}
              className="kapi-cur-tab"
              onClick={() => setTab("command")}
            >
              CLI command
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={tab === "result"}
              className="kapi-cur-tab"
              onClick={() => setTab("result")}
            >
              What kapi produced
            </button>
          </div>
          <div role="tabpanel" hidden={tab !== "command"}>
            {cmdPane}
          </div>
          <div role="tabpanel" hidden={tab !== "result"}>
            {resultPane}
          </div>
        </div>
      );
    }

    return (
      <div className="kapi-cur">
        {caption && <p className="kapi-cur-meta">{caption}</p>}
        <div className="kapi-cur-dual-grid">
          <div>{cmdPane}</div>
          <div>{resultPane}</div>
        </div>
      </div>
    );
  }

  return { default: DualInner };
});

/**
 * DualExample — a CLI command (+ captured terminal output) beside the curated
 * result the framework produced. Lazy + client-only.
 */
export default function DualExample(props: DualExampleProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<div className="kapi-cur" />}>
      {() => (
        <Suspense fallback={<div className="kapi-cur" />}>
          <LazyDual {...props} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}
