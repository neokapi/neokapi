import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { VideoExplorer } from "@site/src/components/Lab";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import styles from "./pdf.module.css";

// The Video Lab: drop in a video and the whole multimodal pipeline runs in your
// browser — ffmpeg.wasm demuxes it into an audio track + sampled frames, Whisper
// transcribes the speech into subtitle cues, and PP-OCRv5 reads the on-screen
// frame text. The same engines the native kapi-av / kapi-asr / kapi-vision
// plugins run, only the runtime differs (WebAssembly here, native there).

export default function VideoLabPage(): React.ReactElement {
  // OCR models are served same-origin (staged at docs build), or from the CDN
  // when configured — the same resolution the Vision Lab uses.
  const { i18n, siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const localizedModels = useBaseUrl("/models/vision");
  const sameOriginBase =
    i18n.currentLocale === i18n.defaultLocale
      ? localizedModels
      : localizedModels.replace(`/${i18n.currentLocale}/`, "/");
  const modelBase = cdnEnabled(cdn) ? cdnHref(cdn, "/models/vision") : sameOriginBase;
  // The sample clip is a video asset (gitignored web/static/video, published to
  // the docs-assets release + CDN), so serve it from the CDN when configured.
  const sampleLocal = useBaseUrl("/video/samples/multimodal-sample.mp4");
  const sampleUrl = cdnEnabled(cdn)
    ? cdnHref(cdn, "/video/samples/multimodal-sample.mp4")
    : sampleLocal;
  const samples = [{ url: sampleUrl, name: "sample clip" }];

  return (
    <Layout
      title="Video Lab"
      description="Drop in a video and run the whole multimodal pipeline in your browser — ffmpeg.wasm demux, Whisper speech recognition, and PP-OCRv5 on-screen text. The same engines the native plugins run; nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Video Lab</h1>
          <p className={styles.lede}>
            Drop in a video and the <Link to="/lab">Lab</Link> runs the whole multimodal pipeline in
            your browser. <strong>ffmpeg.wasm</strong> demuxes it into an audio track and sampled
            frames; <strong>Whisper</strong> transcribes the speech into a subtitle track; and{" "}
            <strong>PP-OCRv5</strong> reads the on-screen frame text, overlaid at its timecode — the
            same engines the native <code>kapi-av</code> / <code>kapi-asr</code> /{" "}
            <code>kapi-vision</code> plugins run, only the runtime differs. The ffmpeg core
            (~32&nbsp;MB) and the Whisper model (~40&nbsp;MB) load on first use. Nothing is mocked.
            For an instant, no-download tour, see the{" "}
            <Link to="/lab/multimodal">Multimodal Showcase</Link>.
          </p>
          <nav className={styles.nav} aria-label="Related labs">
            <Link to="/lab">Lab</Link>
            <Link to="/lab/audio">Audio Lab</Link>
            <Link to="/lab/vision">Vision Lab</Link>
            <Link to="/lab/multimodal">Multimodal Showcase</Link>
          </nav>
        </div>
        <VideoExplorer samples={samples} modelBase={modelBase} />
      </main>
    </Layout>
  );
}
