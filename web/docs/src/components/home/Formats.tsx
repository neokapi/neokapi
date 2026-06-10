import clsx from "clsx";
import Link from "@docusaurus/Link";
import Heading from "@theme/Heading";
import styles from "./home.module.css";

// Format categories — named, not counted. Per the brand guide, the home page
// never hardcodes the format count (the code controls it); it names the
// categories and links to the generated Format Reference for the full list.
const FORMAT_GROUPS = [
  {
    category: "Data",
    description: "Structured key–value and tabular files — the catalogs apps ship strings in.",
  },
  {
    category: "Content",
    description: "Authored prose and markup, plus subtitle and caption tracks.",
  },
  {
    category: "Office & publishing",
    description: "Word-processing, spreadsheet, presentation, e-book, and layout documents.",
  },
  {
    category: "Interchange",
    description: "Bilingual handoff and translation-memory formats for the translator workflow.",
  },
];

export default function Formats() {
  return (
    <section className={clsx(styles.section, styles.sectionAlt)}>
      <div className="container">
        <div className={styles.sectionHead}>
          <Heading as="h2" className={styles.sectionTitle}>
            The formats your content lives in
          </Heading>
          <p className={styles.sectionLede}>
            Native readers and writers for localization, data, content, subtitle, and office
            formats, detected by extension, MIME type, or content — with more available through the
            okapi-bridge. A round-trip, in place, not string-and-key extraction.
          </p>
        </div>

        <div className={clsx(styles.grid, styles.grid4)}>
          {FORMAT_GROUPS.map((group) => (
            <div key={group.category} className={styles.card}>
              <Heading as="h3" className={styles.cardTitle}>
                {group.category}
              </Heading>
              <p className={styles.cardText}>{group.description}</p>
            </div>
          ))}
        </div>

        <div className={styles.sectionFoot}>
          <Link className={styles.ctaLink} to="/formats">
            Browse the Format Reference &rarr;
          </Link>
        </div>
      </div>
    </section>
  );
}
