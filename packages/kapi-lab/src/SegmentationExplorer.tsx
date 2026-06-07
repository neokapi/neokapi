import React, { useEffect, useMemo, useState } from "react";
import { cn } from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";

export interface SegmentationExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Initial text in the editor. */
  defaultText?: string;
  /** Initial BCP-47 locale (rule selection / ICU locale). */
  defaultLocale?: string;
}

// Friendly labels for the engines the wasm build registers. SRX is pure-Go and
// always present in the browser; UAX-29 is served by the ICU4X companion-wasm
// bridge and needs the host page to load ICU4X (see the lab's ICU4X glue).
const ENGINE_LABELS: Record<string, string> = {
  srx: "SRX rules (pure-Go, in-browser)",
  uax29: "UAX-29 (ICU4X)",
};

const PRESETS: { label: string; locale: string; text: string }[] = [
  {
    label: "English — abbreviations & numbers",
    locale: "en",
    text: "Dr. Smith met Mr. Jones in Washington. They talked for an hour. The deal closed at 3 p.m. for $9.99 per unit.",
  },
  {
    label: "English — quotes & initials",
    locale: "en",
    text: 'J. R. R. Tolkien wrote it. She said "Hello there." Then she left for the U.S. on Monday.',
  },
  {
    label: "French — M. and guillemets",
    locale: "fr",
    text: "M. Dupont est arrivé à 14 h. Il a dit « Bonjour ». La valeur est 3,14 aujourd'hui.",
  },
  {
    label: "German — z.B. / usw.",
    locale: "de",
    text: "Das gilt z.B. montags. Herr Dr. Müller kam an. Es gilt auch usw.",
  },
];

// SegmentationExplorer runs the real segmentation engine in WASM and shows where
// the sentence boundaries fall — letting a learner compare the rule-based SRX
// engine against the UAX-29 (ICU4X) baseline on text they control: abbreviations,
// decimals, initials, quotes. Segmentation is non-destructive (a stand-off
// overlay), so the engine never rewrites the text — it only marks boundaries.
export default function SegmentationExplorer({
  assets,
  defaultText,
  defaultLocale,
}: SegmentationExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const [text, setText] = useState(defaultText ?? PRESETS[0].text);
  const [engine, setEngine] = useState("srx");
  const [locale, setLocale] = useState(defaultLocale ?? PRESETS[0].locale);
  const [segments, setSegments] = useState<string[]>([]);
  const [ranEngine, setRanEngine] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // The engines this build actually registered (srx always; uax29 when the
  // ICU4X bridge is linked). Available only once the runtime is ready.
  const engines = useMemo(
    () => (runtime.ready ? runtime.segmentEngines() : ["srx"]),
    [runtime.ready, runtime.segmentEngines],
  );

  // Re-segment whenever the runtime is ready or any input changes. segment() is
  // synchronous, so no async/cancellation dance is needed.
  useEffect(() => {
    if (!runtime.ready) return;
    const res = runtime.segment(text, engine, locale);
    if (res.ok) {
      setSegments((res.segments ?? []).map((s) => s.text));
      setRanEngine(res.engine ?? engine);
      setError(null);
    } else {
      setSegments([]);
      setRanEngine(null);
      setError(res.error ?? "could not segment");
    }
  }, [runtime.ready, runtime.segment, text, engine, locale]);

  const uaxUnavailable =
    engine === "uax29" && !!error && /icu4x|not loaded|unavailable/i.test(error);

  return (
    <div className="kapi-reference flex flex-col gap-3 text-foreground">
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Engine</span>
          <select
            className="rounded border border-border bg-background px-2 py-1 text-sm"
            value={engine}
            onChange={(e) => setEngine(e.target.value)}
          >
            {engines.map((id) => (
              <option key={id} value={id}>
                {ENGINE_LABELS[id] ?? id}
              </option>
            ))}
          </select>
        </label>

        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Locale</span>
          <input
            className="w-20 rounded border border-border bg-background px-2 py-1 text-sm"
            value={locale}
            onChange={(e) => setLocale(e.target.value)}
            spellCheck={false}
          />
        </label>

        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Preset</span>
          <select
            className="rounded border border-border bg-background px-2 py-1 text-sm"
            value=""
            onChange={(e) => {
              const p = PRESETS[Number(e.target.value)];
              if (p) {
                setText(p.text);
                setLocale(p.locale);
              }
            }}
          >
            <option value="" disabled>
              Load a preset…
            </option>
            {PRESETS.map((p, i) => (
              <option key={p.label} value={i}>
                {p.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <textarea
        className="min-h-[6rem] w-full resize-y rounded border border-border bg-background p-2 font-mono text-sm"
        value={text}
        onChange={(e) => setText(e.target.value)}
        spellCheck={false}
        aria-label="Text to segment"
      />

      <div
        className={cn("min-h-[1.4rem] text-sm text-muted-foreground", error && "text-destructive")}
      >
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready &&
          !error &&
          ranEngine &&
          `${segments.length} segment${segments.length === 1 ? "" : "s"} · engine: ${ranEngine}`}
        {runtime.ready && uaxUnavailable && "UAX-29 needs ICU4X loaded on this page — try SRX."}
        {runtime.ready && error && !uaxUnavailable && `Error: ${error}`}
      </div>

      {segments.length > 0 && (
        <ol className="flex flex-col gap-2">
          {segments.map((seg, i) => (
            <li key={i} className="flex gap-3 rounded border border-border bg-muted/30 p-2 text-sm">
              <span className="select-none font-mono text-xs text-muted-foreground">{i + 1}</span>
              <span className="whitespace-pre-wrap">{seg}</span>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
