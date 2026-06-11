import React, { useMemo, useState } from "react";

import styles from "./CsvTable.module.css";

interface CsvTableProps {
  /** The raw CSV text. The first row is treated as the header. */
  csv: string;
  /** Optional caption rendered above the view toggle. */
  caption?: string;
}

type View = "table" | "raw";

/**
 * Parse CSV text into rows of fields. Handles the common cases this docs glossary
 * needs: a header row, plain values, and quoted fields that may contain commas
 * or escaped double-quotes (`""`). Apostrophes inside unquoted values (e.g.
 * `nom d'utilisateur`) are left untouched. Blank lines are skipped.
 */
function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let field = "";
  let row: string[] = [];
  let inQuotes = false;

  const pushField = () => {
    row.push(field);
    field = "";
  };
  const pushRow = () => {
    pushField();
    // Skip fully-empty rows (e.g. a trailing newline).
    if (row.length > 1 || row[0] !== "") {
      rows.push(row);
    }
    row = [];
  };

  const normalized = text.replace(/\r\n?/g, "\n");
  for (let i = 0; i < normalized.length; i++) {
    const ch = normalized[i];
    if (inQuotes) {
      if (ch === '"') {
        if (normalized[i + 1] === '"') {
          field += '"';
          i++; // consume the escaped quote
        } else {
          inQuotes = false;
        }
      } else {
        field += ch;
      }
    } else if (ch === '"') {
      inQuotes = true;
    } else if (ch === ",") {
      pushField();
    } else if (ch === "\n") {
      pushRow();
    } else {
      field += ch;
    }
  }
  // Flush the final field/row if the text didn't end with a newline.
  if (field !== "" || row.length > 0) {
    pushRow();
  }

  return rows;
}

/**
 * Render a single raw CSV line with a light syntax treatment: the header row is
 * bold and commas are dimmed so the column structure reads at a glance.
 */
function renderRawLine(line: string, isHeader: boolean, key: number): React.ReactNode {
  const parts = line.split(",");
  return (
    <div key={key} className={isHeader ? styles.rawHeaderLine : styles.rawLine}>
      {parts.map((part, idx) => (
        <React.Fragment key={idx}>
          <span>{part}</span>
          {idx < parts.length - 1 && <span className={styles.rawComma}>,</span>}
        </React.Fragment>
      ))}
    </div>
  );
}

/**
 * CsvTable renders CSV text either as a themed table (default) or as the original
 * raw text, toggled by a small segmented control. SSR-safe: no browser-only APIs
 * run during render, so it hydrates cleanly under Docusaurus.
 */
export default function CsvTable({ csv, caption }: CsvTableProps): React.ReactElement {
  const [view, setView] = useState<View>("table");

  const source = csv.trim();
  const rows = useMemo(() => parseCsv(source), [source]);
  const header = rows[0] ?? [];
  const body = rows.slice(1);
  const rawLines = useMemo(() => source.split("\n"), [source]);

  return (
    <figure className={styles.figure}>
      <div className={styles.toolbar}>
        {caption && <figcaption className={styles.caption}>{caption}</figcaption>}
        <div className={styles.toggle} role="group" aria-label="Glossary view">
          <button
            type="button"
            className={view === "table" ? styles.toggleActive : styles.toggleButton}
            aria-pressed={view === "table"}
            onClick={() => setView("table")}
          >
            Table
          </button>
          <button
            type="button"
            className={view === "raw" ? styles.toggleActive : styles.toggleButton}
            aria-pressed={view === "raw"}
            onClick={() => setView("raw")}
          >
            Raw
          </button>
        </div>
      </div>

      {view === "table" ? (
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                {header.map((cell, idx) => (
                  <th key={idx}>{cell}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {body.map((cells, rowIdx) => (
                <tr key={rowIdx}>
                  {header.map((_, colIdx) => (
                    <td key={colIdx}>{cells[colIdx] ?? ""}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <pre className={styles.raw}>
          {rawLines.map((line, idx) => renderRawLine(line, idx === 0, idx))}
        </pre>
      )}
    </figure>
  );
}
