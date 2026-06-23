import useBaseUrl from "@docusaurus/useBaseUrl";
import { tools } from "@neokapi/reference-data";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import { BeforeAfter } from "@site/src/components/curated";
import Markdown, { unfence } from "@site/src/components/reference/Markdown";
import { toolBeforeAfter } from "@site/src/components/reference/curatedEmbed";
import ParametersTable from "./ParametersTable";
import InteractiveConfig from "./InteractiveConfig";
import styles from "./pages.module.css";

interface Props {
  /** Tool id, e.g. "pseudo-translate". */
  id: string;
  /**
   * Which engine provides the tool. Required because tool ids collide across
   * sources (a built-in and an Okapi-bridge tool can share an id); `source:id`
   * is unique.
   */
  source: ReferenceSource;
}

/**
 * The full, static reference body for one processing tool, rendered from the
 * generated `@neokapi/reference-data` dataset — overview, metadata, a live
 * BeforeAfter transform (when the tool is offline-runnable on a bundled
 * sample), a parameters table, and authored examples / notes / limitations.
 * Imported by the generated MDX page; all content derives from the data.
 */
export default function ToolReferencePage({ id, source }: Props) {
  const toolsHref = useBaseUrl("/tools");
  const entry: ReferenceEntry | undefined = tools.entries.find(
    (e) => e.id === id && e.source === source,
  );
  if (!entry) {
    return (
      <p>
        Unknown tool: {id} ({source})
      </p>
    );
  }

  const doc = entry.doc;
  const sourceLabel = entry.source === "okapi" ? "Okapi bridge" : "Built-in";
  const beforeAfter = toolBeforeAfter(entry);

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
        {entry.category && <Meta label="Category" value={entry.category} />}
        {entry.cardinality && <Meta label="Cardinality" value={entry.cardinality} />}
        {entry.inputs && entry.inputs.length > 0 && (
          <Meta label="Inputs" value={entry.inputs.join(", ")} mono />
        )}
        {entry.outputs && entry.outputs.length > 0 && (
          <Meta label="Outputs" value={entry.outputs.join(", ")} mono />
        )}
        {entry.requires && entry.requires.length > 0 && (
          <Meta label="Requires" value={entry.requires.join(", ")} mono />
        )}
        {entry.tags && entry.tags.length > 0 && <Meta label="Tags" value={entry.tags.join(", ")} />}
      </div>

      {/* Curated: source → transformed result (when offline-runnable) */}
      {beforeAfter && (
        <section className={styles.curatedSection}>
          <h2 className={styles.sectionHeading}>See it transform</h2>
          <BeforeAfter
            sample={beforeAfter.sample}
            command={beforeAfter.command}
            outputPath={beforeAfter.output}
            caption={beforeAfter.caption}
          />
        </section>
      )}

      {/* Parameters — static, crawlable table (no JS required) */}
      {entry.schema?.properties && Object.keys(entry.schema.properties).length > 0 ? (
        <section>
          <h2 className={styles.sectionHeading}>Parameters</h2>
          <ParametersTable schema={entry.schema} doc={doc} />
        </section>
      ) : (
        <p className={styles.noConfig}>This tool has no configurable parameters.</p>
      )}

      {/* Live configurator: form + flow-step YAML + presets + Run */}
      <InteractiveConfig entry={entry} kind="tool" />

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
        &larr; Back to the <a href={toolsHref}>Tool Reference</a>
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
