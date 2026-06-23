import { useState, useCallback, useMemo } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import type { ReferenceEntry } from "@neokapi/reference-data";
import { SchemaForm } from "@neokapi/ui-primitives";
import { seedDefaults, buildFormatYamlLines, buildToolYamlLines, yamlText } from "../yaml";
import { buildRunOptions, pickFixture, notRunnableReason } from "../run-config";
import styles from "../styles.module.css";
import pageStyles from "./pages.module.css";

interface Props {
  entry: ReferenceEntry;
  /** "format" | "tool" — controls the YAML shape (recipe block vs flow step). */
  kind: "format" | "tool";
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
 * The interactive configuration block for one format/tool static reference
 * page: preset buttons, a live SchemaForm beside a sticky configuration-output
 * panel ("Copy YAML"), and a "Run this config" action that opens the in-browser
 * playground. This is the live configurator that the per-entry reference detail
 * view once rendered in a modal; it now lives directly on each static page.
 *
 * SSR-safe: the SchemaForm and the playground-driven Run controls are wrapped in
 * {@link BrowserOnly} so they never hydrate on the server. The preset buttons
 * and the YAML output render from deterministic schema defaults, so the initial
 * server and client renders match. Renders `null` when there is nothing to offer
 * (no parameters and not runnable) — the page already shows its own
 * "no configurable parameters" note in that case.
 */
export default function InteractiveConfig({ entry, kind }: Props) {
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
      kind === "format"
        ? buildFormatYamlLines(entry.id, values, schema)
        : buildToolYamlLines(values, schema),
    [kind, entry.id, values, schema],
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

  const hasForm = paramCount > 0;
  // Param-less, non-runnable entries with no hint reason have nothing to show:
  // the page's own "no configurable parameters" note covers them.
  if (!hasForm && !runOptions && !whyNotRunnable) return null;

  return (
    <section className={`${pageStyles.page} kapi-reference`}>
      <h2 className={pageStyles.sectionHeading}>{hasForm ? "Configure it live" : "Run it"}</h2>

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
      {!hasForm ? (
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
    </section>
  );
}
