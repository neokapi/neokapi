import React, { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { X, RotateCcw, Share2, Check } from "lucide-react";
import KapiEmbed from "./KapiEmbed";
import type { KapiEmbedHandle } from "./KapiEmbed";
import { registerKapiModal, serializeSession, deserializeSession } from "./store";
import type { OpenKapiOptions } from "./store";
import { useKapiConfig } from "./provider";

// URL query param carrying a shareable session token (see store.ts).
const SHARE_PARAM = "s";

/**
 * If the current URL carries a `?s=` session token, decode it into an
 * openKapi request that restores the files and replays the commands. Returns
 * null when there is no token or it is malformed. Browser-only.
 */
function requestFromUrl(): OpenKapiOptions | null {
  if (typeof window === "undefined") return null;
  const token = new URLSearchParams(window.location.search).get(SHARE_PARAM);
  if (!token) return null;
  const session = deserializeSession(token);
  if (!session) return null;
  // Restore files first (no seed — the shared files ARE the state), then replay
  // the recorded commands as steps.
  return { files: session.files, steps: session.steps, autoRun: true };
}

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
  // Transient "Copied" / "Shared" feedback on the Share button.
  const [shared, setShared] = useState(false);
  const shareTimer = useRef<number | undefined>(undefined);

  const embedRef = useRef<KapiEmbedHandle>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const lastFocused = useRef<HTMLElement | null>(null);

  const close = useCallback(() => {
    setOpen(false);
    // Restore focus to the trigger.
    lastFocused.current?.focus?.();
  }, []);

  // Capture the current session, encode it into a `?s=` URL, write it to the
  // address bar, and copy it to the clipboard. Falls back gracefully when the
  // clipboard API is unavailable (still updates the URL, which is shareable).
  const share = useCallback(async () => {
    const session = embedRef.current?.snapshot();
    if (!session) return;
    const token = serializeSession(session);
    const url = new URL(window.location.href);
    url.searchParams.set(SHARE_PARAM, token);
    const href = url.toString();
    try {
      window.history.replaceState(null, "", href);
    } catch {
      /* ignore — sandboxed iframe */
    }
    try {
      await navigator.clipboard?.writeText(href);
    } catch {
      /* clipboard blocked — the address bar still holds the link */
    }
    setShared(true);
    window.clearTimeout(shareTimer.current);
    shareTimer.current = window.setTimeout(() => setShared(false), 1800);
  }, []);

  useEffect(() => () => window.clearTimeout(shareTimer.current), []);

  useEffect(() => {
    const unsubscribe = registerKapiModal((opts: OpenKapiOptions) => {
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

    // Deep-link: if the page was opened with a `?s=` session token, restore it
    // immediately (mounts the embed and replays the shared commands).
    const fromUrl = requestFromUrl();
    if (fromUrl) {
      lastSeed.current = fromUrl.seed;
      setOpen(true);
      setInitialReq(fromUrl);
      setMounted(true);
    }

    return unsubscribe;
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
          <span className="kapi-pg-modal-hint">Press Esc to close</span>
          <button
            type="button"
            className="kapi-pg-btn kapi-pg-btn--sm"
            onClick={share}
            title="Copy a shareable link that restores these files and replays these commands"
          >
            {shared ? (
              <Check size={14} aria-hidden="true" />
            ) : (
              <Share2 size={14} aria-hidden="true" />
            )}
            <span>{shared ? "Link copied" : "Share"}</span>
          </button>
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
            files={initialReq.files}
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
