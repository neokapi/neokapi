import React, { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { X, RotateCcw } from "lucide-react";
import KapiEmbed from "./KapiEmbed";
import type { KapiEmbedHandle } from "./KapiEmbed";
import { registerKapiModal } from "./store";
import type { OpenKapiOptions } from "./store";
import { useKapiConfig } from "./provider";

// Selector for focusable descendants — used by the focus trap.
const FOCUSABLE =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

/**
 * The single shared modal. Mount exactly one instance near the app root. It
 * listens for `openKapi(...)` calls, opens near-fullscreen, boots the WASM
 * runtime on first open, and keeps it warm afterwards.
 *
 * Session model (see #659): on a *new* open while a session already exists, the
 * session is preserved — new seed fixtures are ensured to exist and the new
 * command runs against the warm volume. A Reset button wipes the cwd back to a
 * fresh seed.
 */
export default function KapiModal(): React.ReactElement | null {
  const { wasmExecUrl, wasmUrl } = useKapiConfig();
  const [open, setOpen] = useState(false);
  // Whether the heavy embed has ever mounted. Once true we keep it mounted
  // (display:none when closed) so the runtime stays warm and terminal history
  // survives across opens.
  const [mounted, setMounted] = useState(false);
  // The request used for the very first mount of the embed.
  const [initialReq, setInitialReq] = useState<OpenKapiOptions>({});
  // Remember the most recent seed so Reset restores the right fixtures.
  const lastSeed = useRef<string[] | undefined>(undefined);

  const embedRef = useRef<KapiEmbedHandle>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const lastFocused = useRef<HTMLElement | null>(null);

  const close = useCallback(() => {
    setOpen(false);
    // Restore focus to the trigger.
    lastFocused.current?.focus?.();
  }, []);

  useEffect(() => {
    return registerKapiModal((opts: OpenKapiOptions) => {
      lastFocused.current = (document.activeElement as HTMLElement) ?? null;
      lastSeed.current = opts.seed;
      setOpen(true);
      setMounted((wasMounted) => {
        if (!wasMounted) {
          // First open: mount the embed with this request; it boots + runs.
          setInitialReq(opts);
          return true;
        }
        // Already warm: drive the existing session with the new request.
        // Defer to the next tick so the embed is on-screen and laid out.
        window.setTimeout(() => embedRef.current?.openWith(opts), 0);
        return true;
      });
    });
  }, []);

  // Esc to close + focus trap while open.
  useEffect(() => {
    if (!open) return;
    const dialog = dialogRef.current;

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        // If a file-preview dialog is open over the modal, let it handle Esc
        // (close the innermost layer first).
        if (document.querySelector(".kapi-pg-overlay")) return;
        e.preventDefault();
        e.stopPropagation();
        close();
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

    // Capture phase: xterm's textarea handles (and stops propagation of) some
    // keys, so a bubble-phase listener would miss Escape/Tab when the terminal
    // has focus. Capturing lets the modal intercept them first.
    document.addEventListener("keydown", onKeyDown, true);
    // Prevent the page behind from scrolling.
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    // Move focus into the dialog (the close button) for keyboard users.
    const toFocus = dialog?.querySelector<HTMLElement>("[data-kapi-initial-focus]");
    toFocus?.focus();

    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      document.body.style.overflow = prevOverflow;
    };
  }, [open, close]);

  if (!mounted) return null;

  // Portal to <body> so the overlay escapes the navbar's stacking context and
  // any ancestor transforms — it must paint above everything, including the
  // sticky site header.
  return createPortal(
    <div
      className={`kapi-pg-modal-overlay${open ? "" : " kapi-pg-modal-overlay--hidden"}`}
      onClick={close}
      aria-hidden={open ? undefined : true}
    >
      <div
        ref={dialogRef}
        className="kapi-pg-modal"
        role="dialog"
        aria-modal="true"
        aria-label="kapi playground"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="kapi-pg-modal-bar">
          <span className="kapi-pg-modal-title">kapi playground</span>
          <span className="kapi-pg-modal-hint">in-browser, no server · Esc to close</span>
          <button
            type="button"
            className="kapi-pg-btn kapi-pg-btn--sm"
            onClick={() => embedRef.current?.reset(lastSeed.current)}
            title="Reset the in-memory project to its seed files"
          >
            <RotateCcw size={14} aria-hidden="true" />
            <span>Reset</span>
          </button>
          <button
            type="button"
            className="kapi-pg-icon-btn"
            onClick={close}
            aria-label="Close playground"
            title="Close"
            data-kapi-initial-focus
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>
        <div className="kapi-pg-modal-body">
          <KapiEmbed
            ref={embedRef}
            wasmExecUrl={wasmExecUrl}
            wasmUrl={wasmUrl}
            seed={initialReq.seed}
            cmd={initialReq.cmd}
            steps={initialReq.steps}
            autoRun={initialReq.autoRun}
            showToolbar={false}
            fill
          />
        </div>
      </div>
    </div>,
    document.body,
  );
}
