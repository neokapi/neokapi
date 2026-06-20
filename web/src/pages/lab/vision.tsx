import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { VisionExplorer } from "@site/src/components/Lab";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import styles from "./pdf.module.css";

// The Vision Lab: upload an image (or use a bundled sample) and run the real
// kapi-vision models in your browser. Text comes from PP-OCRv5 (detection +
// recognition) and document layout from PP-DocLayoutV3 — the same ONNX models
// the native kapi-vision plugin runs, executed here via onnxruntime-web. Nothing
// is mocked: only the runtime differs (WebAssembly instead of native onnxruntime).

export default function VisionLabPage(): React.ReactElement {
  const samples = [
    { url: useBaseUrl("/samples/vision-doc.png"), name: "document" },
    { url: useBaseUrl("/samples/vision-hello.png"), name: "hello" },
    { url: useBaseUrl("/samples/vision-handwriting.png"), name: "handwriting" },
    { url: useBaseUrl("/samples/embedded-image.docx"), name: "report.docx" },
  ];
  // Models are served same-origin (staged into web/static/models/vision at docs
  // build): GitHub release URLs are CORS-blocked for browser fetch. They are
  // deduplicated to the default-locale (root) output (docusaurus.config
  // dropLocaleVisionModels), so strip any locale segment to fetch that single
  // copy — a no-op when useBaseUrl isn't locale-prefixed (default locale).
  //
  // When a CDN origin is configured (cdnBaseUrl customField, from $DOCS_CDN_URL)
  // the models are served (CORS-enabled, whole — no GitHub-Pages size split) from
  // the CDN instead, bypassing the same-origin staging and per-locale dedup. The
  // CDN path is versioned (/models/vision/<modelsVersion>/, pinned in
  // web/models.version) so a PR can point at a different model set by bumping
  // that file — its preview then loads the matching set.
  const { i18n, siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const localizedModels = useBaseUrl("/models/vision");
  const sameOriginBase =
    i18n.currentLocale === i18n.defaultLocale
      ? localizedModels
      : localizedModels.replace(`/${i18n.currentLocale}/`, "/");
  const modelBase = cdnEnabled(cdn)
    ? cdnHref(cdn, `/models/vision/${cdn.modelsVersion}`)
    : sameOriginBase;
  return (
    <Layout
      title="Vision Lab"
      description="Upload an image and run the real kapi-vision models in your browser — text via PP-OCRv5 OCR and document layout via PP-DocLayoutV3, executed with onnxruntime-web. Nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Vision Lab</h1>
          <p className={styles.lede}>
            Drop in an image and the <Link to="/lab">Core Framework</Link> runs the real{" "}
            <code>kapi-vision</code> models right here in your browser. Text is detected and
            recognized by <strong>PP-OCRv5</strong>; document <strong>layout</strong> (headings,
            paragraphs, tables, figures) comes from <strong>PP-DocLayoutV3</strong> — the same ONNX
            models the native plugin runs, executed via <strong>onnxruntime-web</strong>. The OCR
            models (~21 MB) load on first use; the layout model (~132 MB) downloads only when you
            ask for it. You can also drop in a <strong>.docx</strong> — the embedded image is pulled
            straight from the document and run through the same models. Toggle{" "}
            <strong>handwriting fallback</strong> to re-read low-confidence lines with TrOCR (loaded
            on demand): PP-OCR handles clean text fast, TrOCR rescues the hard lines. Add a third{" "}
            <strong>local-LLM tier</strong> (<code>🧠 Local LLM</code>) to re-read the
            still-uncertain residual with a vision model on your own machine via{" "}
            <strong>Ollama</strong> — keyless and on-device, no key to paste and nothing leaves your
            browser (start Ollama with this origin allowed). Nothing is mocked — only the runtime
            differs.
          </p>
        </div>
        <VisionExplorer samples={samples} modelBase={modelBase} />
      </main>
    </Layout>
  );
}
