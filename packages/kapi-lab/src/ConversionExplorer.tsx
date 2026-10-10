import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CodeView, DocumentViewer, FormatPreview } from "@neokapi/ui-primitives/preview";
import type { ContentTree } from "@neokapi/ui-primitives/preview";

// CodeView's highlight languages (mirrors ui-primitives highlight.Lang).
type Lang = "json" | "xml" | "yaml" | "properties" | "po" | "markdown" | "csv" | "text";
import { useLabRuntime } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSelectorField from "./FileSelectorField";
import { resolveSelection, useFileLibrary, type FileSelection } from "./fileLibrary";
import { SAMPLES } from "./samples";
import shared from "./styles.module.css";

// ConversionExplorer shows one document every way at once. The input is parsed
// into the content model and shown in the shared DocumentViewer (Preview /
// Blocks / Structure / Layout / Stats — the same widget the other labs use), and
// alongside the built-in views sits one extra pill per *document* output format.
// Selecting a format pill runs the real kapi `kconv` in WASM and shows that
// serialization two ways: a faithful rendering of the document the target
// reconstructs (the converted output read back and projected via FormatPreview,
// left) and its raw source (right) — so a table that survives docx→Markdown, or
// inline bold/links, is visible in the preview, not just the raw output. The
// model-level tabs never change as you switch formats — that is the point: one
// content model, many serializations.
//
// The inputs are the bundled text samples plus any Office documents the host
// hands in by URL (a Word handbook, an Excel workbook, a PowerPoint deck): the
// reader side is every format the engine reads, the writer side the document
// formats it generates.

export interface ConversionTarget {
  id: string;
  label: string;
  /** Output extension used for the in-FS path (the --to flag selects the format). */
  ext: string;
}

/** A binary sample the host serves (a .docx, .xlsx, .pptx), fetched on mount. */
export interface ConversionSampleSpec {
  /** URL to fetch the bytes from (e.g. "/samples/handbook.docx"). */
  url: string;
  /** File name in the library (defaults to the URL's basename). */
  name?: string;
}

// The families whose generative writers are document conversion targets: text
// with block structure and inline styling (HTML, Markdown, DocLang, AsciiDoc)
// and plain text. The engine declares every format's family
// (`kapi formats list --json`), so the pills follow the registry rather than a
// list kept here. A catalog writer (JSON, YAML, .strings) is generative too,
// but a Word document does not convert to a string catalog; those belong to
// `kconv` on catalog inputs. Bilingual interchange (XLIFF, PO) belongs to the
// extract → merge loop and is excluded on its own flag.
export const DOCUMENT_FAMILIES: ReadonlySet<string> = new Set(["rich-markup", "plain-text"]);

// DOCUMENT_TARGETS is the set shown while the engine boots and the SSR
// fallback; once ready the lab replaces it with the engine's own list of
// document-family generative writers.
export const DOCUMENT_TARGETS: ConversionTarget[] = [
  { id: "asciidoc", label: "AsciiDoc", ext: "adoc" },
  { id: "doclang", label: "DocLang", ext: "dclg.xml" },
  { id: "html", label: "HTML", ext: "html" },
  { id: "markdown", label: "Markdown", ext: "md" },
  { id: "plaintext", label: "Plain Text", ext: "txt" },
];

// langForTarget maps a format id to a CodeView highlight language. XML-family
// formats (doclang/xliff/…) highlight as xml; unknown ids fall back to plain.
const TARGET_LANG: Record<string, Lang> = {
  markdown: "markdown",
  mdx: "markdown",
  html: "xml",
  json: "json",
  kbf: "json",
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
  /**
   * The content tree of the *converted* output, read back through the engine.
   * FormatPreview renders its projected render AST (tree.render) for the
   * Rendered pane — so the preview reflects the target's reconstruction of the
   * document (reconstructed tables, inline formatting), proving the structure
   * survived the format crossing rather than re-projecting the source as HTML.
   */
  tree?: ContentTree | null;
  error?: string;
}

export interface ConversionExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /**
   * Input selected on first render: a bundled text sample's id, or the name of
   * one of `samples` (a file the host serves).
   */
  defaultSampleId?: string;
  /** Restrict the offered text samples. */
  sampleIds?: string[];
  /** Binary samples the host serves, fetched on mount and added to the library. */
  samples?: ConversionSampleSpec[];
  /** Output format whose pill is active on first render (default: markdown). */
  defaultTarget?: string;
  /**
   * Start with the engine when mounted, with no Run gate of its own: for a
   * host whose own Play already asked the reader (the learning labs).
   */
  autoStart?: boolean;
}

