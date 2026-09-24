import React, { useState } from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { AudioExplorer, VideoExplorer } from "@site/src/components/Lab";
import { LabLaunch } from "@site/src/components/Lab/LabLaunch";
import { LabPageShell } from "@site/src/components/Lab/LabPageShell";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";

// Browser recognition bridges recover speech and on-screen text as timed
// content. Native kapi integrates the corresponding operations through plugins.
// Each explorer requests models only when its processing action is invoked.

type Tab = "audio" | "video";

export default function MediaLabPage(): React.ReactElement {
  const { i18n, siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);

  // The shared sample clip (a video asset served from the CDN when configured,
  // else the staged web/static/video copy). The audio tab decodes its audio
  // track; the video tab demuxes the whole thing.
  const sampleLocal = useBaseUrl("/video/samples/multimodal-sample.mp4");
  const sampleUrl = cdnEnabled(cdn)
    ? cdnHref(cdn, "/video/samples/multimodal-sample.mp4")
    : sampleLocal;
  const samples = [{ url: sampleUrl, name: "sample clip" }];

  // OCR models for the video tab: same-origin (staged at docs build) or CDN.
  const localizedModels = useBaseUrl("/models/vision");
  const sameOriginBase =
    i18n.currentLocale === i18n.defaultLocale
      ? localizedModels
      : localizedModels.replace(`/${i18n.currentLocale}/`, "/");
  const modelBase = cdnEnabled(cdn)
    ? cdnHref(cdn, `/models/vision/${cdn.modelsVersion}`)
    : sameOriginBase;

  const [tab, setTab] = useState<Tab>("audio");
  const tabClass = (active: boolean): string =>
    active
      ? "rounded-lg bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground"
      : "rounded-lg px-4 py-1.5 text-sm text-muted-foreground hover:bg-muted/60";

  return (
    <Layout
      title="Audio and video"
      description="Inspect speech recognition and on-screen text as timed content in audio and video."
    >
      <LabPageShell
        title="Audio and video"
        lede={
          <>
            Compare speech and on-screen text with the timed content recovered from a clip. Check
            wording and timestamps against playback. These experiments run recognition and media
            processing through browser bridges; native kapi integrates these operations through
            plugins.
          </>
        }
        heroExtra={
          <div
            className="mt-4 inline-flex gap-1 rounded-xl border p-1"
            role="group"
            aria-label="Audio or video"
          >
            <button
              type="button"
              aria-pressed={tab === "audio"}
              className={tabClass(tab === "audio")}
              onClick={() => setTab("audio")}
            >
              Audio
            </button>
            <button
              type="button"
              aria-pressed={tab === "video"}
              className={tabClass(tab === "video")}
              onClick={() => setTab("video")}
            >
              Video
            </button>
          </div>
        }
      >
        <LabLaunch
          key={tab}
          label={`Open ${tab} experiment`}
          description="Open the workspace to select a clip. Transcribe or Process downloads the required models and media tools on first use. Downloads can be substantial; progress appears in the workspace. Processing stays on your device. Switching between audio and video closes the current experiment."
        >
          {tab === "audio" ? (
            <AudioExplorer samples={samples} />
          ) : (
            <VideoExplorer samples={samples} modelBase={modelBase} />
          )}
        </LabLaunch>
      </LabPageShell>
    </Layout>
  );
}
