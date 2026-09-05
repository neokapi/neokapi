import Link from "@docusaurus/Link";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import { categoryLabel, sourceLabel } from "./labels";
import styles from "./styles.module.css";

// Held at module level so the build-time transform rewrites each call.
const SOURCE_TITLE: Record<ReferenceSource, string> = {
  "built-in": t("Built into neokapi", "tooltip on the source badge"),
  plugin: t("Provided by an engine plugin", "tooltip on the source badge"),
  okapi: t("Provided by the Okapi Framework bridge", "tooltip on the source badge"),
};

function paramCountLabel(n: number): string {
  return n === 1
    ? t("1 param", "parameter count on a reference card")
    : t("{count} params", "parameter count on a reference card", { count: n });
}

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
  const isPlugin = source !== "built-in";
  const label = sourceLabel(source);
  return (
    <span
      className={`${styles.sourceBadge} ${isPlugin ? styles.sourcePlugin : styles.sourceBuiltin}`}
      title={SOURCE_TITLE[source]}
    >
      {label}
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
          entry.category && <span className={styles.category}>{categoryLabel(entry.category)}</span>
        )}
        {paramCount > 0 && (
          <span className={styles.gridParamCount}>{paramCountLabel(paramCount)}</span>
        )}
      </span>
    </Link>
  );
}
