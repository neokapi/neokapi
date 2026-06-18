import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import useBaseUrl from "@docusaurus/useBaseUrl";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import ReferenceDetail from "./ReferenceDetail";
import styles from "./styles.module.css";

interface Props {
  entry: ReferenceEntry;
  /** Canonical static page route for this entry, e.g. "/reference/formats/json". */
  href: string;
  onClose: () => void;
}

function SourceBadge({ source }: { source: ReferenceSource }) {
  const label = source === "okapi" ? "Okapi bridge" : source === "plugin" ? "Plugin" : "Built-in";
  const cls =
    source === "okapi" || source === "plugin" ? styles.sourceOkapi : styles.sourceBuiltin;
  return <span className={`${styles.sourceBadge} ${cls}`}>{label}</span>;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

/**
 * Deep-linkable config explorer. Rendered (via a body portal) whenever the URL
 * carries a matching `?id=`; closing clears the query. Implements the usual
 * dialog affordances: focus trap, Escape, overlay click, scroll lock, and
 * focus restoration to the card that opened it.
 */
export default function ReferenceModal({ entry, href, onClose }: Props) {
  const panelRef = useRef<HTMLDivElement>(null);
  const restoreFocusRef = useRef<Element | null>(null);
  const [linkCopied, setLinkCopied] = useState(false);
  const pageHref = useBaseUrl(href);

  const copyLink = useCallback(() => {
    navigator.clipboard.writeText(window.location.href).then(() => {
      setLinkCopied(true);
      setTimeout(() => setLinkCopied(false), 1500);
    });
  }, []);

  // Lock body scroll while open.
  useEffect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, []);

  // Capture the trigger, move focus into the dialog, and restore on close.
  useEffect(() => {
    restoreFocusRef.current = document.activeElement;
    panelRef.current?.focus();
    return () => {
      if (restoreFocusRef.current instanceof HTMLElement) {
        restoreFocusRef.current.focus();
      }
    };
  }, []);

  // Escape to close + Tab focus trap.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key !== "Tab") return;
      const panel = panelRef.current;
      if (!panel) return;
      const items = panel.querySelectorAll<HTMLElement>(FOCUSABLE);
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      if (e.shiftKey && (active === first || active === panel)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  const titleId = `ref-modal-title-${entry.id}`;

  return createPortal(
    <div
      className={styles.overlay}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        className={styles.modal}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
      >
        <header className={styles.modalHeader}>
          <h2 id={titleId} className={styles.modalTitle}>
            {entry.displayName}
          </h2>
          <SourceBadge source={entry.source} />
          <div className={styles.modalActions}>
            <a
              className={styles.copyButton}
              href={pageHref}
              title="Open the full, shareable reference page for this entry"
            >
              View full page
            </a>
            <button
              type="button"
              className={styles.copyButton}
              onClick={copyLink}
              title="Copy a shareable link to this configuration"
            >
              {linkCopied ? "Link copied" : "Copy link"}
            </button>
            <button
              type="button"
              className={styles.modalClose}
              onClick={onClose}
              aria-label="Close"
            >
              &#10005;
            </button>
          </div>
        </header>

        <div className={styles.modalBody}>
          <ReferenceDetail entry={entry} />
        </div>
      </div>
    </div>,
    document.body,
  );
}
