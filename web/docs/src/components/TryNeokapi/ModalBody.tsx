import React, { useState } from "react";
import { ArrowRight } from "lucide-react";
import { useLabRuntime } from "@neokapi/kapi-lab";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";
import Showcase from "./Showcase";
import RealProof from "./RealProof";
import OwnFiles from "./OwnFiles";
import type { DemoId } from "./demos";
import styles from "./styles.module.css";

// The heavy modal body: it boots the kapi WASM engine (lazily, only because it
// is mounted only when the modal opens) and drives the LIVE showcase. It keeps
// the three demo tabs, the find/replace controls, the real Download source /
// Download result proof, and the "Try with your own files" path.

interface ModalBodyProps {
  demos: { id: DemoId; label: string; icon: typeof ArrowRight }[];
}

export default function ModalBody({ demos }: ModalBodyProps): React.ReactElement {
  const assets = useKapiPlaygroundConfig();
  // The modal owns the engine for its whole lifetime — booting on open is fine
  // (the page hero stays zero-wasm; only opening the modal pulls the engine).
  const runtime = useLabRuntime(assets);

  const [demo, setDemo] = useState<DemoId>("search-replace");
  const [find, setFind] = useState("Acme");
  const [replace, setReplace] = useState("Globex");
  const [own, setOwn] = useState(false);

  return (
    <div className={styles.body}>
      <div className={styles.tabBar} role="tablist" aria-label="Pick a demo">
        {demos.map((d) => {
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
          <Showcase runtime={runtime} demo={demo} find={find} replace={replace} />
          <RealProof assets={assets} find={find} replace={replace} runtime={runtime} />
        </>
      )}
    </div>
  );
}
