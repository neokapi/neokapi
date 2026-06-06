import React, { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { Play, Upload } from "lucide-react";
import { HERO_SAMPLES } from "@neokapi/kapi-playground/samples";
import type { HeroSample } from "@neokapi/kapi-playground/samples";
import { Badge, Button, cn } from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import { FileIcon } from "./fileTypes";
import { downloadBytes, downloadText, formatBytes } from "./download";
import OutputView from "./OutputView";

// A picked input: either one of the curated hero samples or a file the learner
// dropped/uploaded. We keep the raw bytes so binary formats (.docx) survive.
export interface DropInput {
  name: string;
  bytes: Uint8Array;
  binary: boolean;
}

// What the widget renders after a run:
//   "output" — the file the tool wrote, in the full OutputView (Blocks/Structure/
//              Native + download). The default; best for file-producing tools.
//   "stat"   — a compact metric card parsed from a tool's --json stdout
//              (word-count → blocks / words). For tools that report, not rewrite.
//   "diff"   — before/after source text side by side, plus a download. For
//              transforms a learner wants to compare at a glance.
export type ToolDropRender = "output" | "stat" | "diff";

export interface ToolDropStat {
  label: string;
  value: string;
}

export interface ToolDropWidgetProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** The tool/command id (used in status text and the default output name). */
  tool: string;
  /**
   * Build the argv for a flag-driven tool. Given the input and output paths
   * (absolute /project/… paths the widget owns), return the full argv, e.g.
   * `["pseudo-translate", in, "-o", out]`. Mutually exclusive with `recipe`.
   */
  buildArgv?: (inPath: string, outPath: string) => string[];
  /**
   * For config-bearing tools, return an inline `.kapi` recipe (one `lab` flow
   * carrying the tool + its config). The widget writes it and runs
   * `run lab -p <recipe> -i <in> -o <out>`. Mutually exclusive with `buildArgv`.
   */
  recipe?: () => string;
  /** Extra argv appended to the run (e.g. ["--target-lang", "fr"]). */
  extraArgs?: string[];
  /** Restrict the offered samples to these hero-sample ids (default: all). */
  sampleIds?: string[];
  /** Sample selected on first render (default: first offered). */
  autoSampleId?: string;
  /** Allow binary uploads (.docx/.xlsx/.pptx). Default true. */
  acceptBinary?: boolean;
  /** How to render the result (default "output"). */
  render?: ToolDropRender;
  /**
   * For render="stat": parse the tool's captured stdout into metric cards.
   * Receives the captured stdout (e.g. word-count --json) and returns the
   * cards to show. Defaults to a JSON word-count parser.
   */
  parseStat?: (stdout: string) => ToolDropStat[];
  /** Run automatically once the runtime is ready and on input change. Default true. */
  autoRun?: boolean;
  className?: string;
}

const dec = new TextDecoder();

