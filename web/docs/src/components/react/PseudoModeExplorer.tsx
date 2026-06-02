import React, { useEffect, useState } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";

// The runtime subpath is browser-safe: it imports only React + local relative
// modules. The package ROOT (`@neokapi/kapi-react`) pulls in the build-time
// extractor, which depends on the native `@swc/core` Rust binary — that must
// never enter the docs client bundle, so we import only from the two runtime
// subpaths here.
import { __t, useNeokapi } from "@neokapi/kapi-react/runtime";
import {
  setPseudoMode,
  getPseudoMode,
  BUILT_IN_ALPHABETS,
  type AlphabetName,
  type PseudoConfig,
} from "@neokapi/kapi-react/runtime/pseudo";

import styles from "./PseudoModeExplorer.module.css";

// A handful of real UI strings. In a kapi-react app the Vite/Rollup plugin
// rewrites each JSX text node into a `__t(hash, fallback, params?)` call at
// build time; here we call `__t` directly with the source text as the
// `fallback`, which is exactly what the plugin emits. The docs site has no
// catalog loaded, so the dict is empty and `__t` returns the fallback — then
// the runtime string transform (pseudo, when active) runs on top of it. This
// is the documented "works without a catalog" path.
//
// Each hash is an arbitrary stable key; with an empty dict its only role is to
// look up nothing and fall through to the fallback. The `{name}` string shows
// that `{param}` placeholders survive the transform verbatim.
const STRINGS: ReadonlyArray<{
  hash: string;
  source: string;
  params?: Record<string, string | number>;
}> = [
  { hash: "demo.welcome", source: "Welcome back" },
  { hash: "demo.save", source: "Save changes" },
  { hash: "demo.signedIn", source: "Signed in as {name}", params: { name: "Ada" } },
  { hash: "demo.settings", source: "Open settings" },
];

const ALPHABETS = Object.keys(BUILT_IN_ALPHABETS) as AlphabetName[];

function PseudoModeExplorerInner(): React.ReactElement {
  // Subscribe to the runtime store. `setPseudoMode` installs/removes the
  // string transform via `setStringTransform`, which bumps the store version
  // and re-renders every `useNeokapi()` subscriber — so flipping the toggle
  // below repaints the translated strings with no manual force-render.
  useNeokapi();

  const [on, setOn] = useState<boolean>(() => getPseudoMode() !== null);
  const [expansion, setExpansion] = useState(30);
  const [alphabet, setAlphabet] = useState<AlphabetName>("accented");

  // Drive the module-global runtime state from React state. Runs only in the
  // browser (this component is mounted inside <BrowserOnly>), so it never
  // executes during Docusaurus server rendering.
  useEffect(() => {
    if (!on) {
      setPseudoMode(null);
      return;
    }
    const config: PseudoConfig = { expansion, alphabet };
    setPseudoMode(config);
  }, [on, expansion, alphabet]);

  // Clear the global transform when the widget unmounts (navigating away),
  // so pseudo mode doesn't bleed into the rest of the docs site.
  useEffect(() => {
    return () => setPseudoMode(null);
  }, []);

  return (
    <div className={styles.explorer}>
      <div className={styles.controls}>
        <label className={styles.toggle}>
          <input
            type="checkbox"
            checked={on}
            onChange={(e) => setOn(e.target.checked)}
            aria-label="pseudo mode"
          />
          <span>
            Pseudo mode <strong>{on ? "on" : "off"}</strong>
          </span>
        </label>
        <label className={styles.control}>
          <span>
            expansion: <strong>+{expansion}%</strong>
          </span>
          <input
            type="range"
            min={0}
            max={100}
            step={5}
            value={expansion}
            disabled={!on}
            onChange={(e) => setExpansion(Number(e.target.value))}
            aria-label="expansion"
          />
        </label>
        <label className={styles.control}>
          <span>alphabet</span>
          <select
            value={alphabet}
            disabled={!on}
            onChange={(e) => setAlphabet(e.target.value as AlphabetName)}
            aria-label="alphabet"
          >
            {ALPHABETS.map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className={styles.colLabel}>Live UI — rendered through the kapi-react runtime</div>
      <ul className={styles.strings}>
        {STRINGS.map((s) => (
          <li key={s.hash} className={styles.stringRow}>
            {__t(s.hash, s.source, s.params)}
          </li>
        ))}
      </ul>

      <div className={styles.hint}>
        No catalog is loaded — every string falls through to its source text, and{" "}
        <code>setPseudoMode</code> transforms it on the fly. Notice <code>{"{name}"}</code> stays
        literal: <code>{"{param}"}</code> tokens are preserved through the transform.
      </div>
    </div>
  );
}

/**
 * PseudoModeExplorer surfaces kapi-react's runtime `setPseudoMode` as an
 * in-browser toggle. The runtime mutates module-global state and re-renders via
 * `useSyncExternalStore`, so the widget is client-only: Docusaurus
 * server-renders pages, and touching that state during SSR would cause a
 * hydration mismatch. `<BrowserOnly>` defers the whole subtree to the browser.
 */
export function PseudoModeExplorer(): React.ReactElement {
  return (
    <BrowserOnly
      fallback={
        <div className={styles.explorer}>
          <div className={styles.hint}>Loading the pseudo-mode explorer…</div>
        </div>
      }
    >
      {() => <PseudoModeExplorerInner />}
    </BrowserOnly>
  );
}

export default PseudoModeExplorer;
