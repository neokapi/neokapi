import React, { useEffect, useState } from "react";
import { ArrowRight, Loader2 } from "lucide-react";
import { TRY_SAMPLES, type TrySample } from "@neokapi/kapi-playground";
import DocumentRender from "@neokapi/kapi-lab/DocumentRender";
import { treeToRenderDoc, type RenderDoc } from "@neokapi/kapi-lab/renderDoc";
import type { LabRuntime } from "@neokapi/kapi-lab";
import { buildSearchReplaceRecipe } from "@neokapi/kapi-lab";
import type { DemoId } from "./demos";
import styles from "./styles.module.css";

// The LIVE showcase: for each real sample (deck.pptx · report.xlsx · guide.md)
// it boots the kapi WASM engine, inspects the source bytes into a RenderDoc,
// runs the active demo's REAL transform through the canonical Go engine, then
// inspects the output bytes into a target RenderDoc — and paints both through
// the shared DocumentRender, highlighting exactly what the pipeline changed.
//
// This replaces the old faked mock documents: every slide, cell, and paragraph
// here is what the engine actually extracted, and every highlighted word is a
// real change the engine wrote back into the file.

interface ShowcaseProps {
  runtime: LabRuntime;
  demo: DemoId;
  find: string;
  replace: string;
}

// Per-sample extraction result.
interface SampleView {
  sample: TrySample;
  source: RenderDoc | null;
  target: RenderDoc | null;
  stats: { blocks: number; words: number; characters: number } | null;
  error: string | null;
}

function recipeFor(demo: DemoId, find: string, replace: string): string | null {
  if (demo === "search-replace") return buildSearchReplaceRecipe(find, replace, false);
  return null; // pseudo/insights run via a dedicated command / no transform
}

function countStats(doc: RenderDoc): { blocks: number; words: number; characters: number } {
  const texts: string[] = [];
  doc.slides?.forEach((s) => {
    if (s.title) texts.push(s.title.text);
    s.bullets.forEach((b) => texts.push(b.text));
  });
  (doc.sheets ?? (doc.sheet ? [doc.sheet] : [])).forEach((sh) =>
    sh.cells.forEach((c) => texts.push(c.text)),
  );
  doc.paragraphs?.forEach((p) => texts.push(p.text));
  doc.lines?.forEach((l) => texts.push(l.text));
  let words = 0;
  let characters = 0;
  for (const t of texts) {
    const trimmed = t.trim();
    words += trimmed ? trimmed.split(/\s+/).length : 0;
    characters += t.length;
  }
  return { blocks: texts.length, words, characters };
}

// Run the active demo's transform on one sample through the real engine, then
// inspect both source and output into render models.
async function runSample(
  rt: LabRuntime,
  sample: TrySample,
  demo: DemoId,
  find: string,
  replace: string,
): Promise<SampleView> {
  const bytes = sample.bytes();
  const srcRes = await rt.inspect(sample.filename, bytes);
  if (!srcRes.ok || !srcRes.tree) {
    return {
      sample,
      source: null,
      target: null,
      stats: null,
      error: srcRes.error ?? "inspect failed",
    };
  }
  const source = treeToRenderDoc(srcRes.tree);
  const stats = countStats(source);

  if (demo === "insights") {
    return { sample, source, target: null, stats, error: null };
  }

  // Seed source + recipe, run the engine, read the transformed output, inspect.
  rt.writeFile(sample.filename, bytes);
  const out = `out-${sample.filename}`;
  let code: number;
  if (demo === "pseudo") {
    code = await rt.run([
      "pseudo-translate",
      `/project/${sample.filename}`,
      "-o",
      `/project/${out}`,
    ]);
  } else {
    const recipe = recipeFor(demo, find, replace);
    if (!recipe) return { sample, source, target: null, stats, error: "unsupported demo" };
    rt.writeFile("try.kapi", recipe);
    code = await rt.run([
      "run",
      "lab",
      "-p",
      "/project/try.kapi",
      "-i",
      `/project/${sample.filename}`,
      "-o",
      `/project/${out}`,
      "--target-lang",
      "fr",
    ]);
  }
  if (code !== 0) {
    return { sample, source, target: null, stats, error: `engine exited ${code}` };
  }
  const outBytes = rt.readBytes(`/project/${out}`);
  if (!outBytes) {
    return { sample, source, target: null, stats, error: "no output produced" };
  }
  const outRes = await rt.inspect(out, outBytes);
  const target = outRes.ok && outRes.tree ? treeToRenderDoc(outRes.tree) : null;
  return {
    sample,
    source,
    target,
    stats,
    error: outRes.ok ? null : (outRes.error ?? "inspect failed"),
  };
}