// The browser wasm build forces CLICOLOR_FORCE=1 (so the terminal renders ANSI),
// which means even `--json` output arrives wrapped in colour escape codes. Strip
// them before parsing. Exported-shape regex matches CSI sequences (ESC [ … m/K/…).
// eslint-disable-next-line no-control-regex
const ANSI = /\x1b\[[0-9;]*[A-Za-z]/g;

function stripAnsi(s: string): string {
  return s.replace(ANSI, "");
}

// The default stat parser understands `kapi word-count --json` output:
//   { total_source_words, document_count, documents: { uri: { source_words, block_count } } }
export function parseWordCountStat(stdout: string): ToolDropStat[] {
  try {
    const j = JSON.parse(stripAnsi(stdout)) as {
      total_source_words?: number;
      documents?: Record<string, { source_words?: number; block_count?: number }>;
    };
    const docs = Object.values(j.documents ?? {});
    const blocks = docs.reduce((n, d) => n + (d.block_count ?? 0), 0);
    const words = j.total_source_words ?? docs.reduce((n, d) => n + (d.source_words ?? 0), 0);
    const chars = words * 5; // rough: estimate characters from words (avg word ~5 chars)
    return [
      { label: "Blocks", value: String(blocks) },
      { label: "Words", value: words.toLocaleString() },
      { label: "~Characters", value: chars.toLocaleString() },
    ];
  } catch {
    return [];
  }
}

function offeredSamples(sampleIds?: string[]): HeroSample[] {
  if (!sampleIds) return HERO_SAMPLES;
  return sampleIds
    .map((id) => HERO_SAMPLES.find((s) => s.id === id))
    .filter((s): s is HeroSample => !!s);
}

// ToolDropWidget is the reusable no-terminal "drop a file → see the result"
// surface. A learner drops a file (or picks a sample), the widget runs ONE tool
// on it in the shared kapi WASM, and renders the result — the written file
// (OutputView), a parsed stat card, or a before/after diff — with a download.
//
// It is lazy: the WASM boots only when the host mounts it (useLabRuntime boots
// on first render of the booting subtree), and runs are namespaced per widget
// instance so two widgets on a page never collide in the in-memory filesystem.
export default function ToolDropWidget({
  assets,
  tool,
  buildArgv,
  recipe,
  extraArgs = [],
  sampleIds,
  autoSampleId,
  acceptBinary = true,
  render = "output",
  parseStat = parseWordCountStat,
  autoRun = true,
  className,
}: ToolDropWidgetProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const samples = useMemo(() => offeredSamples(sampleIds), [sampleIds]);
  // A per-instance namespace so output/recipe paths never clash across widgets.
  const ns = useId().replace(/[:]/g, "");

  const initial = useMemo<DropInput>(() => {
    const s = samples.find((x) => x.id === autoSampleId) ?? samples[0];
    return { name: s.filename, bytes: s.bytes(), binary: s.binary };
  }, [samples, autoSampleId]);

  const [input, setInput] = useState<DropInput>(initial);
  const [outPath, setOutPath] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const [stats, setStats] = useState<ToolDropStat[] | null>(null);
  const [diff, setDiff] = useState<{ before: string; after: string; bytes: Uint8Array } | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const pickSample = useCallback((s: HeroSample) => {
    setInput({ name: s.filename, bytes: s.bytes(), binary: s.binary });
    setError(null);
  }, []);

  const acceptFiles = useCallback(
    async (files: FileList | File[]) => {
      const f = Array.from(files)[0];
      if (!f) return;
      const bytes = new Uint8Array(await f.arrayBuffer());
      const binary = /\.(docx|xlsx|pptx|pdf|zip)$/i.test(f.name);
      if (binary && !acceptBinary) {
        setError("This widget only accepts text files.");
        return;
      }
      setInput({ name: f.name, bytes, binary });
      setError(null);
    },
    [acceptBinary],
  );

  const runTool = useCallback(async () => {
    if (!runtime.ready) return;
    setBusy(true);
    setError(null);
    // Namespace input + output under the instance id so concurrent widgets on
    // one page never overwrite each other's files in the shared memfs.
    const inPath = runtime.writeFile(`${ns}-${input.name}`, input.bytes);
    const outPath = `/project/${ns}-out-${input.name}`;

    let argv: string[];
    if (recipe) {
      const recipePath = runtime.writeFile(`${ns}-recipe.kapi`, recipe());
      argv = ["run", "lab", "-p", recipePath, "-i", inPath, "-o", outPath, ...extraArgs];
    } else if (buildArgv) {
      argv = [...buildArgv(inPath, outPath), ...extraArgs];
    } else {
      setError("ToolDropWidget needs either buildArgv or recipe.");
      setBusy(false);
      return;
    }

    if (render === "stat") {
      // Stat tools report on stdout; capture it and parse to cards.
      const { code, output } = await runtime.runCapture(argv);
      const cards = parseStat(output);
      if (code !== 0 && cards.length === 0) {
        setError(output.trim() || `the run exited ${code}`);
        setStats(null);
      } else {
        setStats(cards);
      }
      setBusy(false);
      return;
    }

    const code = await runtime.run(argv);
    const outBytes = runtime.readBytes(outPath);
    if (code !== 0 && !outBytes) {
      setError(`the run exited ${code}`);
      setBusy(false);
      return;
    }
    if (!outBytes || outBytes.length === 0) {
      setError("the run produced no output");
      setBusy(false);
      return;
    }

    if (render === "diff") {
      const before = input.binary ? "" : dec.decode(input.bytes);
      const after = input.binary ? "" : dec.decode(outBytes);
      setDiff({ before, after, bytes: outBytes });
    } else {
      setOutPath(outPath);
      setVersion((v) => v + 1);
    }
    setBusy(false);
  }, [
    runtime.ready,
    runtime.writeFile,
    runtime.run,
    runtime.runCapture,
    runtime.readBytes,
    ns,
    input,
    recipe,
    buildArgv,
    extraArgs,
    render,
    parseStat,
  ]);

  // Auto-run once ready and whenever the input changes. Debounced so a fast
  // sample-swap or a config-driven recipe re-render coalesces into one run.
  useEffect(() => {
    if (!autoRun || !runtime.ready) return;
    const h = setTimeout(() => void runTool(), 200);
    return () => clearTimeout(h);
  }, [autoRun, runtime.ready, runTool]);

  return (
    <div className={cn("kapi-reference flex flex-col gap-3 text-foreground", className)}>
      {/* Drop-zone + sample chips. */}
      <div
        className={cn(
          "flex flex-col gap-2 rounded-lg border border-dashed bg-card p-3 transition-colors",
          dragging && "border-primary bg-primary/5",
        )}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          void acceptFiles(e.dataTransfer.files);
        }}
      >
        <div className="flex flex-wrap items-center gap-2">
          <FileIcon filename={input.name} size={16} />
          <span className="font-mono text-sm">{input.name}</span>
          <span className="text-xs tabular-nums text-muted-foreground">
            {formatBytes(input.bytes.length)}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="ml-auto"
            onClick={() => fileRef.current?.click()}
          >
            <Upload /> Drop or choose a file
          </Button>
          <input
            ref={fileRef}
            type="file"
            className="hidden"
            accept={acceptBinary ? undefined : ".json,.html,.xml,.xliff,.po,.txt,.yaml,.yml,.md"}
            onChange={(e) => {
              if (e.target.files) void acceptFiles(e.target.files);
              e.target.value = "";
            }}
          />
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-muted-foreground">Try a sample:</span>
          {samples.map((s) => (
            <button
              key={s.id}
              type="button"
              onClick={() => pickSample(s)}
              className={cn(
                "inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs transition-colors",
                input.name === s.filename
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border bg-background text-muted-foreground hover:border-primary",
              )}
            >
              <FileIcon filename={s.filename} size={13} />
              {s.label}
            </button>
          ))}
          <Button
            type="button"
            size="sm"
            className="ml-auto"
            onClick={() => void runTool()}
            disabled={!runtime.ready || busy}
          >
            <Play /> Run
          </Button>
        </div>
      </div>

      {/* Status line. */}
      <div
        className={cn("min-h-[1.2rem] text-sm text-muted-foreground", error && "text-destructive")}
      >
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && `Running ${tool}…`}
        {runtime.ready && !busy && error && `Error: ${error}`}
      </div>

      {/* Result. */}
      {render === "stat" && stats && (
        <div className="flex flex-wrap gap-2">
          {stats.map((s) => (
            <div
              key={s.label}
              className="flex min-w-[6rem] flex-col gap-0.5 rounded-lg border bg-card px-4 py-3"
            >
              <span className="text-2xl font-bold tabular-nums">{s.value}</span>
              <span className="text-xs uppercase tracking-wide text-muted-foreground">
                {s.label}
              </span>
            </div>
          ))}
        </div>
      )}

      {render === "diff" && diff && (
        <div className="flex flex-col gap-2 rounded-lg border bg-card p-3">
          <div className="grid gap-3 md:grid-cols-2">
            <div className="flex flex-col gap-1">
              <Badge variant="outline" className="self-start">
                Before
              </Badge>
              <pre className="overflow-auto rounded bg-muted/40 p-2 text-xs">
                {input.binary ? "(binary input — download to inspect)" : diff.before}
              </pre>
            </div>
            <div className="flex flex-col gap-1">
              <Badge variant="outline" className="self-start border-primary/50 text-primary">
                After
              </Badge>
              <pre className="overflow-auto rounded bg-muted/40 p-2 text-xs">
                {input.binary ? "(binary output — download to inspect)" : diff.after}
              </pre>
            </div>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="self-start"
            onClick={() =>
              input.binary
                ? downloadBytes(input.name, diff.bytes)
                : downloadText(input.name, diff.after)
            }
          >
            Download result
          </Button>
        </div>
      )}

      {render === "output" && outPath && (
        <OutputView runtime={runtime} path={outPath} version={version} />
      )}
    </div>
  );
}
