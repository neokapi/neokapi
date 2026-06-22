import React from "react";
import Layout from "@theme/Layout";
import { ModelsExplorer } from "@site/src/components/Lab";
import styles from "./models.module.css";

// Models & Providers lab: the in-browser twin of `kapi models` / the desktop
// model picker. It shows the three sources of models kapi can translate with —
// a LOCAL provider that runs on-device (the same model names everywhere; each on
// the best browser engine — WebLLM or ONNX over WebGPU — here, Ollama on the
// desktop app and CLI), CLOUD providers that need an API key, and plugins that
// add formats/segmenters — and the Local section runs a real translation in the
// browser so the desktop-like experience is tangible.

export default function ModelsLabPage(): React.ReactElement {
  return (
    <Layout
      title="Models & Providers Lab"
      description="See every model kapi can translate with — the same on-device models on web and desktop (each on the best browser engine, WebLLM or ONNX over WebGPU; Ollama on desktop/CLI), cloud providers, and plugins — and run a real local translation in your browser, no API key, nothing sent to a server."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Models &amp; Providers</h1>
          <p className={styles.lede}>
            kapi translates with the same set of models everywhere — only the backend differs. In
            this browser each model runs on the best engine that has a build for it (WebLLM or ONNX,
            both on WebGPU); on the desktop app and CLI they run via Ollama. Pick a local model and
            translate right here, then see the identical command for the CLI.
          </p>
        </div>
        <ModelsExplorer />
      </main>
    </Layout>
  );
}
