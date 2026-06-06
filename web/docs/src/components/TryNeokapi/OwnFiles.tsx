import React from "react";
import { PseudoTranslateWidget, SearchReplaceWidget, StatsWidget } from "../Lab";
import type { DemoId } from "./demos";
import styles from "./styles.module.css";

// "Try with your own files": swaps the faked showcase for the REAL engine path.
// Reuses the existing Lab drop-a-file widgets (drop/choose + real wasm engine +
// block diff / stats), one per active demo. No faked preview here — the widgets
// show the real block diff/result. wasm boots lazily inside each widget on its
// first run, so toggling this on does not by itself pull the engine.
//
// The widgets offer the small hero corpus (json + docx) as samples; a visitor
// can also drop their own file. For pseudo/stats the JSON sample auto-loads; for
// search-replace it defaults to Acme → Globex to match the showcase narrative.

interface OwnFilesProps {
  demo: DemoId;
}

const SAMPLE_IDS = ["json", "docx"];

export default function OwnFiles({ demo }: OwnFilesProps): React.ReactElement {
  return (
    <div className={styles.ownWrap}>
      {demo === "search-replace" && (
        <SearchReplaceWidget
          sampleIds={SAMPLE_IDS}
          autoSampleId="json"
          defaultFind="Acme"
          defaultReplace="Globex"
        />
      )}
      {demo === "insights" && <StatsWidget sampleIds={SAMPLE_IDS} autoSampleId="json" />}
      {demo === "pseudo" && <PseudoTranslateWidget sampleIds={SAMPLE_IDS} autoSampleId="json" />}
    </div>
  );
}
