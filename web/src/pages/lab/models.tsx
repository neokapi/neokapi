import React from "react";
import Layout from "@theme/Layout";
import { ModelsExplorer } from "@site/src/components/Lab";
import styles from "./models.module.css";

// Models & Providers lab: the in-browser twin of `kapi models` / the desktop
// model picker. It shows the three sources of models kapi can translate with —
// a LOCAL provider that runs on-device (WebLLM/WebGPU here, Ollama on the
// desktop app and CLI), CLOUD providers that need an API key, and plugins that
// add formats/segmenters — and the Local section runs a real translation in the
// browser so the desktop-like experience is tangible.

export default function ModelsLabPage(): React.ReactElement {
  return (
    <Layout
      title="Models & Providers Lab"
      description="See every model kapi can translate with — a local on-device provider (WebLLM in your browser, Ollama on desktop/CLI), cloud providers, and plugins — and run a real local translation in your browser, no API key, nothing sent to a server."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Models &amp; Providers</h1>
          <p className={styles.lede}>
            kapi translates with the same set of models everywhere — it just uses the best{" "}
            <strong>local</strong> engine for each platform: WebLLM (WebGPU) in your browser, Ollama
            on the desktop app and CLI. Pick a local model and translate right here, then see the
            identical command for the CLI.
          </p>
        </div>
        <ModelsExplorer />
      </main>
    </Layout>
  );
}
