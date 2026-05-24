import React from "react";
import { Play } from "lucide-react";
import { openKapi } from "./store";
import "./styles.css";

export interface RunnableSnippetProps {
  /** The command to show and run, e.g. "kapi word-count messages.json". */
  cmd: string;
  /** Fixture names seeded into the session before the command runs. */
  seed?: string[];
  /**
   * When true, the command is placed at the prompt (not auto-run) so the
   * reader presses Enter themselves. Defaults to false (auto-run on open).
   */
  editable?: boolean;
  /**
   * Optional static expected-output sample shown inline beneath the command.
   * Forward-looking for W7's CI-captured output; purely presentational here.
   */
  expected?: string;
}

/**
 * An inline, SSR-clean command block with a `▸ Run` button. Clicking Run opens
 * the shared KapiModal via `openKapi(...)`. This component imports NO xterm or
 * wasm code — the heavy runtime lives entirely in the modal, which is
 * code-split, so a docs page that merely renders snippets fetches zero wasm.
 */
export default function RunnableSnippet({
  cmd,
  seed,
  editable = false,
  expected,
}: RunnableSnippetProps): React.ReactElement {
  return (
    <div className="kapi-pg-snippet">
      <div className="kapi-pg-snippet-row">
        <code className="kapi-pg-snippet-code">
          <span className="kapi-pg-snippet-prompt" aria-hidden="true">
            $
          </span>{" "}
          {cmd}
        </code>
        <button
          type="button"
          className="kapi-pg-run-btn"
          onClick={() => openKapi({ cmd, seed, autoRun: !editable })}
          aria-label={`Run: ${cmd}`}
        >
          <Play size={13} aria-hidden="true" fill="currentColor" />
          <span>Run</span>
        </button>
      </div>
      {expected && <pre className="kapi-pg-snippet-expected">{expected}</pre>}
    </div>
  );
}
