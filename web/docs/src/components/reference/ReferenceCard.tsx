import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import styles from "./styles.module.css";

interface Props {
  entry: ReferenceEntry;
  /** Opens the detail modal for this entry (and writes ?id= to the URL). */
  onSelect: (id: string) => void;
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
 * A compact, clickable card in the reference grid. Clicking opens the full
 * {@link ReferenceModal} for the entry; the heavy detail view and form state
 * live there, so the grid stays cheap to render even with hundreds of cards.
 */
export default function ReferenceCard({ entry, onSelect }: Props) {
  const schema = entry.schema;
  const paramCount = Object.keys(schema?.properties ?? {}).length;

  return (
    <button
      type="button"
      className={styles.gridCard}
      onClick={() => onSelect(entry.id)}
      aria-haspopup="dialog"
    >
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
    </button>
  );
}
