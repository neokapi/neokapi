import type { ComponentSchema, ReferenceDoc, PropertySchema } from "@neokapi/reference-data";
import styles from "./pages.module.css";

interface Props {
  schema: ComponentSchema | undefined;
  doc: ReferenceDoc | undefined;
}

/** Render a default value compactly. */
function fmtDefault(v: unknown): string {
  if (v === undefined || v === null) return "";
  if (typeof v === "string") return v === "" ? '""' : v;
  if (typeof v === "boolean" || typeof v === "number") return String(v);
  if (Array.isArray(v)) return v.length === 0 ? "[]" : JSON.stringify(v);
  return JSON.stringify(v);
}

/** Human-readable type label, surfacing enum membership. */
function typeLabel(p: PropertySchema): string {
  if (p.enum && p.enum.length > 0) {
    return p.enum.map((e) => String(e)).join(" | ");
  }
  if (p.type === "array" && p.items?.type) return `${p.items.type}[]`;
  return p.type;
}

/**
 * A static parameters table built from a SchemaForm `ComponentSchema`'s
 * `properties`, enriched with per-parameter descriptions from the reference
 * `doc.parameters` when present. Used by the format and tool static pages.
 * Hidden (returns null) when the schema declares no properties.
 */
export default function ParametersTable({ schema, doc }: Props) {
  const props = schema?.properties ?? {};
  const names = Object.keys(props);
  if (names.length === 0) return null;

  const paramDocs = doc?.parameters ?? {};

  // Stable order: respect ui:order when set, else alphabetical.
  names.sort((a, b) => {
    const oa = props[a]["ui:order"];
    const ob = props[b]["ui:order"];
    if (oa !== undefined && ob !== undefined) return oa - ob;
    if (oa !== undefined) return -1;
    if (ob !== undefined) return 1;
    return a.localeCompare(b);
  });

  return (
    <table className={styles.paramTable}>
      <thead>
        <tr>
          <th>Parameter</th>
          <th>Type</th>
          <th>Default</th>
          <th>Description</th>
        </tr>
      </thead>
      <tbody>
        {names.map((name) => {
          const p = props[name];
          const pd = paramDocs[name];
          const desc = p.description || pd?.description || pd?.help || "";
          return (
            <tr key={name}>
              <td className={styles.paramName}>
                <code>{name}</code>
                {p.deprecated && <span className={styles.deprecated}>deprecated</span>}
              </td>
              <td className={styles.paramType}>{typeLabel(p)}</td>
              <td className={styles.paramDefault}>{fmtDefault(p.default)}</td>
              <td className={styles.paramDesc}>{desc}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
