import React, { useEffect, useRef, useState } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import { usePluginManager, probePluginCaches } from "@neokapi/kapi-playground/plugins";
import type {
  PluginDescriptor,
  PluginState,
  EngineState,
  ModelDescriptor,
  ModelState,
} from "@neokapi/kapi-playground/plugins";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";
import styles from "./styles.module.css";

// The Neokapi WebAssembly Lab status widget — a compact navbar pill that mirrors
// "what is loaded in this browser tab" (the desktop/CLI plugin manager, on the
// web). The collapsed pill shows engine status + a ready/total count; expanding
// it reveals the plugin list with explicit Download actions and live progress.
// It reads the single shared plugin store, so a download started here or from any
// lab updates both.

function formatSize(bytes?: number): string {
  if (!bytes) return "";
  if (bytes >= 1_000_000_000) return `${(bytes / 1_000_000_000).toFixed(1)} gb`;
  if (bytes >= 1_000_000) return `${(bytes / 1_000_000).toFixed(1)} mb`;
  return `${Math.round(bytes / 1000)} kb`;
}

function engineDotClass(engine: EngineState): string {
  switch (engine.phase) {
    case "ready":
      return styles.dotReady;
    case "booting":
      return styles.dotBusy;
    case "error":
      return styles.dotError;
    default:
      return styles.dotIdle;
  }
}

function DownloadIcon(): React.ReactElement {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="7 10 12 15 17 10" />
      <line x1="12" y1="15" x2="12" y2="3" />
    </svg>
  );
}

