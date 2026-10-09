import React from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import type { ResolvedStage, StageContext } from "@neokapi/kapi-learn/player";
import { ChunkSafeSuspense } from "../ChunkErrorBoundary";
import { lazyWithRetry } from "../../lib/chunkReload";
import { RECORDED_TRACES } from "../Lab/FlowBuilderRunner";

// The explorers an explorer lab can put on its stage, by id, and how a
// chapter's logical props (a sample name, a scenario, a target format) become
// the component's props: the site's sample URLs, the model base, the recorded
// traces. Each explorer loads as its own chunk the first time a lab shows it.

const Loading = (): React.ReactElement => (
  <div
    role="status"
    aria-live="polite"
    style={{ padding: "1rem", color: "var(--ifm-color-emphasis-500)", fontStyle: "italic" }}
  >
    Loading the explorer…
  </div>
);

const LazyFlow = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).FlowBuilderRunner,
}));
const LazyConversion = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).ConversionExplorer,
}));
const LazyPdf = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).PdfExplorer,
}));
const LazyVision = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).VisionExplorer,
}));
const LazyAudio = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).AudioExplorer,
}));
const LazyVideo = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).VideoExplorer,
}));
const LazyMultimodal = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).MultimodalShowcase,
}));
const LazyKbfExplorer = lazyWithRetry(async () => ({
  default: (await import("@neokapi/kapi-lab")).KbfExplorer,
}));
const LazySegmentation = lazyWithRetry(() => import("../Lab/SegmentationLabInner"));
const LazyKbfAnatomy = lazyWithRetry(() => import("../Lab/KbfAnatomy"));

/** The vision samples a chapter names, and the files they are. */
const VISION_FILES: Record<string, string> = {
  document: "vision-doc.png",
  hello: "vision-hello.png",
  handwriting: "vision-handwriting.png",
  "report.docx": "embedded-image.docx",
};

function str(v: unknown): string | undefined {
  return typeof v === "string" ? v : undefined;
}

function list(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

/** The ONNX vision models' base URL: same-origin staging or the CDN, as the vision page resolves it. */
function useModelBase(): string {
  const { i18n, siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const localeModels = useBaseUrl("/models/vision");
  const sameOriginBase =
    i18n.currentLocale === i18n.defaultLocale
      ? localeModels
      : localeModels.replace(`/${i18n.currentLocale}/`, "/");
  return cdnEnabled(cdn) ? cdnHref(cdn, `/models/vision/${cdn.modelsVersion}`) : sameOriginBase;
}

function useClipUrl(): string {
  const { siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const local = useBaseUrl("/video/samples/multimodal-sample.mp4");
  return cdnEnabled(cdn) ? cdnHref(cdn, "/video/samples/multimodal-sample.mp4") : local;
}

function ExplorerStage({ stage, ctx }: { stage: ResolvedStage; ctx: StageContext }) {
  const samplesBase = useBaseUrl("/samples/");
  const tracesBase = useBaseUrl("/data/traces/");
  const modelBase = useModelBase();
  const clipUrl = useClipUrl();
  const p = stage.props;

  let body: React.ReactNode;
  switch (stage.explorer) {
    case "flow":
      body = (
        <LazyFlow
          assets={ctx.assets}
          defaultScenarioId={str(p.scenario)}
          recordedTraces={RECORDED_TRACES.map((t) => ({
            name: t.name,
            description: t.description,
            url: tracesBase + t.path.split("/").pop(),
          }))}
          autoStart
        />
      );
      break;
    case "segmentation":
      body = (
        <LazySegmentation
          assets={ctx.assets}
          defaultSampleId={str(p.sample)}
          autoRun={p.autoRun === true}
        />
      );
      break;
    case "conversion":
      body = (
        <LazyConversion
          assets={ctx.assets}
          defaultSampleId={str(p.sample)}
          defaultTarget={str(p.target)}
          samples={list(p.files).map((name) => ({ url: samplesBase + name, name }))}
          autoStart
        />
      );
      break;
    case "pdf":
      body = (
        <LazyPdf
          assets={ctx.assets}
          samples={list(p.samples).map((name) => ({ url: samplesBase + name, name }))}
          autoStart
        />
      );
      break;
    case "vision":
      body = (
        <LazyVision
          samples={list(p.samples)
            .filter((name) => VISION_FILES[name])
            .map((name) => ({ url: samplesBase + VISION_FILES[name], name }))}
          modelBase={modelBase}
          autoStart
        />
      );
      break;
    case "audio":
      body = <LazyAudio samples={[{ url: clipUrl, name: "sample clip" }]} />;
      break;
    case "video":
      body = <LazyVideo samples={[{ url: clipUrl, name: "sample clip" }]} modelBase={modelBase} />;
      break;
    case "multimodal":
      body = <LazyMultimodal initialChapter={typeof p.chapter === "number" ? p.chapter : 0} />;
      break;
    case "kbf-anatomy":
      body = <LazyKbfAnatomy part={str(p.term) as never} />;
      break;
    case "kbf-explorer":
      body = <LazyKbfExplorer assets={ctx.assets} defaultSampleId={str(p.sample)} autoStart />;
      break;
    default:
      body = (
        <p style={{ padding: "1rem" }}>
          No explorer is registered as <code>{stage.explorer}</code>.
        </p>
      );
  }
  return <ChunkSafeSuspense fallback={<Loading />}>{body}</ChunkSafeSuspense>;
}

/** The stage renderer the explorer player calls, one element per resolved stage. */
export function renderStage(stage: ResolvedStage, ctx: StageContext): React.ReactNode {
  return <ExplorerStage stage={stage} ctx={ctx} />;
}
