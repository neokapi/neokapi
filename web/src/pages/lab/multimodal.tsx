import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { MultimodalShowcase } from "@site/src/components/Lab";
import styles from "./pdf.module.css";

// The Multimodal Showcase: a pre-recorded (canned-data) walkthrough of the
// multimodal localization story — image OCR, audio subtitles, and video — that
// plays instantly with no model download, no ffmpeg, no engine. The reliable
// companion to the live in-browser labs (/lab/audio, /lab/video, /lab/vision):
// the extraction results are baked in (mirroring `kapi inspect`), and a simulated
// playhead animates the subtitle highlight + frame-OCR overlay exactly as they
// would over real playback.

export default function MultimodalLabPage(): React.ReactElement {
  return (
    <Layout
      title="Multimodal Showcase"
      description="A pre-recorded walkthrough of multimodal localization — image OCR, audio subtitles, and video — playing instantly in your browser with no engine or model download."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Multimodal Showcase</h1>
          <p className={styles.lede}>
            A guided tour of how kapi localizes <strong>images</strong>, <strong>audio</strong>, and{" "}
            <strong>video</strong> — translating the text inside each asset and rendering the
            result. This showcase is <strong>pre-recorded</strong>: the extraction (OCR, speech
            recognition, demux) is baked in, so it plays anywhere, instantly, with no model download
            or ffmpeg. To run the real engines in your browser, see the live{" "}
            <Link to="/lab/vision">Vision Lab</Link> (and the audio/video labs).
          </p>
          <nav className={styles.nav} aria-label="Related labs">
            <Link to="/lab">Lab</Link>
            <Link to="/lab/vision">Vision Lab</Link>
            <Link to="/contribute/architecture/030-multimodal-extraction-and-llm-refinement">
              Multimodal extraction &amp; refinement
            </Link>
          </nav>
        </div>
        <MultimodalShowcase />
      </main>
    </Layout>
  );
}
