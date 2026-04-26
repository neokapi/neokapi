import { useState, useCallback, useMemo } from "react";
import type { FormatDoc, GroupDoc, PropDoc } from "./_types";
import styles from "./_index.module.css";

interface Props {
  format: FormatDoc;
  defaultExpanded?: boolean;
}

/** PascalCase a format ID: "html" → "Html", "csv" → "Csv". */
function pascalCase(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** Build YAML config output with the full kind/version envelope. */
function buildYaml(
  formatId: string,
  values: Record<string, any>,
  properties: Record<string, PropDoc>,
): string {
  const kind = `${pascalCase(formatId)}FormatConfig`;
  const lines = [
    `apiVersion: neokapi/v1`,
    `kind: ${kind}`,
    `metadata:`,
    `  name: ${formatId}`,
    `spec:`,
  ];

  const entries = Object.entries(values).filter(([key, val]) => {
    const prop = properties[key];
    if (!prop) return false;
    if (prop.default !== undefined && val === prop.default) return false;
    if (val === "" || (Array.isArray(val) && val.length === 0)) return false;
    return true;
  });

  if (entries.length === 0) {
    lines.push(`  # (default configuration)`);
    return lines.join("\n");
  }

  for (const [key, val] of entries.sort(([a], [b]) => a.localeCompare(b))) {
    if (typeof val === "boolean") {
      lines.push(`  ${key}: ${val}`);
    } else if (typeof val === "string") {
      lines.push(`  ${key}: "${val}"`);
    } else if (Array.isArray(val)) {
      if (val.length === 0) continue;
      lines.push(`  ${key}:`);
      for (const item of val) {
        lines.push(`    - ${JSON.stringify(item)}`);
      }
    } else {
      lines.push(`  ${key}: ${JSON.stringify(val)}`);
    }
  }
  return lines.join("\n");
}

export default function FormatCard({ format, defaultExpanded }: Props) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? false);
  const [values, setValues] = useState<Record<string, any>>(() => initValues(format.properties));
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(
    () => new Set((format.groups ?? []).filter((g) => g.collapsed).map((g) => g.id)),
  );
  const [copied, setCopied] = useState(false);
  const [activePreset, setActivePreset] = useState<string | null>(null);

  const props = format.properties ?? {};
  const paramCount = Object.keys(props).length;

  const toggleExpand = useCallback(() => setExpanded((p) => !p), []);

  const toggleGroup = useCallback((groupId: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(groupId)) next.delete(groupId);
      else next.add(groupId);
      return next;
    });
  }, []);

  const setValue = useCallback((key: string, val: any) => {
    setValues((prev) => ({ ...prev, [key]: val }));
    setActivePreset(null);
  }, []);

  const resetValues = useCallback(() => {
    setValues(initValues(format.properties));
    setActivePreset(null);
  }, [format.properties]);

  const applyPreset = useCallback(
    (presetId: string, params: Record<string, any>) => {
      setValues((prev) => {
        const next = initValues(format.properties);
        return { ...next, ...params };
      });
      setActivePreset(presetId);
    },
    [format.properties],
  );

  const yamlOutput = useMemo(() => buildYaml(format.id, values, props), [format.id, values, props]);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(yamlOutput).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [yamlOutput]);

  // Determine which fields are rendered in groups vs ungrouped.
  const groupedFields = new Set((format.groups ?? []).flatMap((g) => g.fields));
  const ungroupedFields = Object.keys(props).filter((f) => !groupedFields.has(f));

  return (
    <div className={`${styles.formatCard} ${expanded ? styles.formatCardExpanded : ""}`}>
      <div className={styles.formatHeader} onClick={toggleExpand}>
        <span className={`${styles.expandIcon} ${expanded ? styles.expandIconOpen : ""}`}>
          &#9654;
        </span>
        <span className={styles.formatName}>{format.displayName}</span>
        <span className={styles.formatMeta}>
          {format.extensions?.map((ext) => (
            <span key={ext} className={styles.extBadge}>
              {ext}
            </span>
          ))}
          {format.hasReader && (
            <span className={`${styles.capBadge} ${styles.capReader}`}>Reader</span>
          )}
          {format.hasWriter && (
            <span className={`${styles.capBadge} ${styles.capWriter}`}>Writer</span>
          )}
        </span>
        {paramCount > 0 && (
          <span className={styles.configCount}>
            {paramCount} parameter{paramCount !== 1 ? "s" : ""}
          </span>
        )}
      </div>

      {expanded && (
        <div className={styles.formatBody}>
          {format.description && <p className={styles.formatDescription}>{format.description}</p>}

          <div className={styles.metaGrid}>
            {format.mimeTypes && format.mimeTypes.length > 0 && (
              <div className={styles.metaSection}>
                <span className={styles.metaLabel}>MIME Types</span>
                <span className={styles.metaValue}>{format.mimeTypes.join(", ")}</span>
              </div>
            )}
            <div className={styles.metaSection}>
              <span className={styles.metaLabel}>Format ID</span>
              <span className={styles.metaValue}>{format.id}</span>
            </div>
          </div>

          {/* Presets */}
          {format.presets && format.presets.length > 0 && (
            <div className={styles.presetSection}>
              <div className={styles.presetTitle}>Presets</div>
              <div className={styles.presetButtons}>
                <button
                  className={`${styles.presetButton} ${activePreset === null ? styles.presetButtonActive : ""}`}
                  onClick={() => resetValues()}
                  title="Default configuration"
                >
                  Default
                </button>
                {format.presets.map((preset) => (
                  <button
                    key={preset.id}
                    className={`${styles.presetButton} ${activePreset === preset.id ? styles.presetButtonActive : ""}`}
                    onClick={() => applyPreset(preset.id, preset.parameters)}
                    title={preset.description}
                  >
                    {preset.name}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Parameters */}
          {paramCount === 0 ? (
            <p className={styles.noConfig}>This format has no configurable parameters.</p>
          ) : (
            <>
              {(format.groups ?? []).map((group) => (
                <ParameterGroup
                  key={group.id}
                  group={group}
                  properties={props}
                  values={values}
                  collapsed={collapsedGroups.has(group.id)}
                  onToggle={() => toggleGroup(group.id)}
                  onChange={setValue}
                />
              ))}
              {ungroupedFields.length > 0 &&
                ungroupedFields.map((field) => (
                  <ParameterRow
                    key={field}
                    name={field}
                    prop={props[field]}
                    value={values[field]}
                    onChange={(val) => setValue(field, val)}
                  />
                ))}

              {/* YAML output */}
              <div className={styles.outputSection}>
                <div className={styles.outputHeader}>
                  <span className={styles.outputTitle}>Configuration Output</span>
                  <div className={styles.outputActions}>
                    <button className={styles.resetButton} onClick={resetValues}>
                      Reset
                    </button>
                    <button className={styles.copyButton} onClick={handleCopy}>
                      {copied ? "Copied!" : "Copy YAML"}
                    </button>
                  </div>
                </div>
                <pre className={styles.yamlOutput}>{yamlOutput}</pre>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}

function ParameterGroup({
  group,
  properties,
  values,
  collapsed,
  onToggle,
  onChange,
}: {
  group: GroupDoc;
  properties: Record<string, PropDoc>;
  values: Record<string, any>;
  collapsed: boolean;
  onToggle: () => void;
  onChange: (key: string, val: any) => void;
}) {
  // Only show fields that exist in properties.
  const fields = group.fields.filter((f) => properties[f]);
  if (fields.length === 0) return null;

  return (
    <div className={styles.paramGroup}>
      <div className={styles.groupHeader} onClick={onToggle}>
        <span className={`${styles.groupToggle} ${!collapsed ? styles.groupToggleOpen : ""}`}>
          &#9654;
        </span>
        <span className={styles.groupLabel}>{group.label}</span>
      </div>
      {!collapsed &&
        fields.map((field) => (
          <ParameterRow
            key={field}
            name={field}
            prop={properties[field]}
            value={values[field]}
            onChange={(val) => onChange(field, val)}
          />
        ))}
    </div>
  );
}

function ParameterRow({
  name,
  prop,
  value,
  onChange,
}: {
  name: string;
  prop: PropDoc;
  value: any;
  onChange: (val: any) => void;
}) {
  const isModified = prop.default !== undefined && value !== prop.default;

  return (
    <div className={styles.paramRow}>
      <div className={styles.paramInfo}>
        <span className={styles.paramName}>
          {name}
          {isModified && <span className={styles.modifiedIndicator}> *</span>}
        </span>
        <span className={styles.paramType}>{prop.type}</span>
      </div>
      <div className={styles.paramControl}>
        {prop.type === "boolean" ? (
          <div className={styles.checkbox}>
            <input
              type="checkbox"
              id={`param-${name}`}
              checked={!!value}
              onChange={(e) => onChange(e.target.checked)}
            />
            <label htmlFor={`param-${name}`}>{prop.description}</label>
          </div>
        ) : (
          <>
            <p className={styles.paramDesc}>{prop.description}</p>
            {prop.type === "string" && (
              <input
                type="text"
                className={styles.textInput}
                value={value ?? ""}
                placeholder={prop.default !== undefined ? String(prop.default) : undefined}
                onChange={(e) => onChange(e.target.value)}
              />
            )}
            {(prop.type === "array" || prop.type === "object") && (
              <input
                type="text"
                className={styles.textInput}
                value={Array.isArray(value) ? value.join(", ") : (value ?? "")}
                placeholder={`Enter ${prop.type === "array" ? "comma-separated values" : "JSON object"}`}
                onChange={(e) => {
                  const raw = e.target.value;
                  if (prop.type === "array") {
                    onChange(
                      raw
                        ? raw
                            .split(",")
                            .map((s) => s.trim())
                            .filter(Boolean)
                        : [],
                    );
                  } else {
                    onChange(raw);
                  }
                }}
              />
            )}
          </>
        )}
      </div>
    </div>
  );
}

function initValues(properties: Record<string, PropDoc> | undefined): Record<string, any> {
  if (!properties) return {};
  const result: Record<string, any> = {};
  for (const [key, prop] of Object.entries(properties)) {
    if (prop.default !== undefined) {
      result[key] = prop.default;
    } else if (prop.type === "boolean") {
      result[key] = false;
    } else if (prop.type === "string") {
      result[key] = "";
    } else if (prop.type === "array") {
      result[key] = [];
    } else {
      result[key] = "";
    }
  }
  return result;
}
