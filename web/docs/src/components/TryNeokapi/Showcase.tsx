import React from "react";
import clsx from "clsx";
import {
  MOCK_DOCS,
  insightsFor,
  segmentsFor,
  type DemoId,
  type MockDoc,
  type MockField,
  type Segment,
} from "./mocks";
import styles from "./styles.module.css";

// The FAKED visual showcase: three mock "documents" rendered as a PowerPoint
// slide, an Excel worksheet, and a Markdown doc, with the active demo's
// simulated transform applied and the changed spans highlighted. Pure
// presentation over hardcoded strings — no wasm, no real files. The real engine
// runs only on the separate Download-result / own-files paths.

interface ShowcaseProps {
  demo: DemoId;
  find: string;
  replace: string;
}

function Segments({ segments }: { segments: Segment[] }): React.ReactElement {
  return (
    <>
      {segments.map((s, i) =>
        s.changed ? (
          <mark key={i} className={styles.changed}>
            {s.text}
          </mark>
        ) : (
          <React.Fragment key={i}>{s.text}</React.Fragment>
        ),
      )}
    </>
  );
}

function fieldSegments(demo: DemoId, find: string, replace: string) {
  return (f: MockField) => <Segments segments={segmentsFor(demo, f, find, replace)} />;
}

function SlideCard({ doc, render }: { doc: MockDoc; render: (f: MockField) => React.ReactNode }) {
  const [title, sub, ...bullets] = doc.fields;
  return (
    <div className={clsx(styles.docBody, styles.slide)}>
      <div className={styles.slideTitle}>{render(title)}</div>
      <div className={styles.slideSub}>{render(sub)}</div>
      <ul className={styles.slideBullets}>
        {bullets.map((b) => (
          <li key={b.id}>{render(b)}</li>
        ))}
      </ul>
    </div>
  );
}

function SheetCard({ doc, render }: { doc: MockDoc; render: (f: MockField) => React.ReactNode }) {
  // Pair the fields into rows of [A, B] to mimic a two-column worksheet.
  const rows: MockField[][] = [];
  for (let i = 0; i < doc.fields.length; i += 2) rows.push(doc.fields.slice(i, i + 2));
  return (
    <div className={styles.docBody}>
      <table className={styles.sheet}>
        <tbody>
          <tr className={styles.colHead}>
            <td className={styles.rowHead}></td>
            <td>A</td>
            <td>B</td>
          </tr>
          {rows.map((row, r) => (
            <tr key={r}>
              <td className={styles.rowHead}>{r + 1}</td>
              {row.map((cell) => (
                <td key={cell.id}>{render(cell)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function MarkdownCard({
  doc,
  render,
}: {
  doc: MockDoc;
  render: (f: MockField) => React.ReactNode;
}) {
  const [h1, para, ...list] = doc.fields;
  return (
    <div className={clsx(styles.docBody, styles.md)}>
      <div className={styles.mdH1}>{render(h1)}</div>
      <div>{render(para)}</div>
      <ul className={styles.mdList}>
        {list.map((li) => (
          <li key={li.id}>{render(li)}</li>
        ))}
      </ul>
    </div>
  );
}

function barClass(kind: MockDoc["kind"]): string {
  if (kind === "pptx") return styles.barPptx;
  if (kind === "xlsx") return styles.barXlsx;
  return styles.barMd;
}

function Insights({ doc }: { doc: MockDoc }): React.ReactElement {
  const s = insightsFor(doc);
  return (
    <div className={styles.insights}>
      <span className={styles.statChip}>
        <span className={styles.statNum}>{s.blocks}</span>
        <span className={styles.statLabel}>blocks</span>
      </span>
      <span className={styles.statChip}>
        <span className={styles.statNum}>{s.words}</span>
        <span className={styles.statLabel}>words</span>
      </span>
      <span className={styles.statChip}>
        <span className={styles.statNum}>{s.characters}</span>
        <span className={styles.statLabel}>chars</span>
      </span>
    </div>
  );
}

export default function Showcase({ demo, find, replace }: ShowcaseProps): React.ReactElement {
  const render = fieldSegments(demo, find, replace);
  return (
    <div className={styles.grid}>
      {MOCK_DOCS.map((doc) => (
        <div key={doc.kind} className={styles.doc}>
          <div className={clsx(styles.docBar, barClass(doc.kind))}>
            {doc.title}
            <span className={styles.docName}>{doc.filename}</span>
          </div>
          {doc.kind === "pptx" && <SlideCard doc={doc} render={render} />}
          {doc.kind === "xlsx" && <SheetCard doc={doc} render={render} />}
          {doc.kind === "md" && <MarkdownCard doc={doc} render={render} />}
          {demo === "insights" && (
            <div style={{ padding: "0 0.75rem 0.7rem" }}>
              <Insights doc={doc} />
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
