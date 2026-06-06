import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, Loader2, Sparkles } from "lucide-react";
import { TRY_SAMPLES, getFixture } from "@neokapi/kapi-playground";
import { useLabRuntime, type LabRuntime, type ContentTree } from "@neokapi/kapi-lab";
import FileBrowser, { type BrowserFile } from "@neokapi/kapi-lab/FileBrowser";
import DocumentViewer from "@neokapi/kapi-lab/DocumentViewer";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";
import styles from "./styles.module.css";

// The modal is ONE coherent surface: a FileBrowser of real sample files across
// formats (pptx · xlsx · md · json · html) backed by live extraction, and a
// DocumentViewer for the selected file. It is driven by the REAL kapi WASM
// engine booted on modal open, but the engine work is split by cost:
//
//   • On boot, each sample is parsed once with `inspect`; the parsed tree backs
//     both its grid thumbnail AND its viewer — so OPENING a file is instant and
//     does no new engine work (it can never hang).
//   • Annotate + translate is an explicit, on-demand action in the detail bar:
//     `inspectAnnotated` adds the read-only annotator overlays (vocabulary,
//     brand, rule-based QA) and a real pseudo-translate run produces an fr-FR
//     target merged back on, lighting up the viewer's source↔target toggle.
//
// Download (source + transformed) and a "your own files" drop entry round it
// out. The old Showcase / RealProof / OwnFiles panel-soup is gone — this single
// browser→viewer flow replaces it.

const TARGET_LOCALE = "fr-FR";

// Above this size we skip parsing an own-file for its grid thumbnail (it gets a
// plain file tile and is parsed lazily on open) — so we never load a big
// document into a tiny preview.
const THUMBNAIL_MAX_BYTES = 1_000_000;

// The curated sample set, one per recognizable structure. The three binary
// Office/markdown samples come from TRY_SAMPLES (real round-tripping bytes); the
// JSON catalog and HTML page come from the shared kit fixtures (UTF-8 text).
interface SampleSource {
  id: string;
  filename: string;
  bytes: () => Uint8Array;
}

const enc = new TextEncoder();

function textSample(id: string, fixtureName: string): SampleSource {
  const fx = getFixture(fixtureName);
  if (!fx) throw new Error(`unknown fixture: ${fixtureName}`);
  return { id, filename: fx.name, bytes: () => enc.encode(fx.content) };
}

const SAMPLE_SOURCES: SampleSource[] = [
  ...TRY_SAMPLES.map((s) => ({ id: s.id, filename: s.filename, bytes: s.bytes })),
  textSample("json", "messages.json"),
  textSample("html", "page.html"),
];

// A file ready to show in the viewer: the parsed source tree (from the boot
// inspect we already ran for the thumbnail), plus the original bytes for
// Download. `enriched` flips once the on-demand annotate + translate has run, so
// the tree then also carries overlays and a merged fr-FR target.
interface ViewerFile {
  id: string;
  filename: string;
  tree: ContentTree;
  bytes: Uint8Array;
  enriched?: boolean;
}

// Merge an output extraction's per-block source runs onto the source tree as a
// `targets[locale]` map, matched by block id (the extraction is deterministic so
// ids align). This makes the DocumentViewer's source↔target toggle a real
// before/after of the engine's transform.
function mergeTarget(source: ContentTree, output: ContentTree, locale: string): ContentTree {
  const outById = new Map<string, ContentTree["root"][number]>();
  const indexOut = (n: ContentTree["root"][number]): void => {
    if (n.kind === "block") outById.set(n.id, n);
    n.children?.forEach(indexOut);
  };
  output.root.forEach(indexOut);

  const clone = (n: ContentTree["root"][number]): ContentTree["root"][number] => {
    if (n.kind !== "block") {
      return { ...n, children: n.children?.map(clone) };
    }
    const out = outById.get(n.id);
    const targetRuns = out?.source;
    return {
      ...n,
      ...(targetRuns ? { targets: { ...n.targets, [locale]: targetRuns } } : {}),
    };
  };
  return { ...source, root: source.root.map(clone) };
}

