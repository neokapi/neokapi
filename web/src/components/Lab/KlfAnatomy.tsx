import React, { useMemo, useState } from "react";
import Link from "@docusaurus/Link";
import { tokenize } from "@neokapi/ui-primitives/preview";
import type { Token } from "@neokapi/ui-primitives/preview";
import { ANATOMY_LINES, ANATOMY_TEXT, TERMS, termById, type TermId } from "./klfAnatomyDoc";
import styles from "./KlfAnatomy.module.css";

// KlfAnatomy — the worked example on the KLF anatomy page: a two-pane reading
// of one realistic .klf document. The left pane is the highlighted source,
// every line tagged with the anatomical part it belongs to; selecting a part
// (by clicking a line, or a term in the right pane's list) highlights its
// lines and shows the structured explanation. Static by construction: no
// engine, no network — the interactive round-trip lives further down the page.

export default function KlfAnatomy(): React.ReactElement {
  const [selected, setSelected] = useState<TermId>("envelope");

  // Tokenize the whole document once; lines align 1:1 with ANATOMY_LINES.
  const tokenLines = useMemo(() => tokenize(ANATOMY_TEXT.trimEnd(), "json"), []);
  const term = termById(selected);

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
      {/* Source pane */}
      <div
        className={`${styles.code} overflow-auto rounded-lg border bg-card`}
        style={{ maxHeight: "34rem" }}
        aria-label="Annotated .klf source; select a line to explain the part it belongs to"
      >
        <pre className="m-0 bg-transparent py-2 font-mono text-[0.78rem] leading-[1.55]">
          <code>
            {tokenLines.map((tokens, i) => {
              const lineTerm = ANATOMY_LINES[i]?.term;
              const active = lineTerm === selected;
              return (
                <span
                  key={i}
                  onClick={() => lineTerm && setSelected(lineTerm)}
                  className={`flex cursor-pointer items-start pr-3 ${
                    active
                      ? "bg-primary/10 shadow-[inset_3px_0_0] shadow-primary"
                      : "hover:bg-muted/60"
                  }`}
                >
                  <span className="mr-3 w-9 shrink-0 border-r border-border pr-2 text-right text-muted-foreground/70 tabular-nums select-none">
                    {i + 1}
                  </span>
                  <span className="flex-1 whitespace-pre">
                    {tokens.length === 0 ? (
                      <span> </span>
                    ) : (
                      tokens.map((t, j) => <TokenSpan key={j} token={t} />)
                    )}
                  </span>
                </span>
              );
            })}
          </code>
        </pre>
      </div>

      {/* Explanation pane */}
      <div className="min-w-0 lg:sticky lg:top-20 lg:self-start">
        <nav aria-label="Anatomy terms" className="mb-3 flex flex-wrap gap-1.5">
          {TERMS.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => setSelected(t.id)}
              className={`rounded-md border px-2 py-0.5 font-mono text-[0.72rem] transition-colors ${
                t.id === selected
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border bg-transparent text-muted-foreground hover:border-primary/60"
              }`}
            >
              {t.title}
            </button>
          ))}
        </nav>
        <div className="rounded-lg border bg-card p-4">
          <h3 className="mb-1 text-base font-semibold text-foreground">{term.title}</h3>
          {term.body.map((p, i) => (
            <p key={i} className="mb-2 text-sm leading-relaxed text-muted-foreground">
              {p}
            </p>
          ))}
          <p className="mb-0 text-sm">
            <Link to={term.spec}>Normative definition in the specification</Link>
          </p>
        </div>
      </div>
    </div>
  );
}

function TokenSpan({ token }: { token: Token }): React.ReactElement {
  const cls = styles[`t_${token.type}`];
  if (!cls) return <>{token.text}</>;
  return <span className={cls}>{token.text}</span>;
}
