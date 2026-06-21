import useBaseUrl from "@docusaurus/useBaseUrl";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import styles from "./styles.module.css";

interface Props {
  entry: ReferenceEntry;
  /**
   * The canonical static page route for this entry (without the docs baseUrl),
   * e.g. "/reference/formats/json". The card is a real link to this URL so it is
   * crawlable + middle-clickable; a plain left-click opens the quick modal
   * instead (see onSelect).
   */
  href: string;
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
 * A compact card in the reference grid. It is a real link to the entry's static,
 * shareable page (good for SEO + open-in-new-tab); a plain left-click instead
 * opens the in-page {@link ReferenceModal} quick view. The heavy detail/form
 * state lives in the modal (and the static page), so the grid stays cheap.
 */
export default function ReferenceCard({ entry, href, onSelect }: Props) {
  const schema = entry.schema;
  const paramCount = Object.keys(schema?.properties ?? {}).length;
  const resolvedHref = useBaseUrl(href);

  return (
    <a
      className={styles.gridCard}
      href={resolvedHref}
      onClick={(e) => {
        // Let modifier-clicks / middle-clicks follow the real link (new tab,
        // copy address, etc.); a plain left-click opens the quick modal.
        if (
          e.defaultPrevented ||
          e.button !== 0 ||
          e.metaKey ||
          e.ctrlKey ||
          e.shiftKey ||
          e.altKey
        )
          return;
        e.preventDefault();
        onSelect(entry.id);
      }}
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
    </a>
  );
}
