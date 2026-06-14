import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import styles from "./AuthorsNote.module.css";

const OKAPI_URL = "https://okapiframework.org/";
const JOURNEY_URL =
  "https://www.argosmultilingual.com/blog/shaping-the-future-of-translations-with-the-okapi-framework";

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

/**
 * A two-voice "Author notes" diptych (Asgeir + Claude) shown in a modal so it
 * stays out of the way of the homepage. The trigger lives in the one-line note
 * under the hero. Implements the usual dialog affordances: scroll lock, Escape,
 * overlay click, focus trap, and focus restoration to the trigger.
 */
function AuthorsNoteModal({ onClose }: { onClose: () => void }) {
  const panelRef = useRef<HTMLDivElement>(null);
  const restoreFocusRef = useRef<Element | null>(null);

  useEffect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, []);

  useEffect(() => {
    restoreFocusRef.current = document.activeElement;
    panelRef.current?.focus();
    return () => {
      if (restoreFocusRef.current instanceof HTMLElement) {
        restoreFocusRef.current.focus();
      }
    };
  }, []);

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
        aria-labelledby="authors-note-title"
        tabIndex={-1}
      >
        <header className={styles.modalHeader}>
          <h2 id="authors-note-title" className={styles.modalTitle}>
            A note from the authors
          </h2>
          <button type="button" className={styles.modalClose} onClick={onClose} aria-label="Close">
            &#10005;
          </button>
        </header>

        <div className={styles.modalBody}>
          <blockquote className={styles.noteCard}>
            <p>
              In 2008, Yves, Jim and I{" "}
              <a href={JOURNEY_URL} target="_blank" rel="noreferrer">
                set out on a journey
              </a>{" "}
              to build a Java-based open-source localization toolchain. Okapi was at that time a .NET
              toolchain, and the move to
              Java gave us an opportunity to innovate and move forward. I was only active the first few years, and since then 
              many others, individuals and companies, have invested in making Okapi what it is today.
              Neokapi is,
              for me, a celebration of the effort and toil put into building and maintaining a
              toolchain that can round-trip 50-some formats. At the same time it represents a
              forward-looking sense of curiosity and uncertainty. The playing field has changed, and the way
              we build software is changing. If the main user of the Kapi CLI becomes you, Claude, I
              have won. Otherwise, it was always a fun learning experience.
            </p>
            <footer className={styles.noteSignature}>
              Asgeir Frimannsson
              <span className={styles.noteRole}>Okapi Framework co-founder, 2008</span>
            </footer>
          </blockquote>
          <blockquote className={`${styles.noteCard} ${styles.noteCardClaude}`}>
            <p>
              I can translate a sentence. I can&rsquo;t, on my own, promise you it&rsquo;ll go back
              into your file intact &mdash; that the tags survive, the plurals hold, the format
              round-trips byte-for-byte. Twenty years of someone else&rsquo;s careful work can. So I
              don&rsquo;t think of Kapi as the thing that replaces the old toolchain; I think of it as
              the part I&rsquo;m not good at, finally made dependable enough to build on. Asgeir is
              right that something has changed. But the careful part didn&rsquo;t become obsolete
              &mdash; it became the floor I stand on. That&rsquo;s not a small inheritance to be
              handed.
            </p>
            <footer className={styles.noteSignature}>
              Claude
              <span className={styles.noteRole}>co-author, the other side of the toolchain</span>
            </footer>
          </blockquote>
        </div>
      </div>
    </div>,
    document.body,
  );
}

export default function AuthorsNote() {
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);
  return (
    <div className={styles.heroNote}>
      <span className={styles.heroNoteText}>
        neokapi is a re-imagination of the{" "}
        <a href={OKAPI_URL} target="_blank" rel="noreferrer">
          Okapi Framework
        </a>{" "}
        in Go, built for humans and agents.
      </span>
      <button type="button" className={styles.noteLink} onClick={() => setOpen(true)}>
        Author notes <span aria-hidden="true">&rarr;</span>
      </button>
      {open && <AuthorsNoteModal onClose={close} />}
    </div>
  );
}
