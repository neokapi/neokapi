import React, { useCallback, useEffect, useId, useMemo, useState } from "react";
import { Play, Upload } from "lucide-react";
import { HERO_SAMPLES } from "@neokapi/kapi-playground/samples";
import type { HeroSample } from "@neokapi/kapi-playground/samples";
import { Badge, Button, cn } from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";
import type { LabRuntimeAssets } from "./useLabRuntime";
import { CodeView, FileIcon } from "@neokapi/ui-primitives/preview";
import { downloadBytes, downloadText, formatBytes } from "@neokapi/ui-primitives/preview";
import OutputView from "./OutputView";

// A picked input: either one of the curated hero samples or a file the learner
// dropped/uploaded. We keep the raw bytes so binary formats (.docx) survive.
export interface DropInput {
  name: string;
  bytes: Uint8Array;
  binary: boolean;
}

// What the widget renders under the command's output after a run:
//   "output" — the file the tool wrote, in the full OutputView (Blocks/Structure/
//              Native + download). The default; best for file-producing tools.
//   "text"   — nothing beyond the output itself. For tools that report.
//   "stat"   — a compact metric card parsed from a tool's --json stdout
//              (stats → blocks / words / characters), beside the output.
//   "diff"   — before/after source text side by side, plus a download. For
//              transforms a learner wants to compare at a glance.
export type ToolDropRender = "output" | "text" | "stat" | "diff";

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
   * For config-bearing tools, return an inline `kapi.yaml` recipe (one `lab` flow
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
  /** A file to load on first render instead of a sample (e.g. one the host dropped). */
  initialInput?: DropInput | null;
  /** Allow binary uploads (.docx/.xlsx/.pptx). Default true. */
  acceptBinary?: boolean;
  /** How to render the result (default "output"). */
  render?: ToolDropRender;
  /**
   * For render="stat": parse the tool's captured stdout into metric cards.
   * Receives the captured stdout (e.g. stats --json) and returns the
   * cards to show. Defaults to the `kapi stats --json` parser.
   */
  parseStat?: (stdout: string) => ToolDropStat[];
  /**
   * Run as soon as the engine is ready and again on every input change,
   * instead of waiting for the Run control. Default false: the reader runs
   * the command, the way a terminal would.
   */
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

// The default stat parser understands `kapi stats --json` output:
//   { files: [...], total: { blocks, translatable, words, characters, segments } }
export function parseStatsStat(stdout: string): ToolDropStat[] {
  try {
    const j = JSON.parse(stripAnsi(stdout)) as {
      total?: { blocks?: number; translatable?: number; words?: number; characters?: number };
    };
    const t = j.total ?? {};
    return [
      { label: "Blocks", value: String(t.blocks ?? 0) },
      { label: "Words", value: (t.words ?? 0).toLocaleString() },
      { label: "Characters", value: (t.characters ?? 0).toLocaleString() },
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

/** Quote an argument the way a shell line would show it. */
function shellWord(a: string): string {
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(a) ? a : `'${a.replace(/'/g, "'\\''")}'`;
}

/** The command line as the reader would type it: the widget's own paths replaced by names. */
export function commandLine(argv: string[], names: Record<string, string>): string {
  return ["kapi", ...argv.map((a) => shellWord(names[a] ?? a))].join(" ");
}

interface RunResult {
  code: number;
  output: string;
}

// ToolDropWidget is the reusable no-terminal "pick a file, run one command,
// read the result" surface. A learner drops a file (or picks a sample); the
// widget shows the command it is about to run, runs it in the shared kapi
// WASM when Run is pressed, shows what the command printed, and renders the
// result — the written file (OutputView), a parsed stat card, or a
// before/after diff — with a download.
//
// It is lazy: the WASM boots only when the reader presses the gate's Run, and
// runs are namespaced per widget instance so two widgets on a page never
// collide in the in-memory filesystem.
export default function ToolDropWidget({
  assets,
  tool,
  buildArgv,
  recipe,
  extraArgs = [],
  sampleIds,
  autoSampleId,
  initialInput,
  acceptBinary = true,
  render = "output",
  parseStat = parseStatsStat,
  autoRun = false,
  className,
}: ToolDropWidgetProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const samples = useMemo(() => offeredSamples(sampleIds), [sampleIds]);
  // A per-instance namespace so output/recipe paths never clash across widgets.
  const ns = useId().replace(/[:]/g, "");

  const initial = useMemo<DropInput>(() => {
    if (initialInput) return initialInput;
    const s = samples.find((x) => x.id === autoSampleId) ?? samples[0];
    return { name: s.filename, bytes: s.bytes(), binary: s.binary };
  }, [samples, autoSampleId, initialInput]);

  const [input, setInput] = useState<DropInput>(initial);
  const [outPath, setOutPath] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const [stats, setStats] = useState<ToolDropStat[] | null>(null);
  const [diff, setDiff] = useState<{
    before: string;
    after: string;
    bytes: Uint8Array;
  } | null>(null);
  const [result, setResult] = useState<RunResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);

  // The paths this instance owns, and the names the reader sees for them.
  const inPath = `/project/${ns}-${input.name}`;
  const outName = `out/${input.name}`;
  const outAbs = `/project/${ns}-out-${input.name}`;
  const recipeAbs = `/project/${ns}/kapi.yaml`;

  const argv = useMemo<string[] | null>(() => {
    if (recipe) return ["run", "lab", "-p", recipeAbs, "-i", inPath, "-o", outAbs, ...extraArgs];
    if (buildArgv) return [...buildArgv(inPath, outAbs), ...extraArgs];
    return null;
  }, [recipe, buildArgv, extraArgs, inPath, outAbs, recipeAbs]);

  const shown = useMemo(
    () =>
      argv
        ? commandLine(argv, { [inPath]: input.name, [outAbs]: outName, [recipeAbs]: "kapi.yaml" })
        : `kapi ${tool}`,
    [argv, inPath, input.name, outAbs, outName, recipeAbs, tool],
  );

  const clearResult = useCallback(() => {
    setResult(null);
    setStats(null);
    setDiff(null);
    setOutPath(null);
    setError(null);
  }, []);

  const pickSample = useCallback(
    (s: HeroSample) => {
      setInput({ name: s.filename, bytes: s.bytes(), binary: s.binary });
      clearResult();
    },
    [clearResult],
  );

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
      clearResult();
    },
    [acceptBinary, clearResult],
  );

  const runTool = useCallback(async () => {
    if (!runtime.ready || !argv) {
      if (!argv) setError("ToolDropWidget needs either buildArgv or recipe.");
      return;
    }
    setBusy(true);
    setError(null);
    // Namespace input + output under the instance id so concurrent widgets on
    // one page never overwrite each other's files in the shared memfs.
    runtime.writeFile(`${ns}-${input.name}`, input.bytes);
    if (recipe) {
      // The recipe goes in a directory of its own: the run leaves a `.kapi/`
      // state dir beside it, and in /project that dir would have no
      // `kapi.yaml` beside it, which fails every later command run there.
      runtime.mkdir(ns);
      runtime.writeFile(`${ns}/kapi.yaml`, recipe());
    }

    const { code, output } = await runtime.runCapture(argv);
    setResult({ code, output: stripAnsi(output) });

    if (render === "stat") {
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
    if (render === "text") {
      if (code !== 0) setError(`the run exited ${code}`);
      setBusy(false);
      return;
    }

    const outBytes = runtime.readBytes(outAbs);
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
      setOutPath(outAbs);
      setVersion((v) => v + 1);
    }
    setBusy(false);
  }, [
    runtime.ready,
    runtime.mkdir,
    runtime.writeFile,
    runtime.runCapture,
    runtime.readBytes,
    ns,
    input,
    argv,
    recipe,
    render,
    parseStat,
    outAbs,
  ]);

  // Only on request: run once ready and whenever the input changes. Debounced
  // so a fast sample-swap or a config-driven recipe re-render coalesces.
  useEffect(() => {
    if (!autoRun || !runtime.ready) return;
    const h = setTimeout(() => void runTool(), 200);
    return () => clearTimeout(h);
  }, [autoRun, runtime.ready, runTool]);

  return (
    <div className={cn("kapi-reference relative flex flex-col gap-3 text-foreground", className)}>
      {/* The input: a drop-zone with sample chips. */}
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
          {/* Native label→input: clicking the label opens the picker without a
              programmatic .click(), which some browsers block on a hidden input. */}
          <Button asChild variant="outline" size="sm" className="ml-auto cursor-pointer">
            <label>
              <Upload /> Drop or choose a file
              <input
                type="file"
                className="sr-only"
                accept={
                  acceptBinary ? undefined : ".json,.html,.xml,.xliff,.po,.txt,.yaml,.yml,.md"
                }
                onChange={(e) => {
                  if (e.target.files) void acceptFiles(e.target.files);
                  e.target.value = "";
                }}
              />
            </label>
          </Button>
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
        </div>
      </div>

      {/* The command, and the control that runs it. */}
      <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-card px-3 py-2">
        <code className="min-w-0 flex-1 font-mono text-sm">
          <span className="text-muted-foreground" aria-hidden="true">
            ${" "}
          </span>
          {shown}
        </code>
        {result && !busy && (
          <span
            className={cn(
              "text-xs tabular-nums",
              result.code === 0 ? "text-muted-foreground" : "text-destructive",
            )}
          >
            exited {result.code}
          </span>
        )}
        <Button
          type="button"
          size="sm"
          onClick={() => void runTool()}
          disabled={!runtime.ready || busy}
        >
          <Play /> {result ? "Run again" : "Run"}
        </Button>
      </div>

      {/* Status line. */}
      <div
        className={cn("min-h-[1.2rem] text-sm text-muted-foreground", error && "text-destructive")}
      >
        {runtime.status === "booting" && "Starting the kapi engine…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && `Running ${tool}…`}
        {runtime.ready && !busy && error && `Error: ${error}`}
        {runtime.ready &&
          !busy &&
          !error &&
          !result &&
          "Press Run to run the command on this file."}
      </div>

      {/* What the command printed. */}
      {result && (
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-muted-foreground">Output</span>
          <pre className="m-0 max-h-72 overflow-auto rounded-md bg-[#14151c] px-3 py-2 font-mono text-[0.8rem] leading-relaxed text-[#d8dbe6]">
            {result.output.trim() === "" ? "(no output)" : result.output}
          </pre>
        </div>
      )}

      {/* The result, by kind. */}
      <div className={cn(render === "text" ? "" : "min-h-[320px]")}>
        {render === "stat" && stats && (
          <div className="flex flex-wrap gap-2">
            {stats.map((s) => (
              <div
                key={s.label}
                className="flex min-w-[6rem] flex-col gap-0.5 rounded-lg border bg-card px-4 py-3"
              >
                <span className="text-2xl font-bold tabular-nums">{s.value}</span>
                <span className="text-xs text-muted-foreground">{s.label}</span>
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
                {input.binary ? (
                  <pre className="overflow-auto rounded bg-muted/40 p-2 text-xs">
                    (binary input, download to inspect)
                  </pre>
                ) : (
                  <CodeView
                    text={diff.before}
                    filename={input.name}
                    lineNumbers={false}
                    maxHeight="16rem"
                  />
                )}
              </div>
              <div className="flex flex-col gap-1">
                <Badge variant="outline" className="self-start border-primary/50 text-primary">
                  After
                </Badge>
                {input.binary ? (
                  <pre className="overflow-auto rounded bg-muted/40 p-2 text-xs">
                    (binary output, download to inspect)
                  </pre>
                ) : (
                  <CodeView
                    text={diff.after}
                    filename={input.name}
                    lineNumbers={false}
                    maxHeight="16rem"
                  />
                )}
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
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted-foreground">
              The file it wrote, {outName}
            </span>
            <OutputView runtime={runtime} path={outPath} version={version} />
          </div>
        )}
      </div>

      <GateOverlay
        gate={gate}
        title={`kapi ${tool}`}
        description="Runs on the file you pick, in your browser."
      />
    </div>
  );
}
