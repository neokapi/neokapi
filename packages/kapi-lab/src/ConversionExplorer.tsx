import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CodeView, DocumentViewer } from "@neokapi/ui-primitives/preview";
import type { ContentTree } from "@neokapi/ui-primitives/preview";

// CodeView's highlight languages (mirrors ui-primitives highlight.Lang).
type Lang = "json" | "xml" | "yaml" | "properties" | "po" | "markdown" | "csv" | "text";
import { useLabRuntime } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import { SAMPLES } from "./samples";
import shared from "./styles.module.css";

// ConversionExplorer shows one document every way at once. The input is parsed
// into the content model and shown in the shared DocumentViewer (Preview /
// Blocks / Structure / Layout / Stats — the same widget the other labs use), and
// alongside the built-in views sits one extra pill per *generative* output
// format. Selecting a format pill runs the real kapi `convert` (kconv) in WASM
// and shows that serialization two ways: the rendered page (HTML projection,
// left) and its raw source (right). The model-level tabs never change as you
// switch formats — that is the point: one content model, many serializations.
//
// Only generative targets are offered. Skeleton-driven formats (docx/odt/idml/
// epub/…) inject translations back into the *original* file and cannot be
// generated from a foreign model, so they are deliberately absent.

export interface ConversionTarget {
  id: string;
  label: string;
  /** Output extension used for the in-FS path (the --to flag selects the format). */
  ext: string;
}

// GENERATIVE_TARGETS is the curated set shown while the engine boots; once ready
// the lab replaces it with the authoritative generative-writer list queried from
// `kapi formats list --json` (the declared capability — no hardcoding, no plugin
// load). It is also the SSR/not-ready fallback.
// convert is for document/data projection. Bilingual interchange formats
// (XLIFF, PO, TMX, KLF) are deliberately absent — they belong to the
// extract→translate→merge loop (a converted interchange file carries no
// skeleton and cannot be merged back); see AD-005.
export const GENERATIVE_TARGETS: ConversionTarget[] = [
  { id: "doclang", label: "DocLang", ext: "dclg.xml" },
  { id: "markdown", label: "Markdown", ext: "md" },
  { id: "html", label: "HTML", ext: "html" },
  { id: "asciidoc", label: "AsciiDoc", ext: "adoc" },
  { id: "json", label: "JSON", ext: "json" },
  { id: "yaml", label: "YAML", ext: "yaml" },
  { id: "plaintext", label: "Plain text", ext: "txt" },
];

// langForTarget maps a format id to a CodeView highlight language. XML-family
// formats (doclang/xliff/…) highlight as xml; unknown ids fall back to plain.
const TARGET_LANG: Record<string, Lang> = {
  markdown: "markdown",
  html: "xml",
  json: "json",
  klf: "json",
  yaml: "yaml",
  po: "po",
  properties: "properties",
  csv: "csv",
  doclang: "xml",
  xliff: "xml",
  xliff2: "xml",
  ttml: "xml",
  resx: "xml",
  androidxml: "xml",
  xml: "xml",
};
function langForTarget(id: string): Lang {
  return TARGET_LANG[id] ?? "text";
}

// targetLabel derives a short pill label from a format id / display name.
function targetLabel(id: string, displayName?: string): string {
  if (displayName && displayName.length <= 18) return displayName;
  return id;
}

const DEFAULT_SAMPLE_IDS = [
  "article-md",
  "page-html",
  "report-doclang",
  "messages-json",
  "app-xliff",
];

/** Tab value prefix for an output-format pill (e.g. "out:markdown"). */
const OUT_PREFIX = "out:";
const outTab = (id: string): string => `${OUT_PREFIX}${id}`;
const isOutTab = (v: string): boolean => v.startsWith(OUT_PREFIX);
const outTabId = (v: string): string => v.slice(OUT_PREFIX.length);

// A converted format, cached per output id for the current input.
interface OutputState {
  status: "loading" | "ready" | "error";
  /** Serialized output (the Source pane). */
  source?: string;
  /** HTML projection for the Rendered pane (the output itself when target=html). */
  previewHtml?: string;
  error?: string;
}

const enc = new TextEncoder();

export interface ConversionExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Sample selected on first render. */
  defaultSampleId?: string;
  /** Restrict the offered samples. */
  sampleIds?: string[];
  /** Output format whose pill is active on first render (default: doclang). */
  defaultTarget?: string;
}

