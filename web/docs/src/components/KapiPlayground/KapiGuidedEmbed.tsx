import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "./config";
import { EMBED_CONFIGS } from "./embeds";
import type { WalkthroughEmbedConfig } from "./embeds";
import "./guided.css";

// KapiGuidedEmbed — the docs-layer guided playground for a walkthrough.
//
// It reads the GENERATED embed config (web/.../KapiPlayground/embeds/<id>.embed.ts,
// emitted by scripts/walkthrough-gen from the single <id>.scene.yaml source)
// and renders the kit's <KapiEmbed> seeded + scripted from it, beside a
// numbered "guided steps" rail. Each rail item runs exactly one command in the
// warm in-browser session, so the reader steps through the walkthrough the same
// way the recorded video shows it — driven by the same source, so they cannot
// drift.
//
// This component lives in the docs layer (NOT in packages/kapi-playground): it
// composes the kit's public <KapiEmbed> + KapiEmbedHandle API; the core kit is
// untouched.

// The heavy kit (xterm + wasm) loads as a separate async chunk — same split as
// KapiEmbedFullBleed / KapiModalMount, so a docs page renders zero wasm until
// the guided embed first mounts in the browser.
const LazyGuided = React.lazy(async () => {
  const { KapiEmbed, openKapi } = await import("@neokapi/kapi-playground");
  const { Maximize2 } = await import("lucide-react");
  type KapiEmbedHandle = import("@neokapi/kapi-playground").KapiEmbedHandle;

  function Guided({ config }: { config: WalkthroughEmbedConfig }): React.ReactElement {
    const pg = useKapiPlaygroundConfig();
    const embedRef = React.useRef<KapiEmbedHandle>(null);
    const [done, setDone] = React.useState<number>(0);

    const runStep = React.useCallback(
      (index: number) => {
        const step = config.steps[index];
        if (!step) return;
        // Re-seed defensively (gaps only — never clobbers edited files) and run
        // the single command against the warm session.
        embedRef.current?.openWith({
          seed: config.seed,
          files: config.files,
          cmd: step.command,
          autoRun: true,
        });
        setDone((d) => Math.max(d, index + 1));
      },
      [config],
    );

    const runAll = React.useCallback(() => {
      embedRef.current?.openWith({
        seed: config.seed,
        files: config.files,
        cmd: config.steps[0]?.command,
        steps: config.steps.slice(1).map((s) => s.command),
        autoRun: true,
      });
      setDone(config.steps.length);
    }, [config]);

    const reset = React.useCallback(() => {
      embedRef.current?.reset(config.seed);
      setDone(0);
    }, [config]);

    const openFullScreen = React.useCallback(() => {
      openKapi({
        seed: config.seed,
        files: config.files,
        cmd: config.steps[0]?.command,
        steps: config.steps.slice(1).map((s) => s.command),
        autoRun: true,
      });
    }, [config]);

    return (
      <div className="kapi-guided__bleed">
        <div className="kapi-guided__fullscreen-bar">
          <button
            type="button"
            className="kapi-guided__btn kapi-guided__btn--fullscreen"
            onClick={openFullScreen}
            title="Open the full-screen playground and run all steps"
          >
            <Maximize2 size={13} aria-hidden="true" />
            Open full screen
          </button>
        </div>
        <div className="kapi-guided">
          <div className="kapi-guided__rail" aria-label="Guided steps">
            <div className="kapi-guided__rail-head">
              <span className="kapi-guided__rail-title">Run it yourself</span>
              <div className="kapi-guided__rail-actions">
                <button type="button" className="kapi-guided__btn" onClick={runAll}>
                  Run all
                </button>
                <button
                  type="button"
                  className="kapi-guided__btn kapi-guided__btn--ghost"
                  onClick={reset}
                >
                  Reset
                </button>
              </div>
            </div>
            <ol className="kapi-guided__steps">
              {config.steps.map((step, i) => (
                <li
                  key={step.command}
                  className={`kapi-guided__step${i < done ? " kapi-guided__step--done" : ""}`}
                >
                  <button
                    type="button"
                    className="kapi-guided__step-run"
                    onClick={() => runStep(i)}
                    aria-label={`Run step ${i + 1}: ${step.command}`}
                  >
                    <span className="kapi-guided__step-num" aria-hidden="true">
                      {i + 1}
                    </span>
                    <code className="kapi-guided__step-cmd">{step.command}</code>
                  </button>
                  {step.narration && (
                    <p className="kapi-guided__step-narration">{step.narration}</p>
                  )}
                </li>
              ))}
            </ol>
          </div>
          <div className="kapi-guided__terminal">
            <KapiEmbed
              ref={embedRef}
              wasmExecUrl={pg.wasmExecUrl}
              wasmUrl={pg.wasmUrl}
              seed={config.seed}
              files={config.files}
              // Stage the session warm but idle — the first command is at the
              // prompt; the reader drives the steps from the rail.
              cmd={config.steps[0]?.command}
              autoRun={false}
              showToolbar
            />
          </div>
        </div>
      </div>
    );
  }

  function GuidedById({ id }: { id: string }): React.ReactElement {
    const config = EMBED_CONFIGS[id];
    if (!config) {
      return (
        <div className="kapi-guided__notice">
          No embed config for walkthrough <code>{id}</code>. Run{" "}
          <code>node --experimental-strip-types scripts/walkthrough-gen/gen.ts {id}</code>.
        </div>
      );
    }
    return <Guided config={config} />;
  }

  return { default: GuidedById };
});

export interface KapiGuidedEmbedProps {
  /** Walkthrough id — looked up in the generated EMBED_CONFIGS registry. */
  id: string;
}

export default function KapiGuidedEmbed({ id }: KapiGuidedEmbedProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<p>Loading the in-browser terminal…</p>}>
      {() => (
        <Suspense fallback={<p>Loading the in-browser terminal…</p>}>
          <LazyGuided id={id} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}
