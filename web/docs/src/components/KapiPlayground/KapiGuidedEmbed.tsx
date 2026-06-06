import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { Play } from "lucide-react";
import { useKapiPlaygroundConfig } from "./config";
import { EMBED_CONFIGS } from "./embeds";
import type { WalkthroughEmbedConfig } from "./embeds";
import "./guided.css";

// KapiGuidedEmbed — the docs-layer guided playground for a walkthrough.
//
// It reads the GENERATED embed config (web/.../KapiPlayground/embeds/<id>.embed.ts,
// emitted by scripts/walkthrough-gen from the single <id>.scene.yaml source).
//
// In the narrow docs content column the inline 3-panel surface (rail + terminal
// + files) was cramped and wide CLI commands wrapped badly. So in docs this
// component now renders a COMPACT LAUNCHER: a small card listing the walkthrough
// steps with a "Run it now" button styled like the homepage hero CTA. Clicking
// opens the FULL guided experience (numbered rail with per-step run + narration
// ALONGSIDE the terminal + files panel) in a roomy MODAL, where wide commands
// have room to breathe.
//
// Lazy/SSR-clean contract: the launcher pulls ZERO wasm until the modal opens.
// The heavy kit (xterm + wasm + the rail/embed) is a single async chunk that is
// only imported when the reader clicks "Run it now" — exactly like the homepage
// openKapi CTA loads nothing until click.

