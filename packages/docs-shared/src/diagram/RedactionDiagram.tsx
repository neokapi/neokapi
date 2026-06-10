import React from "react";
import "./diagram.css";

/*
  RedactionDiagram — the censor-bar visual for redaction. Sensitive spans are
  drawn as solid black-out bars (the kind you'd see on a declassified document),
  not as visible placeholder text or asterisks, so the page shows what actually
  leaves the machine: nothing. The bar fills with `--kdx-ink`, so it reads as a
  black bar on the light theme and inverts to a light bar on dark.

      <RedactionDiagram
        original="Mr Bean is the new King of England"
        redact={["Mr Bean", "King of England"]}
        translated="M. Bean est le nouveau Roi d'Angleterre"
      />

  Three stacked rows — original source → redacted (what the model / translator
  sees) → restored translation — joined by labeled edges, in the same kdx visual
  language as the rest of the kit. Pure SVG + CSS, themed for light/dark with no
  JS, SSR-safe.
*/

export interface RedactionDiagramProps {
  /** The source sentence, shown in full on the top row. */
  original: string;
  /** Substrings of `original` to black out (matched in order, non-overlapping). */
  redact: string[];
  /** The translated sentence with originals restored, shown on the bottom row. */
  translated: string;
  /** Edge label for original → redacted. Default "redact". */
  redactLabel?: string;
  /** Edge label for redacted → restored. Default "translate, then unredact". */
  restoreLabel?: string;
  caption?: string;
}

type Token = { text: string; redacted: boolean };

/** Split `s` into alternating plain / redacted tokens by scanning for `spans` in order. */
function tokenize(s: string, spans: string[]): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  for (const span of spans) {
    const at = s.indexOf(span, i);
    if (at < 0) continue; // span not found from here — skip it
    if (at > i) tokens.push({ text: s.slice(i, at), redacted: false });
    tokens.push({ text: span, redacted: true });
    i = at + span.length;
  }
  if (i < s.length) tokens.push({ text: s.slice(i), redacted: false });
  return tokens.length ? tokens : [{ text: s, redacted: false }];
}

const ADV = 9; // monospace advance at 15px (0.6em)
const FS = 15; // mono font-size
const PAD = 16;
const BOX_PAD_X = 16;
const ROW_H = 40;
const V_GAP = 50; // edge length between rows
const TOP = 12;

export function RedactionDiagram({
  original,
  redact,
  translated,
  redactLabel = "redact",
  restoreLabel = "translate, then unredact",
  caption,
}: RedactionDiagramProps): React.ReactElement {
  const rows = [original, "·".repeat(original.length), translated];
  const innerW = Math.max(...rows.map((r) => r.length)) * ADV;
  const rowW = innerW + 2 * BOX_PAD_X;
  const xLeft = PAD;
  const cx = xLeft + rowW / 2;
  const textX = xLeft + BOX_PAD_X;

  const notes = ["", "what the model / translator sees", "originals restored locally"];
  const noteX = xLeft + rowW + 16;
  const noteW = Math.max(...notes.map((n) => n.length)) * 6.2;
  const totalW = noteX + noteW + PAD;

  const rowY = (i: number) => TOP + i * (ROW_H + V_GAP);
  const totalH = rowY(2) + ROW_H + (caption ? 8 : 12);

  const redactedTokens = tokenize(original, redact);

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 420), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label="Redaction: the source is shown, sensitive spans are blacked out before translation, then restored locally afterward"
          >
            {/* connectors between rows */}
            {[0, 1].map((i) => {
              const y1 = rowY(i) + ROW_H;
              const y2 = rowY(i + 1);
              const my = (y1 + y2) / 2;
              const label = i === 0 ? redactLabel : restoreLabel;
              return (
                <g key={`edge-${i}`}>
                  <path className="kdx-channel" d={`M ${cx} ${y1} L ${cx} ${y2 - 6}`} />
                  <path
                    className="kdx-arrow"
                    d={`M ${cx - 4} ${y2 - 8} L ${cx + 4} ${y2 - 8} L ${cx} ${y2} Z`}
                  />
                  <text className="kdx-chan" x={cx + 12} y={my + 4} fontSize={12.5}>
                    {label}
                  </text>
                </g>
              );
            })}

            {/* row 0 — original source */}
            <rect className="kdx-box" x={xLeft} y={rowY(0)} width={rowW} height={ROW_H} rx={8} />
            <text className="kdx-mono" x={textX} y={rowY(0) + ROW_H / 2 + 5} fontSize={FS}>
              {original}
            </text>

            {/* row 1 — redacted: plain text + black-out bars */}
            <rect
              className="kdx-box kdx-box--qa"
              x={xLeft}
              y={rowY(1)}
              width={rowW}
              height={ROW_H}
              rx={8}
            />
            {(() => {
              const cy = rowY(1) + ROW_H / 2;
              let x = textX;
              const nodes: React.ReactNode[] = [];
              redactedTokens.forEach((tok, k) => {
                const w = tok.text.length * ADV;
                if (tok.redacted) {
                  // Inset the bar so it doesn't butt against adjacent words.
                  nodes.push(
                    <rect
                      key={`bar-${k}`}
                      className="kdx-blackout"
                      x={x + 3}
                      y={cy - 11}
                      width={Math.max(8, w - 6)}
                      height={20}
                      rx={3}
                    />,
                  );
                } else {
                  nodes.push(
                    <text
                      key={`t-${k}`}
                      className="kdx-mono"
                      x={x}
                      y={cy + 5}
                      fontSize={FS}
                      textLength={w}
                      lengthAdjust="spacingAndGlyphs"
                    >
                      {tok.text}
                    </text>,
                  );
                }
                x += w;
              });
              return nodes;
            })()}
            <text className="kdx-note" x={noteX} y={rowY(1) + ROW_H / 2 + 4} fontSize={12.5}>
              {notes[1]}
            </text>

            {/* row 2 — restored translation */}
            <rect
              className="kdx-box kdx-box--translate"
              x={xLeft}
              y={rowY(2)}
              width={rowW}
              height={ROW_H}
              rx={8}
            />
            <text className="kdx-mono" x={textX} y={rowY(2) + ROW_H / 2 + 5} fontSize={FS}>
              {translated}
            </text>
            <text className="kdx-note" x={noteX} y={rowY(2) + ROW_H / 2 + 4} fontSize={12.5}>
              {notes[2]}
            </text>
          </svg>
        </div>
      </div>
      {caption && <p className="kdx-caption">{caption}</p>}
    </div>
  );
}
