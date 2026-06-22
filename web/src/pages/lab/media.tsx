import React, { useState } from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { AudioExplorer, VideoExplorer } from "@site/src/components/Lab";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import styles from "./pdf.module.css";

// Audio & Video lab: the multimodal localization pipeline in your browser.
// Audio — Whisper speech recognition (onnxruntime-web, the kapi-asr model
// family) turns speech into timing-anchored subtitle cues. Video — ffmpeg.wasm
// (kapi-av) demuxes the clip into an audio track + sampled frames, Whisper
// transcribes the speech, and PP-OCRv5 (kapi-vision) reads on-screen text. The
// same engines the native plugins run; only the runtime differs. Each tab is
// button-gated: nothing downloads or runs until you press it.

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
  const modelBase = cdnEnabled(cdn) ? cdnHref(cdn, `/models/vision/${cdn.modelsVersion}`) : sameOriginBase;

  const [tab, setTab] = useState<Tab>("audio");

  return (
    <Layout
      title="Audio & Video Lab"
      description="Turn what people say and what appears on screen into time-stamped, translatable captions — ready to subtitle a clip in another language. Runs privately on your device; nothing is uploaded."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Audio &amp; Video</h1>
          <p className={styles.lede}>
            Localize what people <em>say</em> and what appears <em>on screen</em>. Add an audio or
            video file and neokapi turns the speech — and any on-screen text — into time-stamped
            captions you can review, translate, and play back in sync. It runs entirely on your
            device, so nothing is uploaded, and each tab only fetches what it needs the first time
            you press play.
          </p>
        </div>

        <div className={styles.tabs} role="tablist" aria-label="Audio or video">
          <button
            type="button"
            role="tab"
            aria-selected={tab === "audio"}
            className={tab === "audio" ? styles.tabActive : styles.tab}
            onClick={() => setTab("audio")}
          >
            Audio
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === "video"}
            className={tab === "video" ? styles.tabActive : styles.tab}
            onClick={() => setTab("video")}
          >
            Video
          </button>
        </div>

        {tab === "audio" ? (
          <AudioExplorer samples={samples} />
        ) : (
          <VideoExplorer samples={samples} modelBase={modelBase} />
        )}
      </main>
    </Layout>
  );
}
