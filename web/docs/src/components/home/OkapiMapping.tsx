import Heading from "@theme/Heading";
import { ArrowRight } from "lucide-react";
import styles from "./home.module.css";

// neokapi reimagines the Okapi Framework in Go; this table names the heritage
// concept-for-concept. Mirrors the terminology table in CLAUDE.md and the
// architecture docs.
const MAPPINGS = [
  { okapi: "Filter", neokapi: "DataFormat", desc: "Reader / Writer" },
  { okapi: "Event", neokapi: "Part", desc: "Processing unit" },
  { okapi: "Step", neokapi: "Tool", desc: "Task unit" },
  { okapi: "Pipeline", neokapi: "Flow", desc: "Tool orchestration" },
  { okapi: "TextUnit", neokapi: "Block", desc: "Translatable content" },
  { okapi: "TextFragment", neokapi: "[]Run", desc: "Run sequence" },
  { okapi: "Code", neokapi: "Run", desc: "Inline markup (Ph / Pc)" },
  { okapi: "Tikal", neokapi: "kapi", desc: "CLI tool" },
];

export default function OkapiMapping() {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.sectionHead}>
          <Heading as="h2" className={styles.sectionTitle}>
            Built on <span className={styles.accent}>Okapi&apos;s foundations</span>
          </Heading>
          <p className={styles.sectionLede}>
            neokapi reimagines the proven Okapi Framework in Go for native speed and near-instant
            local processing. A bridge to Java gives access to the battle-tested Okapi filters and
            steps.
          </p>
        </div>

        <div className={styles.mapTable}>
          <div className={`${styles.mapRow} ${styles.mapHead}`}>
            <span>Okapi (Java)</span>
            <span />
            <span className={styles.mapHeadTo}>neokapi (Go)</span>
            <span className={styles.mapDesc}>Purpose</span>
          </div>
          {MAPPINGS.map((m) => (
            <div key={m.okapi} className={styles.mapRow}>
              <code className={styles.mapFrom}>{m.okapi}</code>
              <ArrowRight size={14} className={styles.mapArrow} aria-hidden="true" />
              <code className={styles.mapTo}>{m.neokapi}</code>
              <span className={styles.mapDesc}>{m.desc}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