function PluginRow({
  d,
  st,
  onDownload,
}: {
  d: PluginDescriptor;
  st: PluginState;
  onDownload: () => void;
}): React.ReactElement {
  const unavailable = st.phase === "unavailable";
  return (
    <div className={`${styles.row} ${unavailable ? styles.rowMuted : ""}`}>
      <div className={styles.rowMain}>
        <div className={styles.rowTitle}>
          <span className={styles.rowLabel}>{d.label}</span>
          {d.sizeBytes ? <span className={styles.rowSize}>{formatSize(d.sizeBytes)}</span> : null}
        </div>
        <div className={styles.rowDesc}>{d.description}</div>
      </div>
      <div className={styles.rowControl}>
        {(st.phase === "ready" || st.phase === "cached") && (
          <span
            className={`${styles.dot} ${styles.dotReady}`}
            aria-label={st.phase === "cached" ? "downloaded (cached)" : "loaded"}
            title={
              st.phase === "cached" ? "Downloaded — cached, loads instantly when used" : "Loaded"
            }
          />
        )}
        {unavailable && <span className={styles.naLabel}>Not available</span>}
        {st.phase === "idle" && (
          <button
            type="button"
            className={styles.downloadBtn}
            onClick={onDownload}
            aria-label={`Download ${d.label}`}
            title="Download"
          >
            <DownloadIcon />
          </button>
        )}
        {st.phase === "error" && (
          <button
            type="button"
            className={`${styles.downloadBtn} ${styles.retryBtn}`}
            onClick={onDownload}
          >
            Retry
          </button>
        )}
        {st.phase === "downloading" && (
          <div className={styles.progressWrap} aria-label="downloading">
            <div className={styles.progressBar}>
              <div
                className={styles.progressFill}
                style={{ width: `${Math.round((st.progress?.frac ?? progressFrac(st)) * 100)}%` }}
              />
              <span className={styles.progressPct}>
                {Math.round((st.progress?.frac ?? progressFrac(st)) * 100)}%
              </span>
            </div>
            {st.progress?.total ? (
              <div className={styles.progressBytes}>
                {formatSize(st.progress.loaded)} of {formatSize(st.progress.total)}
              </div>
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}

function progressFrac(st: PluginState): number {
  const p = st.progress;
  if (!p) return 0;
  if (typeof p.frac === "number") return p.frac;
  if (p.total) return (p.loaded ?? 0) / p.total;
  return 0;
}

// ModelRow renders one on-device LLM model as a child of the "Local LLM" provider
// group — same controls/markup as a plugin row, plus an engine tag and a default
// marker. Wired to ensureModel (download via WebLLM/transformers).
function ModelRow({
  d,
  st,
  onDownload,
}: {
  d: ModelDescriptor;
  st: ModelState;
  onDownload: () => void;
}): React.ReactElement {
  const frac = st.progress?.frac ?? 0;
  return (
    <div className={styles.row}>
      <div className={styles.rowMain}>
        <div className={styles.rowTitle}>
          <span className={styles.rowLabel}>{d.label}</span>
          <span className={styles.rowSize}>{d.size}</span>
        </div>
        <div className={styles.rowDesc}>
          {d.engine}
          {d.isDefault ? " · default" : ""}
          {d.note ? ` · ${d.note}` : ""}
        </div>
      </div>
      <div className={styles.rowControl}>
        {(st.phase === "ready" || st.phase === "cached") && (
          <span
            className={`${styles.dot} ${styles.dotReady}`}
            aria-label={st.phase === "cached" ? "downloaded (cached)" : "loaded"}
            title={
              st.phase === "cached" ? "Downloaded — cached, loads instantly when used" : "Loaded"
            }
          />
        )}
        {st.phase === "idle" && (
          <button
            type="button"
            className={styles.downloadBtn}
            onClick={onDownload}
            aria-label={`Download ${d.label}`}
            title="Download"
          >
            <DownloadIcon />
          </button>
        )}
        {st.phase === "error" && (
          <button
            type="button"
            className={`${styles.downloadBtn} ${styles.retryBtn}`}
            onClick={onDownload}
          >
            Retry
          </button>
        )}
        {st.phase === "downloading" && (
          <div className={styles.progressWrap} aria-label="downloading">
            <div className={styles.progressBar}>
              <div
                className={styles.progressFill}
                style={{ width: `${Math.round(frac * 100)}%` }}
              />
              <span className={styles.progressPct}>{Math.round(frac * 100)}%</span>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default function StatusWidget(): React.ReactElement {
  const config = useKapiPlaygroundConfig();
  const mgr = usePluginManager();
  const { siteConfig, i18n } = useDocusaurusContext();
  const [open, setOpen] = useState(false);
  const [panelStyle, setPanelStyle] = useState<React.CSSProperties | undefined>(undefined);
  const rootRef = useRef<HTMLDivElement>(null);
  const logo = useBaseUrl("img/logo.png");

  // Vision (PP-OCRv5) model base, resolved exactly like the lab pages: CDN when
  // configured, else same-origin /models/vision (mapped back to the root for
  // non-default locales). The bridge's GitHub-release default can't be fetched
  // from a browser (no CORS), so the widget MUST inject this for vision to load.
  const localizedModels = useBaseUrl("/models/vision");
  const cdn = readCdnConfig(siteConfig);
  const visionModelBase = cdnEnabled(cdn)
    ? cdnHref(cdn, `/models/vision/${cdn.modelsVersion}`)
    : i18n.currentLocale === i18n.defaultLocale
      ? localizedModels
      : localizedModels.replace(`/${i18n.currentLocale}/`, "/");

  // Inject the host-resolved asset URLs into the shared manager so a Download
  // here loads the same engine + models the labs use. Idempotent.
  useEffect(() => {
    mgr.configure({
      wasmExecUrl: config.wasmExecUrl,
      wasmUrl: config.wasmUrl,
      visionModelBase,
    });
  }, [mgr, config.wasmExecUrl, config.wasmUrl, visionModelBase]);

  // Reflect models already in the browser cache from a previous session as
  // "downloaded" (rather than offering a fresh download). Runs once.
  useEffect(() => {
    void probePluginCaches();
  }, []);

  // On mobile the pill sits left of the viewport's right edge (the locale and
  // GitHub items follow it), so a pill-anchored `right: 0` panel overflows
  // off-screen. Pin it to the viewport instead — just below the navbar, inset
  // from both edges — and recompute on scroll/resize while open. Desktop keeps
  // the CSS-driven absolute placement (panelStyle stays undefined).
  useEffect(() => {
    if (!open) {
      setPanelStyle(undefined);
      return;
    }
    const place = () => {
      if (window.innerWidth > 996) {
        setPanelStyle(undefined);
        return;
      }
      const rect = rootRef.current?.getBoundingClientRect();
      if (!rect) return;
      setPanelStyle({
        position: "fixed",
        top: Math.round(rect.bottom + 8),
        left: 8,
        right: 8,
        width: "auto",
        maxHeight: `calc(100vh - ${Math.round(rect.bottom + 24)}px)`,
      });
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [open]);

  // Close the popover on an outside click or Escape.
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const { engine } = mgr.state;
  const { ready, total } = mgr.counts;

  return (
    <div className={styles.root} ref={rootRef}>
      <button
        type="button"
        className={styles.pill}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-label="Neokapi WebAssembly Lab status"
      >
        <img src={logo} alt="" className={styles.pillLogo} />
        <span className={styles.pillText}>
          <span className={styles.pillTitle}>Neokapi</span>
          <span className={styles.pillSub}>WebAssembly Lab</span>
        </span>
        <span className={`${styles.dot} ${engineDotClass(engine)}`} />
        {total > 0 && ready > 0 && (
          <span className={styles.count}>
            {ready}/{total}
          </span>
        )}
        <span className={`${styles.chevron} ${open ? styles.chevronOpen : ""}`} aria-hidden>
          ⌄
        </span>
      </button>

      {open && (
        <div className={styles.panel} style={panelStyle} role="dialog" aria-label="Neokapi plugins">
          <div className={styles.panelHeader}>
            <img src={logo} alt="" className={styles.panelLogo} />
            <div className={styles.panelHeaderText}>
              <div className={styles.panelTitle}>Neokapi</div>
              <div className={styles.panelSub}>WebAssembly Lab</div>
            </div>
            <span
              className={`${styles.dot} ${engineDotClass(engine)}`}
              title={`engine: ${engine.phase}`}
            />
          </div>

          <div className={styles.engineLine}>
            {engine.phase === "ready" && <span>Engine ready</span>}
            {engine.phase === "booting" && <span>Booting engine…</span>}
            {engine.phase === "error" && (
              <span className={styles.engineErr}>Engine error: {engine.error}</span>
            )}
            {engine.phase === "idle" && (
              <button
                type="button"
                className={styles.bootBtn}
                onClick={() => void mgr.bootEngine()}
              >
                Boot engine
              </button>
            )}
          </div>

          <div className={styles.plugins}>
            <div className={styles.pluginsHeading}>Plugins</div>
            {mgr.descriptors.map((d) => (
              <PluginRow
                key={d.id}
                d={d}
                st={mgr.state.plugins[d.id]}
                onDownload={() => void mgr.ensure(d.id).catch(() => undefined)}
              />
            ))}
          </div>

          {/* Local LLM provider group — the in-browser local-inference provider
              (NOT the retired native llm plugin). Its on-device models are listed
              as nested children, each downloadable via the WebLLM/transformers
              bridge. Mirrors how `kapi models` lists models under their provider. */}
          <div className={styles.plugins}>
            <div className={styles.pluginsHeading}>Local LLM</div>
            <div className={styles.rowDesc} style={{ margin: "-2px 0 8px" }}>
              On-device LLMs — same lineup as <code>kapi ollama</code>.
            </div>
            <div
              style={{
                marginLeft: 8,
                paddingLeft: 8,
                borderLeft: "2px solid var(--ifm-color-emphasis-200, #e3e8ee)",
              }}
            >
              {mgr.modelDescriptors.map((d) => (
                <ModelRow
                  key={d.id}
                  d={d}
                  st={mgr.state.models[d.id]}
                  onDownload={() => void mgr.ensureModel(d.id).catch(() => undefined)}
                />
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
