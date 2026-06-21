import React from "react";
import Layout from "@theme/Layout";
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
      description="Pull the text out of an image or scan — and see where every line sits on the page — so it can be searched, translated, or rebuilt in another language. Runs privately in your browser."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Vision Lab</h1>
          <p className={styles.lede}>
            Drop in an image — or a document with a picture in it — and neokapi reads the text
            inside it, keeping track of where each line sits and how the page is laid out (headings,
            paragraphs, tables, figures). The result isn&rsquo;t just a wall of text: it&rsquo;s
            structured content you can search, translate, and place back, the same way you would
            with any document. Everything runs in your browser, so nothing you upload leaves your
            device.
          </p>
        </div>
        <VisionExplorer samples={samples} modelBase={modelBase} />
      </main>
    </Layout>
  );
}
