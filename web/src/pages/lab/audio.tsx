import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { AudioExplorer } from "@site/src/components/Lab";
import styles from "./pdf.module.css";

// The Audio Lab: drop in an audio file and run real Whisper speech recognition in
// your browser via @huggingface/transformers (onnxruntime-web) — the same Whisper
// family the native kapi-asr plugin runs through whisper.cpp. The recognized
// segments become timing-anchored subtitle cues, the shape the audio-to-subtitles
// flow translates and writes to .vtt/.srt. Nothing is mocked — only the runtime
// differs (WebAssembly here, native there).

export default function AudioLabPage(): React.ReactElement {
  return (
    <Layout
      title="Audio Lab"
      description="Drop in an audio file and run real Whisper speech recognition in your browser — the same model family the native kapi-asr plugin runs, executed via onnxruntime-web. Nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Audio Lab</h1>
          <p className={styles.lede}>
            Drop in an audio clip and the <Link to="/lab">Lab</Link> runs real{" "}
            <strong>Whisper</strong> speech recognition right here in your browser via{" "}
            <strong>onnxruntime-web</strong> — the same Whisper family the native{" "}
            <code>kapi-asr</code> plugin runs through whisper.cpp. The recognized speech becomes{" "}
            timing-anchored <strong>subtitle cues</strong>: play the audio and the active cue
            highlights. That's exactly the shape the <code>audio-to-subtitles</code> flow translates
            and writes to <code>.vtt</code>/<code>.srt</code>. The model (~40&nbsp;MB) loads on first
            use; nothing is mocked — only the runtime differs. For an instant, no-download tour, see
            the <Link to="/lab/multimodal">Multimodal Showcase</Link>.
          </p>
          <nav className={styles.nav} aria-label="Related labs">
            <Link to="/lab">Lab</Link>
            <Link to="/lab/video">Video Lab</Link>
            <Link to="/lab/multimodal">Multimodal Showcase</Link>
            <Link to="/contribute/architecture/030-multimodal-extraction-and-llm-refinement">
              Multimodal extraction &amp; refinement
            </Link>
          </nav>
        </div>
        <AudioExplorer />
      </main>
    </Layout>
  );
}
