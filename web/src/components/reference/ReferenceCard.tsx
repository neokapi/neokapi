import Link from "@docusaurus/Link";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import styles from "./styles.module.css";

interface Props {
  entry: ReferenceEntry;
  /**
   * The canonical static page route for this entry (without the docs baseUrl),
   * e.g. "/reference/formats/json". The card is a real link to this page —
   * crawlable, middle-clickable, and shareable.
   */
  href: string;
}

function SourceBadge({ source }: { source: ReferenceSource }) {
  const isOkapi = source === "okapi";
  return (
    <span
      className={`${styles.sourceBadge} ${isOkapi ? styles.sourceOkapi : styles.sourceBuiltin}`}
      title={isOkapi ? "Provided by the Okapi bridge plugin" : "Built into neokapi"}
    >
      {isOkapi ? "Okapi bridge" : "Built-in"}
    </span>
  );
}

/**
 * A compact card in the reference grid. It is a real link to the entry's static,
 * shareable reference page (good for SEO + open-in-new-tab); the page owns the
 * heavy detail/form state, so the grid stays cheap to render.
 */
export default function ReferenceCard({ entry, href }: Props) {
  const schema = entry.schema;
  const paramCount = Object.keys(schema?.properties ?? {}).length;

  return (
    <Link className={styles.gridCard} to={href}>
      <span className={styles.gridCardHead}>
        <span className={styles.gridCardName}>{entry.displayName}</span>
        <SourceBadge source={entry.source} />
      </span>

      {entry.description && <span className={styles.gridCardDesc}>{entry.description}</span>}

      <span className={styles.gridCardFoot}>
        {entry.kind === "format" ? (
          <>
            {entry.extensions?.slice(0, 3).map((ext) => (
              <span key={ext} className={styles.tag}>
                {ext}
              </span>
            ))}
            {entry.extensions && entry.extensions.length > 3 && (
              <span className={styles.more}>+{entry.extensions.length - 3}</span>
            )}
            {entry.hasReader && <span className={`${styles.cap} ${styles.capReader}`}>R</span>}
            {entry.hasWriter && <span className={`${styles.cap} ${styles.capWriter}`}>W</span>}
          </>
        ) : (
          entry.category && <span className={styles.category}>{entry.category}</span>
        )}
        {paramCount > 0 && (
          <span className={styles.gridParamCount}>
            {paramCount} param{paramCount !== 1 ? "s" : ""}
          </span>
        )}
      </span>
    </Link>
  );
}
