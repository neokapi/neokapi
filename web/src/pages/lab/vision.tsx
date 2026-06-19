import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import { VisionExplorer } from "@site/src/components/Lab";
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
    { url: useBaseUrl("/samples/embedded-image.docx"), name: "report.docx" },
  ];
  // Models are served same-origin (staged into web/static/models/vision at
  // docs build): GitHub release download URLs are CORS-blocked for browser fetch.
  const modelBase = useBaseUrl("/models/vision");
  return (
    <Layout
      title="Vision Lab"
      description="Upload an image and run the real kapi-vision models in your browser — text via PP-OCRv5 OCR and document layout via PP-DocLayoutV3, executed with onnxruntime-web. Nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Vision Lab</h1>
          <p className={styles.lede}>
            Drop in an image and the <Link to="/lab">Lab</Link> runs the real{" "}
            <code>kapi-vision</code> models right here in your browser. Text is detected and
            recognized by <strong>PP-OCRv5</strong>; document <strong>layout</strong> (headings,
            paragraphs, tables, figures) comes from <strong>PP-DocLayoutV3</strong> — the same ONNX
            models the native plugin runs, executed via <strong>onnxruntime-web</strong>. The OCR
            models (~21 MB) load on first use; the layout model (~132 MB) downloads only when you
            ask for it. You can also drop in a <strong>.docx</strong> — the embedded image is pulled
            straight from the document and run through the same models. Nothing is mocked — only the
            runtime differs.
          </p>
          <nav className={styles.nav} aria-label="Related labs">
            <Link to="/lab">Lab</Link>
            <Link to="/lab/pdf">PDF Lab</Link>
            <Link to="/contribute/architecture/029-vision-and-image-localization">Vision &amp; image localization</Link>
          </nav>
        </div>
        <VisionExplorer samples={samples} modelBase={modelBase} />
      </main>
    </Layout>
  );
}