function StatChips({ stats }: { stats: { blocks: number; words: number; characters: number } }) {
  return (
    <div className={styles.insights}>
      <span className={styles.statChip}>
        <span className={styles.statNum}>{stats.blocks}</span>
        <span className={styles.statLabel}>blocks</span>
      </span>
      <span className={styles.statChip}>
        <span className={styles.statNum}>{stats.words}</span>
        <span className={styles.statLabel}>words</span>
      </span>
      <span className={styles.statChip}>
        <span className={styles.statNum}>{stats.characters}</span>
        <span className={styles.statLabel}>chars</span>
      </span>
    </div>
  );
}

function barClass(id: string): string {
  if (id === "pptx") return styles.barPptx;
  if (id === "xlsx") return styles.barXlsx;
  return styles.barMd;
}

export default function Showcase({
  runtime,
  demo,
  find,
  replace,
}: ShowcaseProps): React.ReactElement {
  const [views, setViews] = useState<SampleView[]>([]);
  const [busy, setBusy] = useState(true);

  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    setBusy(true);
    void (async () => {
      const out: SampleView[] = [];
      for (const sample of TRY_SAMPLES) {
        const v = await runSample(runtime, sample, demo, find, replace);
        if (cancelled) return;
        out.push(v);
      }
      if (!cancelled) {
        setViews(out);
        setBusy(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime, demo, find, replace]);

  if (!runtime.ready || (busy && views.length === 0)) {
    return (
      <div className={styles.showcaseLoading}>
        <Loader2 size={16} aria-hidden="true" className="animate-spin" />
        {runtime.status === "error"
          ? `Engine failed to start: ${runtime.error}`
          : "Extracting with the real engine…"}
      </div>
    );
  }

  return (
    <div className={styles.grid} aria-busy={busy}>
      {views.map((v) => (
        <div key={v.sample.id} className={styles.doc}>
          <div className={`${styles.docBar} ${barClass(v.sample.id)}`}>
            {v.source?.kind === "slides"
              ? "PowerPoint slide"
              : v.source?.kind === "sheet"
                ? "Excel worksheet"
                : "Markdown doc"}
            <span className={styles.docName}>{v.sample.filename}</span>
          </div>
          <div className={styles.docContent}>
            {v.error ? (
              <p className={styles.showcaseError}>{v.error}</p>
            ) : demo === "insights" ? (
              <>
                {v.source && <DocumentRender doc={v.source} gridHeaders={false} />}
                {v.stats && <StatChips stats={v.stats} />}
              </>
            ) : v.source && v.target ? (
              <div className={styles.beforeAfter}>
                <div className={styles.beforeAfterCol}>
                  <span className={styles.beforeAfterTag}>EN</span>
                  <DocumentRender doc={v.source} gridHeaders={false} />
                </div>
                <ArrowRight className={styles.beforeAfterArrow} size={16} aria-hidden="true" />
                <div className={styles.beforeAfterCol}>
                  <span className={styles.beforeAfterTag}>
                    {demo === "pseudo" ? "Pseudo" : "Result"}
                  </span>
                  <DocumentRender doc={v.target} before={v.source} gridHeaders={false} />
                </div>
              </div>
            ) : (
              v.source && <DocumentRender doc={v.source} gridHeaders={false} />
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
