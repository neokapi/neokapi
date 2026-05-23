import React, { useCallback, useEffect, useRef, useState } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import { bootKapiCli } from "./_wasmCli";
import type { KapiCli } from "./_wasmCli";
import KapiTerminal from "./_KapiTerminal";
import FilesPanel from "./_FilesPanel";
import styles from "./styles.module.css";

const SAMPLE = JSON.stringify(
  { greeting: "Hello, World!", farewell: "See you tomorrow", items: { cart: "Your cart is empty" } },
  null,
  2,
);

export default function Playground(): React.ReactElement {
  const wasmExecUrl = useBaseUrl("/wasm/wasm_exec.js");
  const wasmUrl = useBaseUrl("/wasm/kapi-cli.wasm");

  const [cli, setCli] = useState<KapiCli | null>(null);
  const [error, setError] = useState<string>("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [maximized, setMaximized] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const bump = useCallback(() => setRefreshKey((k) => k + 1), []);

  useEffect(() => {
    const onFsChange = () => setIsFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", onFsChange);
    return () => document.removeEventListener("fullscreenchange", onFsChange);
  }, []);

  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) document.exitFullscreen();
    else wrapperRef.current?.requestFullscreen?.();
  }, []);

  useEffect(() => {
    let cancelled = false;
    bootKapiCli(wasmExecUrl, wasmUrl)
      .then((c) => {
        if (cancelled) return;
        if (!c.vol.exists("/project/messages.json")) {
          c.vol.writeFile("/project/messages.json", new TextEncoder().encode(SAMPLE));
        }
        setCli(c);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => { cancelled = true; };
  }, [wasmExecUrl, wasmUrl]);

  if (error) {
    return (
      <div className={styles.notice}>
        <strong>Could not load the kapi CLI.</strong>
        <p>{error}</p>
        <p>
          The module is built by <code>make web-wasm-cli</code> and served from{" "}
          <code>/wasm/kapi-cli.wasm</code>.
        </p>
      </div>
    );
  }

  if (!cli) {
    return <p className={styles.notice}>Loading the kapi CLI (WebAssembly, ~13&nbsp;MB)…</p>;
  }

  return (
    <div ref={wrapperRef} className={`${styles.wrapper} ${maximized ? styles.maximized : ""}`}>
      <div className={styles.toolbar}>
        <span className={styles.toolbarTitle}>kapi terminal</span>
        <button type="button" className="button button--sm button--secondary" onClick={() => setMaximized((m) => !m)}>
          {maximized ? "Restore" : "Maximize"}
        </button>
        <button type="button" className="button button--sm button--secondary" onClick={toggleFullscreen}>
          {isFullscreen ? "Exit full screen" : "Full screen"}
        </button>
      </div>
      <div className={styles.layout}>
        <div className={styles.termPane}>
          <KapiTerminal cli={cli} onFsChange={bump} />
        </div>
        <div className={styles.filesPane}>
          <FilesPanel cli={cli} refreshKey={refreshKey} onChange={bump} />
        </div>
      </div>
    </div>
  );
}