// Run a real pseudo-translate to fr against `baseTree`, inspect the output, and
// merge the target runs back onto `baseTree`. This is the SLOW path — a full
// transform round-trip (a binary OOXML write for Office files) — so it runs in
// the background AFTER the document (and its overlays) are already on screen.
// Any failure returns null, so a slow or unsupported transform never blocks (or
// hangs) opening the file.
async function pseudoTargetTree(
  rt: LabRuntime,
  filename: string,
  bytes: Uint8Array,
  baseTree: ContentTree | null,
): Promise<ContentTree | null> {
  if (!baseTree) return null;
  try {
    rt.writeFile(filename, bytes);
    const out = `out-${filename}`;
    const code = await rt.run([
      "pseudo-translate",
      `/project/${filename}`,
      "-o",
      `/project/${out}`,
      "--target-lang",
      "fr",
    ]);
    if (code !== 0) return null;
    const outBytes = rt.readBytes(`/project/${out}`);
    if (!outBytes) return null;
    const outRes = await rt.inspect(out, outBytes);
    if (!outRes.ok || !outRes.tree) return null;
    return mergeTarget(baseTree, outRes.tree, TARGET_LOCALE);
  } catch {
    return null;
  }
}

export default function ModalBody(): React.ReactElement {
  const assets = useKapiPlaygroundConfig();
  // The modal owns the engine for its whole lifetime — booting on open is fine
  // (the page hero stays zero-wasm; only opening the modal pulls the engine).
  const runtime = useLabRuntime(assets);

  const [files, setFiles] = useState<BrowserFile[]>([]);
  const [busy, setBusy] = useState(true);
  const [opened, setOpened] = useState<ViewerFile | null>(null);
  const [openBusy, setOpenBusy] = useState(false);
  const [enriching, setEnriching] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  // Cache resolved bytes by browser-file id so opening doesn't re-decode.
  const bytesById = useRef(new Map<string, Uint8Array>());

  // On boot, inspect every sample into a browser thumbnail (structure only — the
  // grid thumbnails don't need annotations or a target).
  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    setBusy(true);
    void (async () => {
      const out: BrowserFile[] = [];
      for (const s of SAMPLE_SOURCES) {
        const bytes = s.bytes();
        bytesById.current.set(s.id, bytes);
        const res = await runtime.inspect(s.filename, bytes);
        if (cancelled) return;
        if (res.ok && res.tree) {
          out.push({ id: s.id, filename: s.filename, tree: res.tree, bytes });
          // Reveal the grid progressively as each sample is parsed, so the modal
          // shows a populated grid (not a long blank "extracting…") while the
          // remaining samples stream in.
          setFiles([...out]);
          setBusy(false);
        }
      }
      if (!cancelled) setBusy(false);
    })();
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime]);

  // Inspect a file then open it (used for large own-files that skipped the
  // thumbnail parse — a single fast read-only parse, with a spinner).
  const openByInspect = useCallback(
    async (id: string, filename: string, bytes: Uint8Array) => {
      if (!runtime.ready) return;
      setErr(null);
      setOpenBusy(true);
      try {
        const res = await runtime.inspect(filename, bytes);
        if (!res.ok || !res.tree) throw new Error(res.error ?? "inspect failed");
        setOpened({ id, filename, tree: res.tree, bytes });
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e));
      } finally {
        setOpenBusy(false);
      }
    },
    [runtime],
  );

  // Opening a thumbnail is INSTANT and does zero new engine work when the grid
  // already holds the parsed tree (every sample, and small own-files). A large
  // own-file carries no tree (skipped for the thumbnail), so it is parsed now.
  // Either way the heavy annotate + translate stays an explicit action (below).
  const onSelect = useCallback(
    (f: BrowserFile) => {
      const id = f.id ?? f.filename;
      const bytes = bytesById.current.get(id) ?? f.bytes ?? new Uint8Array();
      if (f.tree) {
        setErr(null);
        setOpened({ id, filename: f.filename, tree: f.tree, bytes });
        return;
      }
      void openByInspect(id, f.filename, bytes);
    },
    [openByInspect],
  );

  // Add the reader's own files as grid thumbnails WITHOUT navigating to the
  // detail view. Small files are parsed for a live thumbnail; files over the
  // thumbnail budget are added as plain tiles (parsed lazily on open) so we never
  // load a big document into a tiny preview.
  const addFiles = useCallback(
    async (incoming: File[]) => {
      if (!runtime.ready) return;
      setErr(null);
      for (const file of incoming) {
        const bytes = new Uint8Array(await file.arrayBuffer());
        const id = `own:${file.name}:${bytes.length}`;
        bytesById.current.set(id, bytes);
        let tree: ContentTree | undefined;
        if (bytes.length <= THUMBNAIL_MAX_BYTES) {
          const res = await runtime.inspect(file.name, bytes);
          if (res.ok && res.tree) tree = res.tree;
        }
        const bf: BrowserFile = { id, filename: file.name, tree, bytes };
        setFiles((cur) => (cur.some((f) => (f.id ?? f.filename) === id) ? cur : [...cur, bf]));
      }
    },
    [runtime],
  );

  // On-demand enrichment for the open file: run the read-only annotators
  // (overlays) then a real fr pseudo-translate target, merging both onto the
  // tree. Explicit + spinner-backed so the heavy work is observable and contained
  // — opening a file stays instant regardless.
  const enrich = useCallback(async () => {
    const cur = opened;
    if (!cur || !runtime.ready || enriching || cur.enriched) return;
    setErr(null);
    setEnriching(true);
    try {
      const annotated = await runtime.inspectAnnotated(cur.filename, cur.bytes);
      const annotatedTree = annotated.ok && annotated.tree ? annotated.tree : cur.tree;
      setOpened((o) => (o && o.id === cur.id ? { ...o, tree: annotatedTree } : o));
      const merged = await pseudoTargetTree(runtime, cur.filename, cur.bytes, annotatedTree);
      setOpened((o) =>
        o && o.id === cur.id ? { ...o, tree: merged ?? annotatedTree, enriched: true } : o,
      );
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setEnriching(false);
    }
  }, [opened, runtime, enriching]);

  const status = useMemo(() => {
    if (runtime.status === "error") return `Engine failed to start: ${runtime.error}`;
    if (!runtime.ready || busy) return "Extracting with the real engine…";
    return null;
  }, [runtime.status, runtime.error, runtime.ready, busy]);

  if (status) {
    return (
      <div className={styles.showcaseLoading}>
        <Loader2 size={16} aria-hidden="true" className="animate-spin" />
        {status}
      </div>
    );
  }

  // The detail view for one selected file.
  if (opened) {
    return (
      <div className={styles.body}>
        <div className={styles.detailBar}>
          <button type="button" className={styles.backBtn} onClick={() => setOpened(null)}>
            <ArrowLeft size={15} aria-hidden="true" /> All files
          </button>
          <span className={styles.detailHint} style={{ marginInlineEnd: "auto" }}>
            {opened.enriched
              ? "Vocabulary, brand & QA annotations · real pseudo-translate target"
              : "Live extraction — annotate & translate to see overlays and an fr target"}
          </span>
          <button
            type="button"
            className={styles.dropBtn}
            onClick={() => void enrich()}
            disabled={enriching || opened.enriched}
          >
            {enriching ? (
              <Loader2 size={15} aria-hidden="true" className="animate-spin" />
            ) : (
              <Sparkles size={15} aria-hidden="true" />
            )}
            {opened.enriched
              ? "Annotated & translated"
              : enriching
                ? "Running the engine…"
                : "Annotate & translate (fr)"}
          </button>
        </div>
        {err && <p className={styles.showcaseError}>Could not run: {err}</p>}
        <DocumentViewer tree={opened.tree} filename={opened.filename} bytes={opened.bytes} />
      </div>
    );
  }

  // The browser of all samples + the own-files "add" tile (in the grid itself).
  return (
    <div className={styles.body}>
      {err && <p className={styles.showcaseError}>Could not open file: {err}</p>}
      <div className={styles.browserHead}>
        <span className={styles.browserHint}>
          Pick a file to see kapi read it, annotate it, and transform it — or drop your own into the
          grid. Live in your browser.
        </span>
      </div>
      <FileBrowser files={files} defaultView="grid" onOpen={onSelect} onAddFiles={addFiles} />
      {openBusy && (
        <div className={styles.showcaseLoading}>
          <Loader2 size={16} aria-hidden="true" className="animate-spin" />
          Reading with the real engine…
        </div>
      )}
    </div>
  );
}
