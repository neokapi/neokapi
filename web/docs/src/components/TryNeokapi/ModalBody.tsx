import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, Loader2, Upload } from "lucide-react";
import { TRY_SAMPLES, getFixture } from "@neokapi/kapi-playground";
import { useLabRuntime, type LabRuntime, type ContentTree } from "@neokapi/kapi-lab";
import FileBrowser, { type BrowserFile } from "@neokapi/kapi-lab/FileBrowser";
import DocumentViewer from "@neokapi/kapi-lab/DocumentViewer";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";
import styles from "./styles.module.css";

// The modal is ONE coherent surface: a FileBrowser of real sample files across
// formats (pptx · xlsx · md · json · html) backed by live extraction, and a
// DocumentViewer for the selected file. Everything is driven by the REAL kapi
// WASM engine booted on modal open:
//
//   • `inspectAnnotated` parses the file AND runs the read-only annotators
//     (vocabulary terms, brand violations, rule-based QA) so the viewer can
//     highlight them on the rendered document.
//   • a real pseudo-translate run produces an fr-FR target, which we merge back
//     onto the annotated source tree so the viewer's source↔target toggle (with
//     the typewriter / crossfade transition) reflects a genuine transform.
//
// Download (source + transformed) and a "your own files" drop entry round it
// out. The old Showcase / RealProof / OwnFiles panel-soup is gone — this single
// browser→viewer flow replaces it.

const TARGET_LOCALE = "fr-FR";

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

// A file ready to show in the viewer: the annotated source tree (with overlays
// AND a merged fr-FR target), plus the original bytes for Download.
interface ViewerFile {
  id: string;
  filename: string;
  tree: ContentTree;
  bytes: Uint8Array;
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
      ...(targetRuns ? { targets: { ...(n.targets ?? {}), [locale]: targetRuns } } : {}),
    };
  };
  return { ...source, root: source.root.map(clone) };
}

// Inspect a file with annotations on, run a real pseudo-translate to fr, inspect
// the output, and merge the target back. Returns a ViewerFile ready to render.
async function buildViewerFile(
  rt: LabRuntime,
  id: string,
  filename: string,
  bytes: Uint8Array,
): Promise<ViewerFile> {
  const annotated = await rt.inspectAnnotated(filename, bytes);
  if (!annotated.ok || !annotated.tree) {
    throw new Error(annotated.error ?? "inspect failed");
  }
  let tree = annotated.tree;

  // Real transform: pseudo-translate produces a deterministic fr-flavored
  // target through the canonical engine (no network, no LLM).
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
  if (code === 0) {
    const outBytes = rt.readBytes(`/project/${out}`);
    if (outBytes) {
      const outRes = await rt.inspect(out, outBytes);
      if (outRes.ok && outRes.tree) {
        tree = mergeTarget(tree, outRes.tree, TARGET_LOCALE);
      }
    }
  }
  return { id, filename, tree, bytes };
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
  const [err, setErr] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
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
        }
      }
      if (!cancelled) {
        setFiles(out);
        setBusy(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime]);

  const open = useCallback(
    async (id: string, filename: string, bytes: Uint8Array) => {
      if (!runtime.ready) return;
      setErr(null);
      setOpenBusy(true);
      try {
        const vf = await buildViewerFile(runtime, id, filename, bytes);
        setOpened(vf);
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e));
      } finally {
        setOpenBusy(false);
      }
    },
    [runtime],
  );

  const onSelect = useCallback(
    (f: BrowserFile) => {
      const bytes = bytesById.current.get(f.id ?? f.filename) ?? f.bytes ?? new Uint8Array();
      void open(f.id ?? f.filename, f.filename, bytes);
    },
    [open],
  );

  const onDrop = useCallback(
    async (file: File) => {
      const bytes = new Uint8Array(await file.arrayBuffer());
      const id = `own:${file.name}`;
      bytesById.current.set(id, bytes);
      void open(id, file.name, bytes);
    },
    [open],
  );

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
          <span className={styles.detailHint}>
            Live extraction · vocabulary, brand &amp; QA annotations · real pseudo-translate target
          </span>
        </div>
        <DocumentViewer tree={opened.tree} filename={opened.filename} bytes={opened.bytes} />
      </div>
    );
  }

  // The browser of all samples + the own-files entry.
  return (
    <div className={styles.body}>
      {err && <p className={styles.showcaseError}>Could not open file: {err}</p>}
      <div className={styles.browserHead}>
        <span className={styles.browserHint}>
          Pick a file to see kapi read it, annotate it, and transform it — live in your browser.
        </span>
        <button
          type="button"
          className={styles.dropBtn}
          onClick={() => fileInput.current?.click()}
          disabled={openBusy}
        >
          {openBusy ? (
            <Loader2 size={15} aria-hidden="true" className="animate-spin" />
          ) : (
            <Upload size={15} aria-hidden="true" />
          )}
          Try your own file
        </button>
        <input
          ref={fileInput}
          type="file"
          className={styles.srOnly}
          aria-label="Open one of your own files"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void onDrop(f);
            e.target.value = "";
          }}
        />
      </div>
      <FileBrowser files={files} defaultView="grid" onOpen={onSelect} />
      {openBusy && (
        <div className={styles.showcaseLoading}>
          <Loader2 size={16} aria-hidden="true" className="animate-spin" />
          Reading + transforming with the real engine…
        </div>
      )}
    </div>
  );
}
