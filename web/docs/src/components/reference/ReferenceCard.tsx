import { useState, useCallback, useMemo } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import { SchemaForm } from "@neokapi/ui-primitives";
import Markdown, { unfence } from "./Markdown";
import { seedDefaults, buildFormatYaml, buildToolYaml } from "./yaml";
import styles from "./styles.module.css";

interface Props {
  entry: ReferenceEntry;
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

export default function ReferenceCard({ entry }: Props) {
  const [expanded, setExpanded] = useState(false);
  const schema = entry.schema;
  const props = schema?.properties ?? {};
  const paramCount = Object.keys(props).length;

  const defaults = useMemo(() => seedDefaults(schema), [schema]);
  const [values, setValues] = useState<Record<string, unknown>>(defaults);
  const [presetValues, setPresetValues] = useState<Record<string, unknown> | undefined>(undefined);
  const [activePreset, setActivePreset] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const toggle = useCallback(() => setExpanded((p) => !p), []);

  const onChange = useCallback((next: Record<string, unknown>) => {
    setValues(next);
  }, []);

  const resetDefaults = useCallback(() => {
    setValues(defaults);
    setPresetValues(undefined);
    setActivePreset(null);
  }, [defaults]);

  const applyPreset = useCallback(
    (name: string, params: Record<string, unknown>) => {
      const next = { ...defaults, ...params };
      setValues(next);
      setPresetValues(next);
      setActivePreset(name);
    },
    [defaults],
  );

  const yaml = useMemo(
    () =>
      entry.kind === "format"
        ? buildFormatYaml(entry.id, values, schema)
        : buildToolYaml(values, schema),
    [entry.kind, entry.id, values, schema],
  );

  const copyYaml = useCallback(() => {
    navigator.clipboard.writeText(yaml).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [yaml]);

  const presets = entry.presets ?? {};
  const presetNames = Object.keys(presets);
  const doc = entry.doc;

  return (
    <div className={`${styles.card} ${expanded ? styles.cardExpanded : ""}`}>
      <button type="button" className={styles.header} onClick={toggle} aria-expanded={expanded}>
        <span className={`${styles.chevron} ${expanded ? styles.chevronOpen : ""}`}>&#9654;</span>
        <span className={styles.name}>{entry.displayName}</span>
        <SourceBadge source={entry.source} />
        <span className={styles.meta}>
          {entry.kind === "format" ? (
            <>
              {entry.extensions?.map((ext) => (
                <span key={ext} className={styles.tag}>
                  {ext}
                </span>
              ))}
              {entry.hasReader && <span className={`${styles.cap} ${styles.capReader}`}>Reader</span>}
              {entry.hasWriter && <span className={`${styles.cap} ${styles.capWriter}`}>Writer</span>}
            </>
          ) : (
            <>
              {entry.category && <span className={styles.category}>{entry.category}</span>}
              {entry.inputs && entry.inputs.length > 0 && (
                <span className={styles.io}>
                  {entry.inputs.join(", ")}
                  {entry.outputs && entry.outputs.length > 0
                    ? ` → ${entry.outputs.join(", ")}`
                    : ""}
                </span>
              )}
            </>
          )}
        </span>
        {paramCount > 0 && (
          <span className={styles.paramCount}>
            {paramCount} parameter{paramCount !== 1 ? "s" : ""}
          </span>
        )}
      </button>

      {expanded && (
        <div className={`${styles.body} kapi-reference`}>
          {/* Overview / description */}
          {doc?.overview ? (
            <Markdown>{doc.overview}</Markdown>
          ) : (
            entry.description && <p className={styles.description}>{entry.description}</p>
          )}

          {/* Metadata grid */}
          <div className={styles.metaGrid}>
            <Meta label="ID" value={entry.id} mono />
            {entry.kind === "format" && entry.mimeTypes && entry.mimeTypes.length > 0 && (
              <Meta label="MIME Types" value={entry.mimeTypes.join(", ")} mono />
            )}
            {entry.kind === "tool" && entry.cardinality && (
              <Meta label="Cardinality" value={entry.cardinality} />
            )}
            {entry.kind === "tool" && entry.requires && entry.requires.length > 0 && (
              <Meta label="Requires" value={entry.requires.join(", ")} mono />
            )}
            {entry.kind === "tool" && entry.tags && entry.tags.length > 0 && (
              <Meta label="Tags" value={entry.tags.join(", ")} />
            )}
          </div>

          {/* Presets */}
          {presetNames.length > 0 && (
            <div className={styles.presetSection}>
              <div className={styles.sectionTitle}>Presets</div>
              <div className={styles.presetButtons}>
                <button
                  type="button"
                  className={`${styles.presetButton} ${activePreset === null ? styles.presetButtonActive : ""}`}
                  onClick={resetDefaults}
                  title="Default configuration"
                >
                  Default
                </button>
                {presetNames.map((name) => (
                  <button
                    key={name}
                    type="button"
                    className={`${styles.presetButton} ${activePreset === name ? styles.presetButtonActive : ""}`}
                    onClick={() => applyPreset(name, presets[name] as Record<string, unknown>)}
                  >
                    {name}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Interactive form (live SchemaForm) */}
          {paramCount === 0 ? (
            <p className={styles.noConfig}>This {entry.kind} has no configurable parameters.</p>
          ) : (
            <div className={styles.formSection}>
              <div className={styles.sectionTitle}>Configuration</div>
              <div className={styles.form}>
                <BrowserOnly fallback={<div className={styles.formFallback}>Loading form…</div>}>
                  {() => (
                    <SchemaForm
                      schema={schema!}
                      values={values}
                      onChange={onChange}
                      presetValues={presetValues}
                      paramDocs={doc?.parameters}
                      hideHeader
                      compact
                    />
                  )}
                </BrowserOnly>
              </div>

              {/* YAML output */}
              <div className={styles.outputHeader}>
                <span className={styles.sectionTitle}>Configuration output</span>
                <div className={styles.outputActions}>
                  <button type="button" className={styles.resetButton} onClick={resetDefaults}>
                    Reset
                  </button>
                  <button type="button" className={styles.copyButton} onClick={copyYaml}>
                    {copied ? "Copied" : "Copy YAML"}
                  </button>
                </div>
              </div>
              <pre className={styles.yaml}>{yaml}</pre>
            </div>
          )}

          {/* Examples */}
          {doc?.examples && doc.examples.length > 0 && (
            <div className={styles.docSection}>
              <div className={styles.sectionTitle}>Examples</div>
              {doc.examples.map((ex, i) => (
                <div key={`${ex.title}-${i}`} className={styles.example}>
                  <div className={styles.exampleTitle}>{ex.title}</div>
                  {ex.description && <Markdown>{ex.description}</Markdown>}
                  {ex.config && <pre className={styles.yaml}>{unfence(ex.config)}</pre>}
                </div>
              ))}
            </div>
          )}

          {/* Processing notes */}
          {doc?.processingNotes && doc.processingNotes.length > 0 && (
            <div className={styles.docSection}>
              <div className={styles.sectionTitle}>Processing notes</div>
              <ul className={styles.noteList}>
                {doc.processingNotes.map((note, i) => (
                  <li key={i}>
                    <Markdown>{note}</Markdown>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {/* Limitations */}
          {doc?.limitations && doc.limitations.length > 0 && (
            <div className={styles.docSection}>
              <div className={styles.sectionTitle}>Limitations</div>
              <ul className={styles.noteList}>
                {doc.limitations.map((lim, i) => (
                  <li key={i}>
                    <Markdown>{lim}</Markdown>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {/* Wiki link */}
          {doc?.wikiUrl && (
            <p className={styles.wikiLink}>
              <a href={doc.wikiUrl} target="_blank" rel="noreferrer">
                Reference documentation
              </a>
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function Meta({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className={styles.metaItem}>
      <span className={styles.metaLabel}>{label}</span>
      <span className={mono ? styles.metaValueMono : styles.metaValue}>{value}</span>
    </div>
  );
}
