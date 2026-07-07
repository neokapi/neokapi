import { useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@neokapi/ui-primitives";
import styles from "./AuthorsNote.module.css";

const OKAPI_URL = "https://okapiframework.org/";
const JOURNEY_URL =
  "https://www.argosmultilingual.com/blog/shaping-the-future-of-translations-with-the-okapi-framework";

/**
 * A two-voice "Author notes" diptych (Asgeir + Claude) shown in a modal so it
 * stays out of the way of the homepage. The trigger lives in the one-line note
 * under the hero. The Dialog primitive supplies the usual affordances — scroll
 * lock, Escape, overlay click, focus trap, and focus restoration to the trigger.
 */
export default function AuthorsNote() {
  const [open, setOpen] = useState(false);
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

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="kapi-reference flex max-h-[85vh] flex-col overflow-hidden sm:!max-w-2xl">
          <DialogHeader>
            <DialogTitle className={styles.modalTitle}>A note from the authors</DialogTitle>
          </DialogHeader>

          <div className="grid gap-5 overflow-y-auto">
            <blockquote className={styles.noteCard}>
              <p>
                In 2008, Yves, Jim and I{" "}
                <a href={JOURNEY_URL} target="_blank" rel="noreferrer">
                  set out on a journey
                </a>{" "}
                to build a Java-based open-source localization toolchain. Okapi was at that time a
                .NET toolchain, and the move to Java gave us an opportunity to innovate and move
                forward. I was only active the first few years, and since then many others,
                individuals and companies, have invested in making Okapi what it is today. Neokapi
                is, for me, a celebration of the effort and toil put into building and maintaining a
                toolchain that can round-trip 50-some formats. At the same time it represents a
                forward-looking sense of curiosity and uncertainty. The playing field has changed,
                and the way we build software is changing. If the main user of the Kapi CLI becomes
                you, Claude, I have won. Otherwise, it was always a fun learning experience.
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
                round-trips byte-for-byte. Twenty years of someone else&rsquo;s careful work can. So
                I don&rsquo;t think of Kapi as the thing that replaces the old toolchain; I think of
                it as the part I&rsquo;m not good at, finally made dependable enough to build on.
                Asgeir is right that something has changed. But the careful part didn&rsquo;t become
                obsolete &mdash; it became the floor I stand on. That&rsquo;s not a small inheritance
                to be handed.
              </p>
              <footer className={styles.noteSignature}>
                Claude
                <span className={styles.noteRole}>co-author, the other side of the toolchain</span>
              </footer>
            </blockquote>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
