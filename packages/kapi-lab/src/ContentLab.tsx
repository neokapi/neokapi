import React, { useCallback, useEffect, useMemo, useState } from "react";
import { GraduationCap } from "lucide-react";
import { DocumentViewer } from "@neokapi/ui-primitives/preview";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
import {
  PortalThemeProvider,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
} from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";
import { useFileLibrary, resolveSelection } from "./fileLibrary";
import type { FileSelection } from "./fileLibrary";
import FileSelectorField from "./FileSelectorField";
import { SAMPLES } from "./samples";
import OutputView from "./OutputView";
import { CONTENT_LESSONS } from "./contentLessons";
import type { ContentLesson, ContentTab } from "./contentLessons";
import shared from "./styles.module.css";

// ContentLab — the lab's CONTENT-MODEL inspect surface (no flow canvas). It runs
// the real WASM reader on a file and visualizes the resulting content model with
// the shared preview kit (DocumentViewer): the structural Preview with inline
// overlay highlights, the Blocks list (source runs, targets, stand-off overlays
// and annotations), Structure / Layout, and the Raw source with a round-trip
// diff. A lesson chooses the lens — which annotators to run, whether to run a
// command and inspect its output, and which tab to open — so the same surface
// specializes to what each page teaches (anatomy, segmentation, terms/QA,
// source↔target, structure, round-trip). It replaces AnatomyExplorer and
// RoundTripExplorer.
//
// Composability: pass `lessons` (a curated set → a picker appears) or a single
// lesson via `lessonIds`/`defaultLessonId`; pages that want one fixed view pass
// a one-element set. The engine is gated behind the shared zero-shift GateOverlay
// — nothing boots or reads until the learner presses play, then the active
// lesson is inspected automatically (this IS an inspector: play → see).

// OutputView accepts a narrower tab set than DocumentViewer; map the lesson tab
// onto it (round-trip lessons want raw/blocks/preview/stats).
type OutputTab = "preview" | "blocks" | "raw" | "stats";
function outputTab(tab: ContentTab | undefined): OutputTab {
  return tab === "preview" || tab === "blocks" || tab === "raw" || tab === "stats" ? tab : "raw";
}

export interface ContentLabProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Lessons offered (default: the built-in content-model set). >1 shows a picker. */
  lessons?: ContentLesson[];
  /** Restrict the offered lessons to these ids (in this order). */
  lessonIds?: string[];
  /** Lesson selected first (default: the first offered). */
  defaultLessonId?: string;
  /** Override the active lesson's sample on first render. */
  defaultSampleId?: string;
  /** Restrict the file library's samples. */
  sampleIds?: string[];
  /** Fill the host height (app-like) instead of the inline result frame. */
  fill?: boolean;
}

