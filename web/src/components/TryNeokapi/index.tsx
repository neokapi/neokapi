import React, { Suspense, useState } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import HeroProcess from "./HeroProcess";
import styles from "./styles.module.css";

// The docs landing centerpiece. The hero is a zero-wasm process "show"
// (HeroProcess): baked RenderDoc frames rendered through the shared FormatPreview
// that auto-advance through kapi end to end — Read → Pre-process → Pseudo →
// Leverage → Translate (ja) → Merge — with a stepper and typewriter/crossfade
// transitions. Clicking it opens a modal (the ui-primitives Dialog) that boots
// the kapi WASM engine and drives a single coherent surface: a FileBrowser of
// real sample files across formats, opening into a DocumentViewer powered by
// live extraction (inspect + inspectAnnotated) with a real pseudo-translate
// target — so the instant teaser and the live proof tell the same story.
//
// The page stays zero-wasm on load: nothing boots the engine until the reader
// opens the modal. The heavy modal body (which imports the lab runtime) is
// client-only + code-split so it never enters the SSR bundle.

// ModalBody imports @neokapi/kapi-lab (the wasm runtime), so it is loaded
// client-only + code-split — it never enters the SSR bundle and pulls no wasm
// until the modal opens.
const LazyModalBody = React.lazy(() => import("./ModalBody"));

function ModalBodyClient(): React.ReactElement {
  return (
    <BrowserOnly fallback={<div className={styles.showcaseLoading}>Loading…</div>}>
      {() => (
        <Suspense fallback={<div className={styles.showcaseLoading}>Loading…</div>}>
          <LazyModalBody />
        </Suspense>
      )}
    </BrowserOnly>
  );
}

export default function TryNeokapi(): React.ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <HeroProcess onOpen={() => setOpen(true)} />

      <Dialog open={open} onOpenChange={setOpen}>
        {/* Cap the modal to the viewport and lay it out as a flex column so the
            body scrolls INTERNALLY — a document with many blocks (e.g. a PPTX
            with dozens of placeholder paragraphs) must not push the dialog past
            the screen. */}
        <DialogContent className="kapi-reference flex max-h-[88dvh] flex-col overflow-hidden sm:!max-w-4xl">
          <DialogHeader>
            <div className={styles.modalHead}>
              <DialogTitle asChild>
                <span className={styles.modalTitle}>
                  Read, change, and ship content in any format.
                </span>
              </DialogTitle>
              <DialogDescription asChild>
                <p className={styles.modalSub}>
                  One engine reads the translatable text out of any document, annotates and
                  transforms it, and writes the file back — faithfully. Pick a file below: this runs
                  the real kapi engine in your browser, live, across every format.
                </p>
              </DialogDescription>
            </div>
          </DialogHeader>
          {open && <ModalBodyClient />}
        </DialogContent>
      </Dialog>
    </>
  );
}
