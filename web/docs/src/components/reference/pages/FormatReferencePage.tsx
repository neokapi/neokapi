import useBaseUrl from "@docusaurus/useBaseUrl";
import { formats } from "@neokapi/reference-data";
import type { ReferenceEntry } from "@neokapi/reference-data";
import { BlockPreview } from "@site/src/components/curated";
import Markdown, { unfence } from "@site/src/components/reference/Markdown";
import { formatPreviewSample } from "@site/src/components/reference/curatedEmbed";
import ParametersTable from "./ParametersTable";
import styles from "./pages.module.css";

interface Props {
  /** Format id, e.g. "json" or "okf_html5". */
  id: string;
}

/**
 * The full, static reference body for one data format, rendered from the
 * generated `@neokapi/reference-data` dataset — overview, metadata, a live
 * BlockPreview (when a bundled sample matches the format), a parameters table,
 * and authored examples / notes / limitations. Imported by the generated MDX
 * page; all content derives from the data.
 */
export default function FormatReferencePage({ id }: Props) {
  const formatsHref = useBaseUrl("/formats");
  const entry: ReferenceEntry | undefined = formats.entries.find((e) => e.id === id);
  if (!entry) {
    return <p>Unknown format: {id}</p>;
  }

  const doc = entry.doc;
  const sourceLabel = entry.source === "okapi" ? "Okapi bridge" : "Built-in";
  const sample = formatPreviewSample(entry);

  return (
    <div className={`${styles.page} kapi-reference`}>
      {/* Overview / description */}
      {doc?.overview ? (
        <Markdown>{doc.overview}</Markdown>
      ) : (
        entry.description && <p className={styles.lead}>{entry.description}</p>
      )}

      {/* Metadata */}
      <div className={styles.metaGrid}>
        <Meta label="ID" value={entry.id} mono />
        <Meta label="Source" value={sourceLabel} />
        {entry.extensions && entry.extensions.length > 0 && (
          <Meta label="Extensions" value={entry.extensions.join(", ")} mono />
        )}
        {entry.mimeTypes && entry.mimeTypes.length > 0 && (
          <Meta label="MIME Types" value={entry.mimeTypes.join(", ")} mono />
        )}
        <Meta
          label="Capabilities"
          value={[entry.hasReader && "Read", entry.hasWriter && "Write"]
            .filter(Boolean)
            .join(" + ")}
        />
      </div>

      {/* Curated: how kapi parses a representative sample (when one matches) */}
      {sample && (
        <section className={styles.curatedSection}>
          <h2 className={styles.sectionHeading}>How kapi reads it</h2>
          <BlockPreview
            sample={sample}
            caption={`How kapi parses a representative ${entry.displayName} file into translatable blocks.`}
          />
        </section>
      )}

      {/* Parameters */}
      {entry.schema?.properties && Object.keys(entry.schema.properties).length > 0 ? (
        <section>
          <h2 className={styles.sectionHeading}>Parameters</h2>
          <ParametersTable schema={entry.schema} doc={doc} />
          <p className={styles.configHint}>
            Configure these parameters interactively and copy the YAML on the{" "}
            <a href={`/formats?id=${encodeURIComponent(entry.id)}`}>Format Reference</a>.
          </p>
        </section>
      ) : (
        <p className={styles.noConfig}>This format has no configurable parameters.</p>
      )}

      {/* Examples */}
      {doc?.examples && doc.examples.length > 0 && (
        <section>
          <h2 className={styles.sectionHeading}>Examples</h2>
          {doc.examples.map((ex, i) => (
            <div key={`${ex.title}-${i}`} className={styles.example}>
              <h3 className={styles.exampleTitle}>{ex.title}</h3>
              {ex.description && <Markdown>{ex.description}</Markdown>}
              {ex.config && <pre className={styles.codeBlock}>{unfence(ex.config)}</pre>}
            </div>
          ))}
        </section>
      )}

      {/* Processing notes */}
      {doc?.processingNotes && doc.processingNotes.length > 0 && (
        <section>
          <h2 className={styles.sectionHeading}>Processing notes</h2>
          <ul className={styles.noteList}>
            {doc.processingNotes.map((note, i) => (
              <li key={i}>
                <Markdown>{note}</Markdown>
              </li>
            ))}
          </ul>
        </section>
      )}

      {/* Limitations */}
      {doc?.limitations && doc.limitations.length > 0 && (
        <section>
          <h2 className={styles.sectionHeading}>Limitations</h2>
          <ul className={styles.noteList}>
            {doc.limitations.map((lim, i) => (
              <li key={i}>
                <Markdown>{lim}</Markdown>
              </li>
            ))}
          </ul>
        </section>
      )}

      {/* Wiki link */}
      {doc?.wikiUrl && (
        <p className={styles.wikiLink}>
          <a href={doc.wikiUrl} target="_blank" rel="noreferrer">
            Reference documentation
          </a>
        </p>
      )}

      <p className={styles.browseBack}>
        &larr; Back to the <a href={formatsHref}>Format Reference</a>
      </p>
    </div>
  );
}

function Meta({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  if (!value) return null;
  return (
    <div className={styles.metaItem}>
      <span className={styles.metaLabel}>{label}</span>
      <span className={mono ? styles.metaValueMono : styles.metaValue}>{value}</span>
    </div>
  );
}
