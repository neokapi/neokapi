import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapters for the @neokapi/kapi-lab explorers. Each is client-only
// (the WASM runtime boots in the browser) and code-split (React.lazy) so the
// heavy lab chunk loads only on pages that embed an explorer. Asset URLs are
// resolved against the site base URL via the shared playground config.

const Loading = (): React.ReactElement => (
  <div style={{ padding: "1rem", color: "var(--ifm-color-emphasis-500)", fontStyle: "italic" }}>
    Loading the interactive lab…
  </div>
);

const LazyAnatomy = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.AnatomyExplorer };
});

const LazyPdf = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.PdfExplorer };
});

const LazyVision = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.VisionExplorer };
});

const LazyMultimodalShowcase = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.MultimodalShowcase };
});

// Segmentation preview is a docs-local component (it dynamic-imports the ICU4X
// `icu` package for the browser UAX-29 option, kept out of the shared kapi-lab).
const LazySegmentation = React.lazy(() => import("./SegmentationPreviewInner"));

const LazyPipeline = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.PipelineExplorer };
});

const LazyBatch = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.BatchExplorer };
});

const LazyConversion = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.ConversionExplorer };
});

const LazyToolDrop = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.ToolDropWidget };
});

const LazyPseudoTranslate = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.PseudoTranslateWidget };
});

const LazyStats = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.StatsWidget };
});

const LazySearchReplace = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.SearchReplaceWidget };
});

export interface AnatomyExplorerProps {
  defaultSampleId?: string;
  sampleIds?: string[];
}

export function AnatomyExplorer(props: AnatomyExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        // useBaseUrl (inside useKapiPlaygroundConfig) must run in a component.
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyAnatomy assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface PdfExplorerProps {
  sampleUrl?: string;
  sampleName?: string;
  samples?: { url: string; name?: string }[];
}

export function PdfExplorer(props: PdfExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyPdf assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface VisionExplorerProps {
  samples?: { url: string; name: string }[];
  modelBase?: string;
}

export function VisionExplorer(props: VisionExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => (
        <Suspense fallback={<Loading />}>
          <LazyVision {...props} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}

export interface MultimodalShowcaseProps {
  initialChapter?: number;
  className?: string;
}

export function MultimodalShowcase(props: MultimodalShowcaseProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => (
        <Suspense fallback={<Loading />}>
          <LazyMultimodalShowcase {...props} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}

export interface SegmentationPreviewProps {
  defaultSampleId?: string;
  sampleIds?: string[];
}

export function SegmentationPreview(props: SegmentationPreviewProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazySegmentation assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface PipelineExplorerProps {
  defaultSampleId?: string;
  defaultPipelineId?: string;
  sampleIds?: string[];
}

export function PipelineExplorer(props: PipelineExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyPipeline assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface BatchExplorerProps {
  sampleIds?: string[];
  defaultPattern?: string;
}

export function BatchExplorer(props: BatchExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyBatch assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface ConversionExplorerProps {
  defaultSampleId?: string;
  sampleIds?: string[];
  defaultTarget?: string;
}

export function ConversionExplorer(props: ConversionExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyConversion assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

// ── Drop-a-file widgets ──────────────────────────────────────────────────────
//
// No-terminal "drop a file → see the result" widgets for single-tool examples.
// Each is client-only + code-split, so a recipe page that embeds one pulls the
// lab chunk (and the WASM, on first run) only when the widget mounts.

export interface ToolDropWidgetProps {
  tool: string;
  buildArgv?: (inPath: string, outPath: string) => string[];
  recipe?: () => string;
  extraArgs?: string[];
  sampleIds?: string[];
  autoSampleId?: string;
  acceptBinary?: boolean;
  render?: "output" | "stat" | "diff";
  autoRun?: boolean;
}

export function ToolDropWidget(props: ToolDropWidgetProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyToolDrop assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface WidgetVariantProps {
  sampleIds?: string[];
  autoSampleId?: string;
  /** A file to load on first render instead of a sample (e.g. one dropped on the hero). */
  initialInput?: { name: string; bytes: Uint8Array; binary: boolean } | null;
}

export function PseudoTranslateWidget(props: WidgetVariantProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyPseudoTranslate assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export function StatsWidget(props: WidgetVariantProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyStats assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface SearchReplaceWidgetProps extends WidgetVariantProps {
  defaultFind?: string;
  defaultReplace?: string;
}

export function SearchReplaceWidget(props: SearchReplaceWidgetProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazySearchReplace assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