function sampleName(spec: ConversionSampleSpec): string {
  return spec.name ?? spec.url.split("/").pop() ?? "sample";
}

export default function ConversionExplorer({
  assets,
  defaultSampleId,
  sampleIds,
  samples,
  defaultTarget,
  autoStart = false,
}: ConversionExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: autoStart });
  const gate = useRunGate(runtime, { autoArm: autoStart });
  const offered = sampleIds ?? DEFAULT_SAMPLE_IDS;
  const library = useFileLibrary({ sampleIds: offered });
  const { addFile } = library;

  // The requested input: a text sample's file name, or a served file's name,
  // which exists in the library only once its fetch lands.
  const requested = useMemo(() => {
    const text = SAMPLES.find((s) => s.id === defaultSampleId);
    if (text) return text.filename;
    const served = (samples ?? []).find((s) => sampleName(s) === defaultSampleId);
    if (served) return sampleName(served);
    const first = SAMPLES.find((s) => s.id === offered[0]);
    if (first) return first.filename;
    return samples && samples.length > 0 ? sampleName(samples[0]) : SAMPLES[0].filename;
  }, [defaultSampleId, samples, offered]);

  const [selection, setSelection] = useState<FileSelection>(() => ({
    mode: "single",
    paths: [requested],
  }));
  const [sampleError, setSampleError] = useState<string | null>(null);

  // Fetch the served samples into the library. Cancel abandoned loads
  // (including StrictMode's effect replay) before they write to the library.
  useEffect(() => {
    if (!samples || samples.length === 0) return;
    const controller = new AbortController();
    setSampleError(null);
    void Promise.allSettled(
      samples.map(async (spec) => {
        const response = await fetch(spec.url, { signal: controller.signal });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const bytes = new Uint8Array(await response.arrayBuffer());
        return { name: sampleName(spec), bytes };
      }),
    ).then((results) => {
      if (controller.signal.aborted) return;
      const failed: string[] = [];
      results.forEach((result, index) => {
        if (result.status === "fulfilled") {
          addFile(result.value.name, result.value.bytes, "sample");
        } else {
          failed.push(sampleName(samples[index]));
        }
      });
      if (failed.length > 0) {
        setSampleError(
          `Could not load ${failed.join(", ")}. Check your connection or upload a file.`,
        );
      }
    });
    return () => controller.abort();
  }, [samples, addFile]);

  const file = useMemo(() => resolveSelection(selection, library)[0], [selection, library]);
  const [targets, setTargets] = useState<ConversionTarget[]>(DOCUMENT_TARGETS);
  // The active tab: a DocumentViewer built-in ("preview", "blocks", …) or an
  // output-format pill ("out:<id>"). Open on the requested format so the lab
  // demonstrates a conversion immediately.
  const [activeTab, setActiveTab] = useState<string>(outTab(defaultTarget ?? "markdown"));
  // The parsed input, feeding the model-level tabs.
  const [inputTree, setInputTree] = useState<ContentTree | null>(null);
  const [inputErr, setInputErr] = useState<string | null>(null);
  const [inputBusy, setInputBusy] = useState(false);
  // Converted outputs, keyed by format id, for the current input.
  const [outputs, setOutputs] = useState<Record<string, OutputState>>({});
  // Format ids whose conversion has been kicked off for the current input — a
  // ref (not state) so it doesn't retrigger effects; reset when the input changes.
  const startedRef = useRef<Set<string>>(new Set());

  const inputBytes = file?.bytes;
  const inputPath = file?.path;

  // Declaratively load the conversion targets from the engine: the writers it
  // reports as generative, outside the interchange formats, in a document
  // family. This is the authoritative list; skeleton-bound formats (docx, odt,
  // idml, epub) are absent because they are not generative, catalog formats
  // because a document is not a catalog. Falls back to the curated default.
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
            family?: string;
            extensions?: string[];
          }[];
        };
        const list = (data.formats ?? [])
          .filter(
            (f) =>
              f.has_writer &&
              f.generative &&
              !f.interchange &&
              DOCUMENT_FAMILIES.has(f.family ?? ""),
          )
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

  // convertTo runs `kapi kconv <in> --to <fmt> -o <out>` in WASM and returns
  // the written output (or throws with the captured error).
  //
  // The verb is `kconv`, not `convert`: #1180 gave every toolbox utility its
  // standalone binary name so bare verbs stay free for real commands. This call
  // site kept the old spelling and the lab answered every conversion with
  // `unknown command "convert" for "kapi"`.
  const convertTo = useCallback(
    async (inPath: string, fmt: string, ext: string): Promise<string> => {
      const outPath = `/project/converted.${ext}`;
      const { code, output: log } = await runtime.runCapture([
        "kconv",
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
    if (!runtime.ready || !inputPath || !inputBytes) return;
    let cancelled = false;
    // New input → drop cached outputs and let the active format reconvert.
    startedRef.current = new Set();
    setOutputs({});
    setInputBusy(true);
    setInputErr(null);
    void runtime
      .inspect(inputPath, inputBytes)
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
  }, [runtime.ready, runtime.inspect, inputPath, inputBytes]);

  // ensureOutput lazily converts the input to one format and caches the result.
  // Guarded by startedRef so each format converts at most once per input.
  const ensureOutput = useCallback(
    async (id: string): Promise<void> => {
      if (!runtime.ready || !inputPath || !inputBytes || startedRef.current.has(id)) return;
      const def = targets.find((t) => t.id === id);
      if (!def) return;
      startedRef.current.add(id);
      setOutputs((o) => ({ ...o, [id]: { status: "loading" } }));
      try {
        const inPath = runtime.writeFile(inputPath, inputBytes);
        const source = await convertTo(inPath, def.id, def.ext);
        // Rendered pane: read the converted output back through the engine and
        // render its projected tree (FormatPreview), so the preview reflects the
        // *target's* reconstruction — the same projected-tree preview the rest of
        // the kit uses, not a separate HTML projection of the source. A read-back
        // failure just hides the preview.
        const res = await runtime.inspect(`converted.${def.ext}`, source).catch(() => null);
        const tree = res && res.ok ? (res.tree ?? null) : null;
        setOutputs((o) => ({ ...o, [id]: { status: "ready", source, tree } }));
      } catch (e) {
        setOutputs((o) => ({
          ...o,
          [id]: {
            status: "error",
            error: e instanceof Error ? e.message : String(e),
          },
        }));
      }
    },
    [runtime.ready, runtime.writeFile, runtime.inspect, convertTo, targets, inputPath, inputBytes],
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

  const waitingForSample = !file && (samples?.length ?? 0) > 0 && !sampleError;

  return (
    <div className={`kapi-reference relative ${shared.explorer}`}>
      <FileSelectorField
        label="Input"
        library={library}
        selection={selection}
        onSelectionChange={setSelection}
        multiple={false}
        sampleIds={offered}
      />

      <div className={`${shared.statusBar} ${inputErr || sampleError ? shared.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads the WASM engine)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && waitingForSample && "Loading the sample…"}
        {runtime.ready && sampleError && `Error: ${sampleError}`}
        {runtime.ready && inputBusy && "Reading document…"}
        {runtime.ready && !inputBusy && inputErr && `Error: ${inputErr}`}
        {runtime.ready && !inputBusy && !inputErr && inputTree && file && (
          <span className={shared.stats}>
            <span className={shared.statBadge}>{file.name}</span>
          </span>
        )}
      </div>

      <div className="min-h-[460px]">
        {inputTree && file && inputBytes && (
          <DocumentViewer
            tree={inputTree}
            filename={file.name}
            bytes={inputBytes}
            value={activeTab}
            onValueChange={onTabChange}
            extraTabs={extraTabs}
          />
        )}
        <p className="mt-3 text-sm text-muted-foreground">
          The reader parses the input into the content model (roles, runs, tables, geometry); the
          model-level tabs describe that one model, and each format pill re-serializes it through a
          generative document writer. The pills are the document formats the engine generates: a
          Word, Excel or PowerPoint file is read and rewritten as any of them, and written back only
          into its own original file.
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
        {state.tree ? (
          <div className="h-[26rem] w-full overflow-auto rounded-md border">
            <FormatPreview tree={state.tree} />
          </div>
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
