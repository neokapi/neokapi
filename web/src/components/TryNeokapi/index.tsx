import React, { Suspense, useState } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import { HeroReelFallback } from "./HeroReel";
import styles from "./styles.module.css";

// HeroReel drives Motion animation + client timers, so it is loaded client-only:
// on the server (and until the client mounts) we render the static HeroReelFallback
// so Motion never enters the SSR bundle. This keeps the docs build SSR-safe.
const LazyHeroReel = React.lazy(() => import("./HeroReel"));

function HeroReelClient({ onOpen }: { onOpen: () => void }): React.ReactElement {
  return (
    <BrowserOnly fallback={<HeroReelFallback onOpen={onOpen} />}>
      {() => (
        <Suspense fallback={<HeroReelFallback onOpen={onOpen} />}>
          <LazyHeroReel onOpen={onOpen} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}

// The docs landing centerpiece. The hero is a zero-wasm, cinematic "content
// loop" reel (HeroReel): a self-playing Motion sequence that loops through four
// phases — a "The Content Loop" title card pulses in, the single-language loop
// (Shape → Write → Check) animates and visibly iterates (Write⇄Check) and
// converges into a persistent green ship gate; a "Going Multilingual" title card
// pulses in, the localization loop (Read → Prep → Recycle → Translate → Check)
// streams formats, memory, and languages and converges into the same green gate.
// The recurring gate is the convergence climax — the loop that resolves, not a
// pipeline. Pure JS animation, no engine boot; guarded behind BrowserOnly with a
// static SSR fallback. Under prefers-reduced-motion it shows a static frame.
// Clicking the card opens a modal (the ui-primitives Dialog) that boots the kapi
// WASM engine and drives a single coherent surface: a FileBrowser of real sample
// files across formats, opening into a DocumentViewer powered by live extraction
// (inspect + inspectAnnotated) with a real pseudo-translate target — so the
// instant teaser and the live proof tell the same story.
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
      <HeroReelClient onOpen={() => setOpen(true)} />

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