export default function ConversionExplorer({
  assets,
  defaultSampleId,
  sampleIds,
  defaultTarget,
}: ConversionExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const offered = sampleIds ?? DEFAULT_SAMPLE_IDS;

  const initial =
    SAMPLES.find((s) => s.id === defaultSampleId) ??
    SAMPLES.find((s) => s.id === offered[0]) ??
    SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initial.filename,
    label: initial.label,
    content: initial.content,
  });
  const [targets, setTargets] = useState<ConversionTarget[]>(GENERATIVE_TARGETS);
  // The active tab: a DocumentViewer built-in ("preview", "blocks", …) or an
  // output-format pill ("out:<id>"). Open on the requested format so the lab
  // demonstrates a conversion immediately.
  const [activeTab, setActiveTab] = useState<string>(outTab(defaultTarget ?? "doclang"));
  // The parsed input, feeding the model-level tabs.
  const [inputTree, setInputTree] = useState<ContentTree | null>(null);
  const [inputErr, setInputErr] = useState<string | null>(null);
  const [inputBusy, setInputBusy] = useState(false);
  // Converted outputs, keyed by format id, for the current input.
  const [outputs, setOutputs] = useState<Record<string, OutputState>>({});
  // Format ids whose conversion has been kicked off for the current input — a
  // ref (not state) so it doesn't retrigger effects; reset when the input changes.
  const startedRef = useRef<Set<string>>(new Set());

  const inputBytes = useMemo(
    () => file.bytes ?? enc.encode(file.content),
    [file.bytes, file.content],
  );

  // Declaratively load the conversion targets from the engine: the writers it
  // reports as `generative` (the declared, no-plugin-load capability). This is
  // the authoritative list — skeleton-bound formats (docx/odt/idml/epub) are
  // absent because they are not generative. Falls back to the curated default.
  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    void (async () => {
      try {
        const { code, output: out } = await runtime.runCapture(["formats", "list", "--json"]);
        if (cancelled || code !== 0) return;
        const data = JSON.parse(out) as {
          formats?: {
            name: string;
            display_name?: string;
            has_writer?: boolean;
            generative?: boolean;
            interchange?: boolean;
            extensions?: string[];
          }[];
        };
        // convert targets: generative document/data writers, excluding bilingual
        // interchange formats (those are the extract/merge loop, not convert).
        const list = (data.formats ?? [])
          .filter((f) => f.has_writer && f.generative && !f.interchange)
          .map((f) => ({
            id: f.name,
            label: targetLabel(f.name, f.display_name),
            ext: (f.extensions?.[0] ?? `.${f.name}`).replace(/^\./, ""),
          }))
          .sort((a, b) => a.label.localeCompare(b.label));
        if (list.length > 0) setTargets(list);
      } catch {
        /* keep the curated fallback */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime.runCapture]);

  // convertTo runs `kapi convert <in> --to <fmt> -o <out>` in WASM and returns
  // the written output (or throws with the captured error).
  const convertTo = useCallback(
    async (inPath: string, fmt: string, ext: string): Promise<string> => {
      const outPath = `/project/converted.${ext}`;
      const { code, output: log } = await runtime.runCapture([
        "convert",
        inPath,
        "--to",
        fmt,
        "-o",
        outPath,
      ]);
      const written = runtime.readFile(outPath);
      if (code !== 0 || written === null) {
        throw new Error((log || "").trim() || `conversion to ${fmt} produced no output`);
      }
      return written;
    },
    [runtime.runCapture, runtime.readFile],
  );

  // Parse the input into the content model whenever the engine becomes ready or
  // the selected file changes. A new input invalidates every cached conversion.
  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    // New input → drop cached outputs and let the active format reconvert.
    startedRef.current = new Set();
    setOutputs({});
    setInputBusy(true);
    setInputErr(null);
    void runtime
      .inspect(file.filename, inputBytes)
      .then((res) => {
        if (cancelled) return;
        if (res.ok && res.tree) {
          setInputTree(res.tree);
        } else {
          setInputTree(null);
          setInputErr(res.error ?? "could not read this document");
        }
      })
      .finally(() => !cancelled && setInputBusy(false));
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime.inspect, file.filename, inputBytes]);

  // ensureOutput lazily converts the input to one format and caches the result.
  // Guarded by startedRef so each format converts at most once per input.
  const ensureOutput = useCallback(
    async (id: string): Promise<void> => {
      if (!runtime.ready || startedRef.current.has(id)) return;
      const def = targets.find((t) => t.id === id);
      if (!def) return;
      startedRef.current.add(id);
      setOutputs((o) => ({ ...o, [id]: { status: "loading" } }));
      try {
        const inPath = runtime.writeFile(file.filename, inputBytes);
        const source = await convertTo(inPath, def.id, def.ext);
        // Rendered pane: reuse the HTML projection (or the output itself when the
        // target already is HTML). A failed projection just hides the preview.
        const previewHtml =
          def.id === "html"
            ? source
            : await convertTo(inPath, "html", "html").catch(() => undefined);
        setOutputs((o) => ({ ...o, [id]: { status: "ready", source, previewHtml } }));
      } catch (e) {
        setOutputs((o) => ({
          ...o,
          [id]: { status: "error", error: e instanceof Error ? e.message : String(e) },
        }));
      }
    },
    [runtime.ready, runtime.writeFile, convertTo, targets, file.filename, inputBytes],
  );

  // Convert the active format on demand: when the engine is ready and the active
  // tab is an output pill, ensure that format is converted. Re-runs after a new
  // input (ensureOutput identity changes and startedRef was cleared above).
  useEffect(() => {
    if (runtime.ready && isOutTab(activeTab)) void ensureOutput(outTabId(activeTab));
  }, [runtime.ready, activeTab, ensureOutput]);

  // If the active output pill isn't in the (engine-loaded) target list, fall
  // back to the first available format so the controlled Tabs always has a match.
  useEffect(() => {
    if (!isOutTab(activeTab)) return;
    const id = outTabId(activeTab);
    if (targets.length > 0 && !targets.some((t) => t.id === id)) {
      setActiveTab(outTab(targets[0].id));
    }
  }, [activeTab, targets]);

  const onTabChange = useCallback((v: string) => {
    setActiveTab(v);
  }, []);

  // Build one extra pill per output format. Each pane renders from the cached
  // conversion state (loading / error / rendered+source).
  const extraTabs = useMemo(
    () =>
      targets.map((target) => ({
        value: outTab(target.id),
        label: target.label,
        content: <OutputPane state={outputs[target.id]} target={target} />,
      })),
    [targets, outputs],
  );

  return (
    <div className={`kapi-reference relative ${shared.explorer}`}>
      <FileSource value={file} onChange={setFile} sampleIds={offered} label="Input" />

      <div className={`${shared.statusBar} ${inputErr ? shared.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads the WASM engine)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && inputBusy && "Reading document…"}
        {runtime.ready && !inputBusy && inputErr && `Error: ${inputErr}`}
        {runtime.ready && !inputBusy && !inputErr && inputTree && (
          <span className={shared.stats}>
            <span className={shared.statBadge}>{file.label}</span>
          </span>
        )}
      </div>

      <div className="min-h-[460px]">
        {inputTree && (
          <DocumentViewer
            tree={inputTree}
            filename={file.label}
            bytes={inputBytes}
            value={activeTab}
            onValueChange={onTabChange}
            extraTabs={extraTabs}
          />
        )}
        <p className="mt-3 text-sm text-muted-foreground">
          The reader parses the input into the content model (roles, runs, tables, geometry); the
          model-level tabs describe that one model, and each format pill re-serializes it through a
          generative writer. Skeleton-driven formats (docx, odt, idml, epub) inject into an original
          file and so cannot be conversion targets.
        </p>
      </div>

      <GateOverlay
        gate={gate}
        title="File conversion"
        description="Convert a document from one format to another."
      />
    </div>
  );
}

// OutputPane renders one converted format: the rendered page (left) and its raw
// source (right). Until the conversion resolves it shows a status line.
function OutputPane({
  state,
  target,
}: {
  state: OutputState | undefined;
  target: ConversionTarget;
}): React.ReactElement {
  if (!state || state.status === "loading") {
    return <p className="py-3 text-sm text-muted-foreground">Converting to {target.label}…</p>;
  }
  if (state.status === "error") {
    return (
      <p className="py-3 text-sm text-destructive">
        Could not convert to {target.label}: {state.error}
      </p>
    );
  }
  const source = state.source ?? "";
  const empty = source.trim() === "" || source.trim() === "[]";
  return (
    <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium text-muted-foreground">Rendered</span>
        {state.previewHtml && state.previewHtml.trim() !== "" ? (
          <iframe
            title={`${target.label} preview`}
            sandbox=""
            srcDoc={state.previewHtml}
            className="h-[26rem] w-full rounded-md border bg-white"
          />
        ) : (
          <p className="rounded-md border border-dashed px-3 py-6 text-sm text-muted-foreground">
            No visual preview for this document.
          </p>
        )}
      </div>
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium text-muted-foreground">Source · {target.label}</span>
        {empty ? (
          <p className="rounded-md border border-dashed px-3 py-6 text-sm text-muted-foreground">
            The {target.label} writer produced an empty document: the reader found no translatable
            content in this source.
          </p>
        ) : (
          <CodeView text={source} lang={langForTarget(target.id)} maxHeight="26rem" />
        )}
      </div>
    </div>
  );
}
