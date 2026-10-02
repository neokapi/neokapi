import React from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { VisionExplorer } from "@site/src/components/Lab";
import { LabLaunch } from "@site/src/components/Lab/LabLaunch";
import { LabPageShell } from "@site/src/components/Lab/LabPageShell";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";

// The Vision Lab: upload an image (or use a bundled sample) and run the real
// kapi-vision models in your browser. Text comes from PP-OCRv5 (detection +
// recognition) and document layout from PP-DocLayoutV3 — the same ONNX models
// the native kapi-vision plugin runs, executed here via onnxruntime-web. Nothing
// is mocked: only the runtime differs (WebAssembly instead of native
// onnxruntime). The explorer is gated: no model is fetched until Run.

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
      title="Vision lab"
      description="Inspect text recognition and page layout in images using models running in your browser."
    >
      <LabPageShell
        title="Vision lab"
        lede={
          <>
            Inspect the text and positions recovered from a sample image. Compare the recognized
            words with the image, then examine the detected page regions. This experiment runs
            recognition models through browser bridges. Native kapi uses plugins to include these
            operations in a processing flow.
          </>
        }
      >
        <LabLaunch description="Open the image workspace to load the sample. Run recognition to download its models; layout analysis loads additional models when selected. Processing stays on your device.">
          <VisionExplorer samples={samples} modelBase={modelBase} />
        </LabLaunch>
      </LabPageShell>
    </Layout>
  );
}
