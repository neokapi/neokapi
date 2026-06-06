import React, { Suspense, useState } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@neokapi/ui-primitives";
import { ArrowRight, BarChart3, Languages, Play, Replace } from "lucide-react";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";
import Showcase from "./Showcase";
import OwnFiles from "./OwnFiles";
import type { DemoId } from "./mocks";
import styles from "./styles.module.css";

// "Try Neokapi in your browser" — the docs landing centerpiece. A single
// prominent hero CTA opens a centered modal (the ui-primitives Dialog) with
// three demo tabs over an instant, FAKED visual showcase of three documents
// (a slide, a worksheet, a markdown doc). The active demo applies a simulated
// transform in plain JS and highlights the changes — no wasm, no real files.
//
// Two real-engine escapes sit beneath the faked visual:
//   • RealProof — Download a genuine source file + a "Download result" that runs
//     the REAL kapi WASM engine on it (booted lazily, only on that click).
//   • OwnFiles — "Try with your own files" swaps the showcase for the real Lab
//     widgets (drop/choose + real engine + block diff/stats).
//
// The page stays zero-wasm on load: nothing here boots the engine until the
// reader presses Download-result or runs an own-files widget.

const DEMOS: { id: DemoId; label: string; icon: typeof Play }[] = [
  { id: "search-replace", label: "Search / replace", icon: Replace },
  { id: "insights", label: "Insights", icon: BarChart3 },
  { id: "pseudo", label: "Pseudo-translate", icon: Languages },
];

// RealProof imports @neokapi/kapi-lab (the wasm runtime hook), so it is loaded
// client-only + code-split — it never enters the SSR bundle and pulls no wasm
// until mounted. The wasm itself boots only when its Download-result is pressed.
const LazyRealProof = React.lazy(() => import("./RealProof"));

function RealProofClient({ find, replace }: { find: string; replace: string }): React.ReactElement {
  return (
    <BrowserOnly fallback={<div />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<div />}>
              <LazyRealProof assets={assets} find={find} replace={replace} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

function ModalBody(): React.ReactElement {
  const [demo, setDemo] = useState<DemoId>("search-replace");
  const [find, setFind] = useState("Acme");
  const [replace, setReplace] = useState("Globex");
  const [own, setOwn] = useState(false);

  return (
    <div className={styles.body}>
      <div className={styles.tabBar} role="tablist" aria-label="Pick a demo">
        {DEMOS.map((d) => {
          const Icon = d.icon;
          return (
            <button
              key={d.id}
              type="button"
              role="tab"
              aria-selected={d.id === demo}
              className={`${styles.tab} ${d.id === demo ? styles.tabActive : ""}`}
              onClick={() => setDemo(d.id)}
            >
              <Icon size={14} aria-hidden="true" /> {d.label}
            </button>
          );
        })}
        <div className={styles.toggleRow} style={{ marginLeft: "auto" }}>
          <button
            type="button"
            className={styles.toggleBtn}
            aria-pressed={own}
            onClick={() => setOwn((v) => !v)}
          >
            {own ? "Back to the showcase" : "Try with your own files"}
          </button>
        </div>
      </div>

      {/* The find/replace controls only matter for the search/replace demo. */}
      {demo === "search-replace" && !own && (
        <div className={styles.controls}>
          <label className={styles.field}>
            Find
            <input
              className={styles.input}
              value={find}
              onChange={(e) => setFind(e.target.value)}
              spellCheck={false}
            />
          </label>
          <ArrowRight className={styles.arrow} size={16} aria-hidden="true" />
          <label className={styles.field}>
            Replace
            <input
              className={styles.input}
              value={replace}
              onChange={(e) => setReplace(e.target.value)}
              spellCheck={false}
            />
          </label>
          <span className={styles.controlNote}>
            Edits the translatable text inside each document — keys, markup, and structure stay
            untouched.
          </span>
        </div>
      )}

      {own ? (
        <OwnFiles demo={demo} />
      ) : (
        <>
          <Showcase demo={demo} find={find} replace={replace} />
          <RealProofClient find={find} replace={replace} />
        </>
      )}
    </div>
  );
}

export default function TryNeokapi(): React.ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        className={styles.heroTrigger}
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
      >
        <span className={styles.heroPlay}>
          <Play size={16} aria-hidden="true" fill="currentColor" />
        </span>
        Try Neokapi in your browser
      </button>
      <p className={styles.heroTriggerHint}>
        Search and replace, count, and pseudo-translate across PowerPoint, Excel, and Markdown —
        runs locally, no sign-up.
      </p>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          className="kapi-reference sm:!max-w-3xl"
          // The modal content is plain CSS-module markup, not shadcn forms, so
          // wrap it in .kapi-reference for the token palette + give it room.
        >
          <DialogHeader>
            <div className={styles.modalHead}>
              <DialogTitle asChild>
                <span className={styles.modalTitle}>Search and replace across file formats.</span>
              </DialogTitle>
              <p className={styles.modalSub}>
                One engine reads the translatable text out of any document, transforms it, and
                writes the file back — faithfully. Try it on three formats at once.
              </p>
            </div>
          </DialogHeader>
          <ModalBody />
        </DialogContent>
      </Dialog>
    </>
  );
}