export default function ContentLab({
  assets,
  lessons,
  lessonIds,
  defaultLessonId,
  defaultSampleId,
  sampleIds,
  fill,
}: ContentLabProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);

  const offered = useMemo(() => {
    const all = lessons ?? CONTENT_LESSONS;
    return lessonIds ? all.filter((l) => lessonIds.includes(l.id)) : all;
  }, [lessons, lessonIds]);
  const initialLesson = offered.find((l) => l.id === defaultLessonId) ?? offered[0];
  const [lesson, setLesson] = useState<ContentLesson>(initialLesson);

  const library = useFileLibrary({ sampleIds });
  const selectionFor = useCallback((sampleId?: string): FileSelection => {
    const s =
      SAMPLES.find((x) => x.id === sampleId) ??
      SAMPLES.find((x) => x.id === "support-reply") ??
      SAMPLES[0];
    return { mode: "single", paths: [s.filename] };
  }, []);
  const [selection, setSelection] = useState<FileSelection>(() =>
    selectionFor(defaultSampleId ?? initialLesson.sampleId),
  );
  const selected = useMemo(() => resolveSelection(selection, library), [selection, library]);
  const file = selected[0];

  const [tree, setTree] = useState<ContentTree | null>(null);
  const [outPath, setOutPath] = useState<string | null>(null);
  const [outVersion, setOutVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Round-trip baseline: the input's text, diffed against the written output.
  const baseline = useMemo(() => {
    if (!file) return null;
    try {
      return new TextDecoder().decode(file.bytes);
    } catch {
      return null;
    }
  }, [file]);

  const selectLesson = useCallback(
    (next: ContentLesson) => {
      setLesson(next);
      setSelection(selectionFor(next.sampleId));
      setTree(null);
      setOutPath(null);
      setError(null);
    },
    [selectionFor],
  );

  // Inspect the active file through the lesson's lens once the engine is ready.
  // This surface is an inspector, so activating (play) leads straight to the
  // content model; switching lesson or file re-inspects. Keyed on file.path +
  // the spec (not object identity) so it doesn't loop on every render.
  const specKey = JSON.stringify(lesson.spec);
  const filePath = file?.path;
  const { ready, inspect, inspectAnnotated, trace, writeFile, readBytes } = runtime;
  useEffect(() => {
    if (!ready || !file) return;
    let cancelled = false;
    void (async () => {
      setBusy(true);
      setError(null);
      setTree(null);
      setOutPath(null);
      try {
        const spec = lesson.spec;
        if (spec.run) {
          const inPath = writeFile(file.name, file.bytes);
          const out = `/project/out-${file.name}`;
          const argv = spec.run.argv.map((a) => (a === "{in}" ? inPath : a === "{out}" ? out : a));
          const res = await trace(argv);
          if (cancelled) return;
          if (!res.ok) {
            setError(res.error ?? "the command produced no output");
          } else if (spec.run.diff) {
            // Round-trip: show the written output diffed against the input.
            setOutPath(out);
            setOutVersion((v) => v + 1);
          } else {
            // Inspect the output for its content model (e.g. the targets a
            // bilingual writer round-trips alongside the source).
            const bytes = readBytes(out);
            const ins = bytes
              ? await inspectAnnotated(`out-${file.name}`, bytes, {})
              : { ok: false as const, error: "no output" };
            if (cancelled) return;
            if (ins.ok && ins.tree) setTree(ins.tree);
            else setError(ins.error ?? "could not inspect the output");
          }
        } else {
          const ins = spec.annotate
            ? await inspectAnnotated(file.name, file.bytes, spec.annotate)
            : await inspect(file.name, file.bytes);
          if (cancelled) return;
          if (ins.ok && ins.tree) setTree(ins.tree);
          else setError(ins.error ?? "could not read the file");
        }
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      }
      if (!cancelled) setBusy(false);
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, filePath, specKey]);

  const showPicker = offered.length > 1;
  const isDiff = !!lesson.spec.run?.diff;

  return (
    // PortalThemeProvider carries `.kapi-reference` onto the Radix-portalled
    // popovers (the lesson + file selects) so their theme vars resolve.
    <PortalThemeProvider className="kapi-reference">
      <div
        className={cn(
          "kapi-reference relative flex flex-col gap-3 text-foreground",
          fill && "h-full",
        )}
      >
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          {showPicker && (
            <Select
              value={lesson.id}
              onValueChange={(v) => {
                const next = offered.find((l) => l.id === v);
                if (next) selectLesson(next);
              }}
            >
              <SelectTrigger
                size="sm"
                className="w-[230px] gap-1.5 text-xs font-medium"
                aria-label="Lesson"
              >
                <GraduationCap className="size-3.5 shrink-0 text-muted-foreground" />
                <SelectValue placeholder="Pick a lesson">{lesson.label}</SelectValue>
              </SelectTrigger>
              <SelectContent className="w-[340px]">
                {offered.map((l) => (
                  <SelectItem key={l.id} value={l.id} textValue={l.label}>
                    <span className="flex min-w-0 flex-col gap-0.5">
                      <span className="text-xs font-medium">{l.label}</span>
                      <span className="line-clamp-2 text-[10px] leading-tight text-muted-foreground">
                        {l.description}
                      </span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <FileSelectorField
            label="File"
            library={library}
            selection={selection}
            onSelectionChange={(sel) => {
              setSelection(sel);
              setTree(null);
              setOutPath(null);
            }}
            sampleIds={sampleIds}
          />
        </div>

        <p
          className="m-0 line-clamp-2 text-xs leading-relaxed text-muted-foreground"
          title={lesson.description}
        >
          {lesson.description}
        </p>

        {/* Result frame: a reserved-height region so activating the engine (the
            gate dissolving) never shifts the page — the body keeps this height
            whether idle, reading, or showing the content model. */}
        <div className={cn("min-h-[540px]", fill && "min-h-0 flex-1")}>
          {isDiff && outPath ? (
            <OutputView
              key={`${lesson.id}:${filePath}`}
              runtime={runtime}
              path={outPath}
              version={outVersion}
              baseline={baseline}
              defaultTab={outputTab(lesson.spec.tab)}
            />
          ) : tree && file ? (
            <DocumentViewer
              key={`${lesson.id}:${filePath}`}
              tree={tree}
              filename={file.name}
              bytes={file.bytes}
              defaultTab={lesson.spec.tab}
            />
          ) : (
            <div className="flex min-h-[540px] items-center justify-center text-sm italic text-muted-foreground">
              {busy ? "Reading…" : error ? "" : ""}
            </div>
          )}
        </div>

        <div className={cn(shared.statusBar, error && shared.statusError)}>
          {runtime.status === "error" && `Failed to start: ${runtime.error}`}
          {runtime.ready && busy && "Reading…"}
          {runtime.ready && !busy && error && `Error: ${error}`}
        </div>

        {/* Shared zero-shift Run gate: an opaque overlay over the laid-out body
            until the engine is ready, then a pure dissolve to the content model. */}
        <GateOverlay
          gate={gate}
          title="Content model"
          description="See how kapi reads your file into the content model — live, in your browser."
        />
      </div>
    </PortalThemeProvider>
  );
}
