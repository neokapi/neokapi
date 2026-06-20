import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { GemmaExplorer } from "@site/src/components/Lab";
import styles from "./pdf.module.css";

// The Gemma Lab: run `kapi translate --provider gemma` entirely in your
// browser. The kapi WASM engine drives the AI tool, and Google's Gemma 4 (E2B)
// model itself runs via transformers.js on WebGPU — the same ONNX model the
// native kapi-llm plugin runs in-process. Nothing is mocked and nothing is sent
// to a server; only the runtime differs (WebGPU instead of native onnxruntime).
export default function GemmaLabPage(): React.ReactElement {
  return (
    <Layout
      title="Gemma Lab"
      description="Translate with Google Gemma 4 running locally in your browser via transformers.js + WebGPU — the same model the native kapi-llm plugin runs. No server, no API key."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Gemma Lab</h1>
          <p className={styles.lede}>
            Translate text with <strong>Gemma&nbsp;4</strong> running right here in your browser.
            The <Link to="/lab">Lab</Link>'s WASM engine runs{" "}
            <code>kapi translate --provider gemma</code>, and the model itself executes via{" "}
            <strong>transformers.js</strong> on <strong>WebGPU</strong> — the same ONNX model the
            native <code>kapi-llm</code> plugin runs in-process. It is a free, private alternative
            to the paid cloud providers: the model is a one-time multi-GB download (then cached by
            your browser), and nothing leaves the page. A <strong>WebGPU-capable browser</strong> is
            required.
          </p>
          <nav className={styles.nav} aria-label="Related labs">
            <Link to="/lab">Lab</Link>
            <Link to="/lab/vision">Vision Lab</Link>
            <Link to="/contribute/architecture/030-multimodal-extraction-and-llm-refinement">
              Multimodal extraction &amp; LLM refinement
            </Link>
          </nav>
        </div>
        <GemmaExplorer />
      </main>
    </Layout>
  );
}
