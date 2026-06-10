import clsx from "clsx";
import Heading from "@theme/Heading";
import { FileText, Languages, ShieldCheck, Wand2, Repeat, Workflow } from "lucide-react";
import styles from "./home.module.css";

// The end-to-end pipeline, folded in from the landing. One engine spans extract
// → translate → check → fix → write back → gate. Plain numbered steps; the
// commands are real and verified against the CLI.
const STEPS = [
  {
    n: "01",
    icon: FileText,
    title: "Extract",
    cmd: "kapi extract report.docx",
    description:
      "Pull translatable text out of any format into a clean structured view. The original structure, styles, and placeholders are remembered for a faithful write-back.",
  },
  {
    n: "02",
    icon: Languages,
    title: "Translate",
    cmd: "kapi ai-translate · or via MCP",
    description:
      "Translate with AI or MT, or let an assistant draft and edit content through MCP — placeholders, inline tags, and markup preserved as it goes.",
  },
  {
    n: "03",
    icon: ShieldCheck,
    title: "Check",
    cmd: "kapi verify",
    description:
      "Run the checks like tests: do-not-translate, placeholder and tag integrity, terminology, and brand voice. Findings name the exact strings and rules that broke.",
  },
  {
    n: "04",
    icon: Wand2,
    title: "Fix",
    cmd: "apply suggestions · re-run",
    description:
      "Resolve a dropped placeholder, a translated product name, or an off-voice phrase, keeping meaning intact, then re-run the checks until they pass.",
  },
  {
    n: "05",
    icon: Repeat,
    title: "Write back",
    cmd: "kapi merge",
    description:
      "Write the result into the original native format, unchanged except where you intended — a faithful round-trip, in every locale.",
  },
  {
    n: "06",
    icon: Workflow,
    title: "Gate",
    cmd: "kapi verify  (in CI)",
    description:
      "Keep recipes, profiles, and termbases version-controlled and gate quality on every commit — the same checks locally and in your pipeline, exiting non-zero on failure.",
  },
];

export default function Pipeline() {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.sectionHead}>
          <span className={styles.eyebrow}>The pipeline</span>
          <Heading as="h2" className={styles.sectionTitle}>
            From any file to <span className={styles.accent}>shipped, in every language</span>
          </Heading>
          <p className={styles.sectionLede}>
            One engine spans the whole pipeline: extract from any format, translate and check the
            content the way you test code, then write the original back, faithfully.
          </p>
        </div>

        <div className={styles.rail}>
          {STEPS.map((s, i) => (
            <span key={s.n} className={styles.railStep}>
              {s.title}
              {i < STEPS.length - 1 && <span className={styles.railArrow}>&rarr;</span>}
            </span>
          ))}
        </div>

        <div className={clsx(styles.grid, styles.grid3)}>
          {STEPS.map((s) => (
            <div key={s.n} className={styles.card}>
              <div className={styles.stepHead}>
                <span className={styles.cardIcon}>
                  <s.icon size={20} aria-hidden="true" />
                </span>
                <span className={styles.stepNum}>{s.n}</span>
              </div>
              <Heading as="h3" className={styles.cardTitle}>
                {s.title}
              </Heading>
              <code className={styles.stepCmd}>{s.cmd}</code>
              <p className={styles.cardText}>{s.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
