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
  const modelBase = cdnEnabled(cdn) ? cdnHref(cdn, "/models/vision") : sameOriginBase;

  const [tab, setTab] = useState<Tab>("audio");

  return (
    <Layout
      title="Audio & Video Lab"
      description="Run the multimodal localization pipeline in your browser — Whisper speech recognition for audio, and ffmpeg.wasm demux + Whisper + PP-OCRv5 for video. The same engines the native plugins run; nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Audio &amp; Video</h1>
          <p className={styles.lede}>
            Localize what people <em>say</em> and what appears <em>on screen</em>. The{" "}
            <strong>Audio</strong> tab runs real <strong>Whisper</strong> speech recognition (via{" "}
            <strong>onnxruntime-web</strong>, the same model family the native <code>kapi-asr</code>{" "}
            plugin runs through whisper.cpp), turning speech into timing-anchored subtitle cues. The{" "}
            <strong>Video</strong> tab runs the whole pipeline: <strong>ffmpeg.wasm</strong> (
            <code>kapi-av</code>) demuxes the clip, Whisper transcribes the speech, and{" "}
            <strong>PP-OCRv5</strong> (<code>kapi-vision</code>) reads the on-screen frame text.
            Only the runtime differs from the desktop plugins; nothing is mocked. Each tab loads its
            model on first run — nothing is fetched until you press play.
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
