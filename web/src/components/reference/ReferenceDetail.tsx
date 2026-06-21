import { useState, useCallback, useMemo } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import type { ReferenceEntry } from "@neokapi/reference-data";
import { SchemaForm } from "@neokapi/ui-primitives";
import Markdown, { unfence } from "./Markdown";
import { seedDefaults, buildFormatYamlLines, buildToolYamlLines, yamlText } from "./yaml";
import { buildRunOptions, pickFixture, notRunnableReason } from "./run-config";
import styles from "./styles.module.css";

interface Props {
  entry: ReferenceEntry;
}

/** All available playground fixture names (must match fixtures.ts). */
const ALL_FIXTURES = [
  "messages.json",
  "app.xliff",
  "page.html",
  "README.md",
  "app.properties",
  "strings.xml",
  "Localizable.xcstrings",
] as const;

/**
 * The full, interactive reference body for one format/tool: overview,
 * metadata, presets, the live SchemaForm beside a sticky configuration-output
 * panel, and authored docs. Rendered inside {@link ReferenceModal}; kept
 * separate so the card grid stays lightweight and the detail view owns its own
 * form state.
 */
export default function ReferenceDetail({ entry }: Props) {
  const schema = entry.schema;
  const props = schema?.properties ?? {};
  const paramCount = Object.keys(props).length;
  const doc = entry.doc;

  const presets = useMemo(() => entry.presets ?? {}, [entry.presets]);
  const presetNames = Object.keys(presets);

  const defaults = useMemo(() => seedDefaults(schema), [schema]);
  const [values, setValues] = useState<Record<string, unknown>>(defaults);
  const [activePreset, setActivePreset] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  // The baseline a user's edits are measured against: the active preset's
  // merged values, or the schema defaults when no preset is selected. Passed to
  // SchemaForm as presetValues so changed fields are highlighted, and used here
  // to flag the changed lines in the output.
  const baseline = useMemo(() => {
    if (activePreset && presets[activePreset]) {
      return { ...defaults, ...(presets[activePreset] as Record<string, unknown>) };
    }
    return defaults;
  }, [activePreset, defaults, presets]);

  const onChange = useCallback((next: Record<string, unknown>) => {
    setValues(next);
  }, []);

  const selectDefault = useCallback(() => {
    setActivePreset(null);
    setValues(defaults);
  }, [defaults]);

  const applyPreset = useCallback(
    (name: string, params: Record<string, unknown>) => {
      setActivePreset(name);
      setValues({ ...defaults, ...params });
    },
    [defaults],
  );

  // Revert the user's edits back to the active baseline, keeping the preset.
  const revert = useCallback(() => setValues(baseline), [baseline]);

  const dirtyKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const key of Object.keys(values)) {
      if (JSON.stringify(values[key]) !== JSON.stringify(baseline[key])) keys.add(key);
    }
    return keys;
  }, [values, baseline]);

  const yamlLines = useMemo(
    () =>
      entry.kind === "format"
        ? buildFormatYamlLines(entry.id, values, schema)
        : buildToolYamlLines(values, schema),
    [entry.kind, entry.id, values, schema],
  );

  const copyYaml = useCallback(() => {
    void navigator.clipboard.writeText(yamlText(yamlLines)).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [yamlLines]);

  // "Run this config" — playground integration. `pickFixture` returns null
  // for entries with no compatible sample; in that case `runOptions` is also
  // null and the Run UI never renders, so the fixture state falls back to the
  // first option purely to keep the <select> controlled.
  const defaultFixture = useMemo(() => pickFixture(entry), [entry]);
  const [selectedFixture, setSelectedFixture] = useState<string>(defaultFixture ?? ALL_FIXTURES[0]);
  const runOptions = useMemo(() => buildRunOptions(entry, values, schema), [entry, values, schema]);
  const whyNotRunnable = useMemo(() => notRunnableReason(entry), [entry]);

  const handleRunConfig = useCallback(() => {
    if (!runOptions) return;
    // Defer the heavy kit import to browser context (openKapi is SSR-clean but
    // the dynamic import keeps the bundle split clean for SSR paths).
    void import("@neokapi/kapi-playground").then(({ openKapi }) => {
      const opts = { ...runOptions };
      // Ensure the user's selected fixture is seeded.
      if (!opts.seed?.includes(selectedFixture)) {
        opts.seed = [selectedFixture, ...(opts.seed ?? []).filter((s) => s !== defaultFixture)];
      }
      // Patch the fixture name in cmd when the user picked a different sample.
      if (defaultFixture && selectedFixture !== defaultFixture && opts.cmd) {
        opts.cmd = opts.cmd.replace(defaultFixture, selectedFixture);
      }
      openKapi(opts);
    });
  }, [runOptions, selectedFixture, defaultFixture]);

  const baselineLabel = activePreset ?? "defaults";
  const dirtyCount = dirtyKeys.size;

  return (
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
              onClick={selectDefault}
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

      {/* Interactive form beside a sticky output panel */}
      {paramCount === 0 ? (
        <div>
          <p className={styles.noConfig}>This {entry.kind} has no configurable parameters.</p>
          {/* Still offer Run for param-less entries that are offline-capable. */}
          <BrowserOnly>
            {() =>
              runOptions ? (
                <div className={`${styles.runSection} ${styles.runSectionStandalone}`}>
                  <div className={styles.runRow}>
                    <label htmlFor={`fixture-solo-${entry.id}`} className={styles.runLabel}>
                      Sample input
                    </label>
                    <select
                      id={`fixture-solo-${entry.id}`}
                      className={styles.fixturePicker}
                      value={selectedFixture}
                      onChange={(e) => setSelectedFixture(e.target.value)}
                    >
                      {ALL_FIXTURES.map((name) => (
                        <option key={name} value={name}>
                          {name}
                        </option>
                      ))}
                    </select>
                  </div>
                  <button
                    type="button"
                    className={`${styles.runButton} ${styles.runButtonStandalone}`}
                    onClick={handleRunConfig}
                    title="Configure and run this tool in your browser"
                  >
                    <span className={styles.runIcon} aria-hidden="true">
                      &#9654;
                    </span>
                    Configure and run in your browser
                  </button>
                </div>
              ) : whyNotRunnable ? (
                <p className={styles.notRunnableHint}>{whyNotRunnable}</p>
              ) : null
            }
          </BrowserOnly>
        </div>
      ) : (
        <div className={styles.configGrid}>
          <section className={styles.panel}>
            <div className={styles.panelHeader}>
              <span className={styles.panelTitle}>Configuration</span>
            </div>
            <div className={styles.panelBody}>
              <BrowserOnly fallback={<div className={styles.formFallback}>Loading form…</div>}>
                {() => (
                  <SchemaForm
                    schema={schema!}
                    values={values}
                    onChange={onChange}
                    presetValues={baseline}
                    paramDocs={doc?.parameters}
                    hideHeader
                    compact
                  />
                )}
              </BrowserOnly>
            </div>
          </section>

          <aside className={`${styles.panel} ${styles.outputPanel}`}>
            <div className={styles.panelHeader}>
              <span className={styles.panelTitle}>Configuration output</span>
              <button
                type="button"
                className={styles.copyButton}
                onClick={copyYaml}
                title="Copy the YAML configuration"
              >
                {copied ? "Copied" : "Copy YAML"}
              </button>
            </div>
            <div className={styles.panelBody}>
              <div className={styles.outputStatus}>
                <span className={styles.baselineChip}>vs {baselineLabel}</span>
                {dirtyCount > 0 ? (
                  <>
                    <span className={styles.dirtyBadge}>{dirtyCount} changed</span>
                    <button type="button" className={styles.revertButton} onClick={revert}>
                      Revert changes
                    </button>
                  </>
                ) : (
                  <span className={styles.cleanNote}>no changes</span>
                )}
              </div>

              <pre className={styles.yaml}>
                {yamlLines.map((line, i) => (
                  <div
                    key={i}
                    className={
                      line.key && dirtyKeys.has(line.key) ? styles.yamlLineDirty : undefined
                    }
                  >
                    {line.text || " "}
                  </div>
                ))}
              </pre>

              {/* Run this config */}
              <BrowserOnly>
                {() =>
                  runOptions ? (
                    <div className={styles.runSection}>
                      <div className={styles.runRow}>
                        <label htmlFor={`fixture-${entry.id}`} className={styles.runLabel}>
                          Sample input
                        </label>
                        <select
                          id={`fixture-${entry.id}`}
                          className={styles.fixturePicker}
                          value={selectedFixture}
                          onChange={(e) => setSelectedFixture(e.target.value)}
                        >
                          {ALL_FIXTURES.map((name) => (
                            <option key={name} value={name}>
                              {name}
                            </option>
                          ))}
                        </select>
                      </div>
                      <button
                        type="button"
                        className={styles.runButton}
                        onClick={handleRunConfig}
                        title="Configure and run this tool in your browser"
                      >
                        <span className={styles.runIcon} aria-hidden="true">
                          &#9654;
                        </span>
                        Configure and run in your browser
                      </button>
                    </div>
                  ) : whyNotRunnable ? (
                    <p className={styles.notRunnableHint}>{whyNotRunnable}</p>
                  ) : null
                }
              </BrowserOnly>
            </div>
          </aside>
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