// The heavy guided modal — xterm + wasm + the rail/embed — loads as a separate
// async chunk. It is only imported the first time the reader opens the modal.
const LazyGuidedModal = React.lazy(async () => {
  const { KapiEmbed } = await import("@neokapi/kapi-playground");
  const { X, RotateCcw } = await import("lucide-react");
  type KapiEmbedHandle = import("@neokapi/kapi-playground").KapiEmbedHandle;

  // Selector for focusable descendants — used by the focus trap. Mirrors the
  // kit's KapiModal so behaviour is identical.
  const FOCUSABLE =
    'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

  function GuidedModal({
    config,
    onClose,
  }: {
    config: WalkthroughEmbedConfig;
    onClose: () => void;
  }): React.ReactElement {
    const pg = useKapiPlaygroundConfig();
    const embedRef = React.useRef<KapiEmbedHandle>(null);
    const dialogRef = React.useRef<HTMLDivElement>(null);
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

    // Esc to close + focus trap while open. Mirrors the kit's KapiModal: capture
    // phase, because xterm's textarea handles (and stops) some keys, and a
    // file-preview dialog over the modal should swallow Esc first.
    React.useEffect(() => {
      const dialog = dialogRef.current;
      const onKeyDown = (e: KeyboardEvent) => {
        if (e.key === "Escape") {
          if (document.querySelector(".kapi-pg-overlay")) return;
          e.preventDefault();
          e.stopPropagation();
          onClose();
          return;
        }
        if (e.key !== "Tab" || !dialog) return;
        const items = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
          (el) => el.offsetParent !== null,
        );
        if (items.length === 0) return;
        const first = items[0];
        const last = items[items.length - 1];
        const active = document.activeElement as HTMLElement | null;
        if (e.shiftKey && active === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && active === last) {
          e.preventDefault();
          first.focus();
        }
      };
      document.addEventListener("keydown", onKeyDown, true);
      const prevOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      const toFocus = dialog?.querySelector<HTMLElement>("[data-kapi-initial-focus]");
      toFocus?.focus();
      return () => {
        document.removeEventListener("keydown", onKeyDown, true);
        document.body.style.overflow = prevOverflow;
      };
    }, [onClose]);

    return (
      <div className="kapi-pg-modal-overlay" onClick={onClose}>
        <div
          ref={dialogRef}
          className="kapi-pg-modal kapi-guided-modal"
          role="dialog"
          aria-modal="true"
          aria-label={`Guided walkthrough: ${config.id}`}
          onClick={(e) => e.stopPropagation()}
        >
          <div className="kapi-pg-modal-bar">
            <span className="kapi-pg-modal-title">Run it yourself</span>
            <span className="kapi-pg-modal-hint">Press Esc to close</span>
            <button
              type="button"
              className="kapi-pg-btn kapi-pg-btn--sm"
              onClick={runAll}
              title="Run every step in sequence"
            >
              <Play size={13} aria-hidden="true" />
              <span>Run all</span>
            </button>
            <button
              type="button"
              className="kapi-pg-btn kapi-pg-btn--sm"
              onClick={reset}
              title="Reset the in-memory project to its seed files"
            >
              <RotateCcw size={14} aria-hidden="true" />
              <span>Reset</span>
            </button>
            <button
              type="button"
              className="kapi-pg-icon-btn"
              onClick={onClose}
              aria-label="Close walkthrough"
              title="Close"
              data-kapi-initial-focus
            >
              <X size={18} aria-hidden="true" />
            </button>
          </div>
          <div className="kapi-pg-modal-body kapi-guided-modal__body">
            <div className="kapi-guided">
              <div className="kapi-guided__rail" aria-label="Guided steps">
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
                  // Stage the session warm but idle — the first command is at
                  // the prompt; the reader drives the steps from the rail.
                  cmd={config.steps[0]?.command}
                  autoRun={false}
                  showToolbar={false}
                  fill
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return { default: GuidedModal };
});

/**
 * The compact, SSR-clean launcher rendered inline in the docs column. Lists the
 * walkthrough steps as static text and offers a "Run it now" button. It imports
 * NO wasm — the heavy guided modal is lazy-loaded only after the first open.
 */
function GuidedLauncher({ config }: { config: WalkthroughEmbedConfig }): React.ReactElement {
  const [open, setOpen] = React.useState(false);
  // Once opened we keep the modal mounted (hidden via its own unmount on close)
  // so reopening is instant after the chunk + wasm are warm. We track "has ever
  // opened" so the lazy chunk is only requested after the first click.
  const [everOpened, setEverOpened] = React.useState(false);

  const openModal = React.useCallback(() => {
    setEverOpened(true);
    setOpen(true);
  }, []);
  const close = React.useCallback(() => setOpen(false), []);

  const stepCount = config.steps.length;

  return (
    <div className="kapi-guided-launcher">
      <div className="kapi-guided-launcher__head">
        <div>
          <span className="kapi-guided-launcher__eyebrow">Interactive walkthrough</span>
          <p className="kapi-guided-launcher__summary">
            {stepCount} step{stepCount === 1 ? "" : "s"}, in an in-browser terminal — no install, no
            server.
          </p>
        </div>
        <button
          type="button"
          className="kapi-guided-launcher__run"
          onClick={openModal}
          aria-haspopup="dialog"
          aria-expanded={open}
        >
          <Play size={15} aria-hidden="true" fill="currentColor" />
          Run it now
        </button>
      </div>
      <ol className="kapi-guided-launcher__steps">
        {config.steps.map((step, i) => (
          <li key={step.command} className="kapi-guided-launcher__step">
            <span className="kapi-guided-launcher__step-num" aria-hidden="true">
              {i + 1}
            </span>
            <code className="kapi-guided-launcher__step-cmd">{step.command}</code>
          </li>
        ))}
      </ol>
      {everOpened && open && (
        <Suspense fallback={null}>
          <LazyGuidedModal config={config} onClose={close} />
        </Suspense>
      )}
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
  return <GuidedLauncher config={config} />;
}

export interface KapiGuidedEmbedProps {
  /** Walkthrough id — looked up in the generated EMBED_CONFIGS registry. */
  id: string;
}

export default function KapiGuidedEmbed({ id }: KapiGuidedEmbedProps): React.ReactElement {
  // The launcher itself is light, but it must stay client-only because the modal
  // it lazy-loads (and the wasm config hook) are browser-only.
  return <BrowserOnly fallback={<p>Loading the walkthrough…</p>}>{() => <GuidedById id={id} />}</BrowserOnly>;
}
