import React, { Suspense, useState } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@neokapi/ui-primitives";
import { BarChart3, Languages, Replace } from "lucide-react";
import Carousel from "./Carousel";
import type { DemoId } from "./demos";
import styles from "./styles.module.css";

// The docs landing centerpiece. The hero is a zero-wasm Format-carousel (baked
// RenderDoc data rendered through the shared DocumentRender) that auto-cycles
// slide → sheet → doc, each revealing EN → FR with the changed words
// highlighted. Clicking it opens a modal (the ui-primitives Dialog) that boots
// the kapi WASM engine and runs the REAL extraction + transform on three real
// files (deck.pptx · report.xlsx · guide.md), showing a genuine before/after via
// the same DocumentRender — so the instant teaser and the live proof look
// identical.
//
// The page stays zero-wasm on load: nothing boots the engine until the reader
// opens the modal. The heavy modal body (which imports the lab runtime) is
// client-only + code-split so it never enters the SSR bundle.

const DEMOS: { id: DemoId; label: string; icon: typeof Replace }[] = [
  { id: "search-replace", label: "Search / replace", icon: Replace },
  { id: "insights", label: "Insights", icon: BarChart3 },
  { id: "pseudo", label: "Pseudo-translate", icon: Languages },
];

// ModalBody imports @neokapi/kapi-lab (the wasm runtime), so it is loaded
// client-only + code-split — it never enters the SSR bundle and pulls no wasm
// until the modal opens.
const LazyModalBody = React.lazy(() => import("./ModalBody"));

function ModalBodyClient(): React.ReactElement {
  return (
    <BrowserOnly fallback={<div className={styles.showcaseLoading}>Loading…</div>}>
      {() => (
        <Suspense fallback={<div className={styles.showcaseLoading}>Loading…</div>}>
          <LazyModalBody demos={DEMOS} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}

export default function TryNeokapi(): React.ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Carousel onOpen={() => setOpen(true)} />

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="kapi-reference sm:!max-w-4xl">
          <DialogHeader>
            <div className={styles.modalHead}>
              <DialogTitle asChild>
                <span className={styles.modalTitle}>
                  Read, change, and ship content in any format.
                </span>
              </DialogTitle>
              <p className={styles.modalSub}>
                One engine reads the translatable text out of any document, transforms it, and
                writes the file back — faithfully. This runs the real kapi engine in your browser,
                live, on three formats at once.
              </p>
            </div>
          </DialogHeader>
          {open && <ModalBodyClient />}
        </DialogContent>
      </Dialog>
    </>
  );
}
