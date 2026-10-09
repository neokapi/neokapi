import useBaseUrl from "@docusaurus/useBaseUrl";
import { checks } from "@neokapi/reference-data";
import type { ReferenceEntry } from "@neokapi/reference-data";
import Markdown from "@site/src/components/reference/Markdown";
import styles from "./pages.module.css";

interface Props {
  /** Check id, the name the code registers it under, e.g. "content-lint". */
  id: string;
}

/**
 * The full, static reference body for one check `kapi check` runs, rendered
 * from the generated `@neokapi/reference-data` dataset: the overview, the rule
 * family and every rule id a finding carries with what it reports and what
 * fixes it, the flags that configure it, and the authored notes and
 * limitations. Imported by the generated MDX page; all content derives from
 * the data.
 */
export default function CheckReferencePage({ id }: Props) {
  const checksHref = useBaseUrl("/reference/checks");
  const entry: ReferenceEntry | undefined = checks.entries.find((e) => e.id === id);
  if (!entry) {
    return <p>Unknown check: {id}</p>;
  }

  const doc = entry.doc;
  const params = doc?.parameters ?? {};
  const paramNames = Object.keys(params).sort((a, b) => a.localeCompare(b));
  const rules = entry.rules ?? [];

  return (
    <div className={`${styles.page} kapi-reference`}>
      {doc?.overview ? (
        <Markdown>{doc.overview}</Markdown>
      ) : (
        entry.description && <p className={styles.lead}>{entry.description}</p>
      )}

      <div className={styles.metaGrid}>
        <Meta label="Check" value={entry.id} mono />
        {entry.ruleFamily && <Meta label="Rule family" value={`${entry.ruleFamily}.*`} mono />}
        <Meta label="Source" value="Built-in" />
      </div>

      {/* The rule ids a findings table carries: the id a reader looks up. */}
      {rules.length > 0 && (
        <section>
          <h2 className={styles.sectionHeading}>Rules</h2>
          <p>
            A finding carries one of these rule ids. The family is <code>{entry.ruleFamily}</code>,
            which differs from the check id the code registers.
          </p>
          <table className={styles.paramTable}>
            <thead>
              <tr>
                <th>Rule</th>
                <th>Severity</th>
                <th>Reports</th>
                <th>Fix</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule) => (
                <tr key={rule.id}>
                  <td className={styles.paramName}>
                    <code>{rule.id}</code>
                  </td>
                  <td className={styles.paramType}>{rule.severity ?? ""}</td>
                  <td className={styles.paramDesc}>
                    <Markdown>{rule.reports ?? ""}</Markdown>
                  </td>
                  <td className={styles.paramDesc}>
                    <Markdown>{rule.fix ?? ""}</Markdown>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {/* The flags that configure the check, when it takes any. */}
      {paramNames.length > 0 ? (
        <section>
          <h2 className={styles.sectionHeading}>Flags</h2>
          <table className={styles.paramTable}>
            <thead>
              <tr>
                <th>Flag</th>
                <th>Values</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              {paramNames.map((name) => {
                const p = params[name];
                return (
                  <tr key={name}>
                    <td className={styles.paramName}>
                      <code>{name}</code>
                    </td>
                    <td className={styles.paramType}>{p.values ?? ""}</td>
                    <td className={styles.paramDesc}>
                      <Markdown>{p.help || p.description || ""}</Markdown>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </section>
      ) : (
        <p className={styles.noConfig}>This check takes no flag of its own.</p>
      )}

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

      <p className={styles.browseBack}>
        &larr; Back to the <a href={checksHref}>Check Reference</a>
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
