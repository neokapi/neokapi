import clsx from "clsx";
import Heading from "@theme/Heading";
import { Boxes, Layers, WifiOff } from "lucide-react";
import styles from "./home.module.css";

// "Three axes, one engine" — folded in from the retired landing app. States what
// the engine spans (formats, the translate/check/transform path, local-first)
// plainly, without competitive framing — this is a framework, documented, not a
// product being sold.
const AXES = [
  {
    icon: Boxes,
    title: "Any asset format",
    description:
      "Native readers and writers for localization, document, data, subtitle, and office formats, with more through the okapi-bridge. A round-trip in place, not string-and-key extraction.",
  },
  {
    icon: Layers,
    title: "Translate, check, and transform, unified",
    description:
      "Translation, terminology, QA, and voice scoring share one content model and one enforcement path, so what you ship stays consistent through every language.",
  },
  {
    icon: WifiOff,
    title: "Local-first",
    description:
      "A single binary with an embedded translation memory and termbase. Run entirely offline with local models, or opt into a cloud LLM — your call, not a default.",
  },
];

export default function Differentiators() {
  return (
    <section className={clsx(styles.section, styles.sectionAlt)}>
      <div className="container">
        <div className={styles.sectionHead}>
          <Heading as="h2" className={styles.sectionTitle}>
            Three axes, <span className={styles.accent}>one engine</span>
          </Heading>
          <p className={styles.sectionLede}>
            Writing tools, brand-voice prompts, and localization services each tend to cover one
            slice. neokapi works across all three from a single content model.
          </p>
        </div>
        <div className={clsx(styles.grid, styles.grid3)}>
          {AXES.map((a) => (
            <div key={a.title} className={styles.card}>
              <span className={styles.cardIcon}>
                <a.icon size={22} aria-hidden="true" />
              </span>
              <Heading as="h3" className={styles.cardTitle}>
                {a.title}
              </Heading>
              <p className={styles.cardText}>{a.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
