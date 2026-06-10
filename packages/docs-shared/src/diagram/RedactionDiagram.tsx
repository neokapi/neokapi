import React from "react";
import "./diagram.css";

/*
  RedactionDiagram — the marker-redaction visual. Sensitive spans are struck out
  the way you'd black them out with a felt-tip marker: a solid bar with rough,
  slightly skewed edges (an SVG turbulence/displacement filter + a small per-bar
  rotation — no rough.js dependency, no runtime JS, SSR-safe). The bar fills with
  `--kdx-ink`, so it reads black on light and inverts to a light bar on dark.

  Not every span is fully sensitive: a span can instead carry a `label`
  (category / placeholder), which renders as a rough-outlined chip showing the
  category — "we hid the value but tell you what kind it was". Pass a plain
  string to black a span out completely, or `{ text, label }` to show the
  category.

      <RedactionDiagram
        original="Mr Bean is the new King of England"
        redact={[
          { text: "Mr Bean", label: "Person" }, // placeholder chip
          "King of England",                    // fully blacked out
        ]}
        translated="M. Bean est le nouveau Roi d'Angleterre"
      />

  Three stacked rows — original source → redacted (what the model / translator
  sees) → restored translation — joined by labeled edges, in the same kdx visual
  language as the rest of the kit. Pure SVG + CSS, themed for light/dark.
*/

/** A span to redact: a bare string is blacked out; `{text, label}` shows the category. */
export type RedactSpan = string | { text: string; label?: string };

export interface RedactionDiagramProps {
  /** The source sentence, shown in full on the top row. */
  original: string;
  /** Spans of `original` to redact (matched in order, non-overlapping). */
  redact: RedactSpan[];
  /** The translated sentence with originals restored, shown on the bottom row. */
  translated: string;
  /** Edge label for original → redacted. Default "redact". */
  redactLabel?: string;
  /** Edge label for redacted → restored. Default "translate, then unredact". */
  restoreLabel?: string;
  caption?: string;
}

type Token = { text: string; redacted: boolean; label?: string };

const spanText = (s: RedactSpan): string => (typeof s === "string" ? s : s.text);
const spanLabel = (s: RedactSpan): string | undefined =>
  typeof s === "string" ? undefined : s.label;

/** Split `s` into alternating plain / redacted tokens by scanning for `spans` in order. */
function tokenize(s: string, spans: RedactSpan[]): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  for (const span of spans) {
    const text = spanText(span);
    const at = s.indexOf(text, i);
    if (at < 0) continue; // span not found from here — skip it
    if (at > i) tokens.push({ text: s.slice(i, at), redacted: false });
    tokens.push({ text, redacted: true, label: spanLabel(span) });
    i = at + text.length;
  }
  if (i < s.length) tokens.push({ text: s.slice(i), redacted: false });
  return tokens.length ? tokens : [{ text: s, redacted: false }];
}

/** Stable hash → deterministic angle/seed, so SSR and client render identically. */
function hash(s: string, salt: number): number {
  let h = salt * 131 + 7;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h;
}

const ADV = 9; // monospace advance at 15px (0.6em)
const FS = 15; // mono font-size
const LABEL_FS = 12.5;
const LABEL_CHAR = 7.2; // approx px per label char
const PAD = 16;
const BOX_PAD_X = 16;
const ROW_H = 40;
const V_GAP = 50; // edge length between rows
const TOP = 12;
const INSET = 3; // gap between a redaction and the adjacent words

interface Placed {
  tok: Token;
  /** Slot width (advance to the next token). */
  w: number;
  /** Left edge within the text band. */
  x0: number;
}

export function RedactionDiagram({
  original,
  redact,
  translated,
  redactLabel = "redact",
  restoreLabel = "translate, then unredact",
  caption,
}: RedactionDiagramProps): React.ReactElement {
  const tokens = tokenize(original, redact);

  // Lay the redacted row out left→right; a labeled chip may be wider than its
  // span, so the row can run wider than the original — that drives the box width.
  const placed: Placed[] = [];
  let acc = 0;
  for (const tok of tokens) {
    let w = tok.text.length * ADV;
    if (tok.redacted && tok.label) {
      w = Math.max(w, tok.label.length * LABEL_CHAR + 22);
    }
    placed.push({ tok, w, x0: acc });
    acc += w;
  }
  const redactedRowW = acc;

  const innerW = Math.max(original.length * ADV, translated.length * ADV, redactedRowW);
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

  // One shared roughen filter; bars at different positions sample different
  // noise, so a single filter still gives each bar its own ragged edge.
  const fid = `kdx-marker-${hash(original, 0).toString(36)}`;
  const cy1 = rowY(1) + ROW_H / 2;

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 420), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label="Redaction: sensitive spans are blacked out with a marker (categories shown where known) before translation, then restored locally"
          >
            <defs>
              <filter id={fid} x="-10%" y="-30%" width="120%" height="160%">
                <feTurbulence
                  type="fractalNoise"
                  baseFrequency="0.018 0.035"
                  numOctaves={2}
                  seed={2}
                  result="noise"
                />
                <feDisplacementMap
                  in="SourceGraphic"
                  in2="noise"
                  scale={1.8}
                  xChannelSelector="R"
                  yChannelSelector="G"
                />
              </filter>
            </defs>

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

            {/* row 1 — redacted: plain text, marker bars, labeled chips */}
            <rect
              className="kdx-box kdx-box--qa"
              x={xLeft}
              y={rowY(1)}
              width={rowW}
              height={ROW_H}
              rx={8}
            />
            {placed.map(({ tok, w, x0 }, k) => {
              const x = textX + x0;
              if (!tok.redacted) {
                return (
                  <text
                    key={`t-${k}`}
                    className="kdx-mono"
                    x={x}
                    y={cy1 + 5}
                    fontSize={FS}
                    textLength={w}
                    lengthAdjust="spacingAndGlyphs"
                  >
                    {tok.text}
                  </text>
                );
              }
              const bx = x + INSET;
              const bw = Math.max(10, w - 2 * INSET);
              const h = hash(tok.text, k + 1);
              const angle = ((h % 7) - 3) * 0.5; // -1.5°…+1.5°
              const rot = `rotate(${angle.toFixed(2)} ${bx + bw / 2} ${cy1})`;
              if (tok.label) {
                // Placeholder chip: rough-outlined box with the category inside.
                return (
                  <g key={`c-${k}`} transform={rot}>
                    <rect
                      className="kdx-redact-chip"
                      x={bx}
                      y={cy1 - 12}
                      width={bw}
                      height={22}
                      rx={3}
                      filter={`url(#${fid})`}
                    />
                    <text
                      className="kdx-redact-label"
                      x={bx + bw / 2}
                      y={cy1 + 4}
                      textAnchor="middle"
                      fontSize={LABEL_FS}
                    >
                      {tok.label}
                    </text>
                  </g>
                );
              }
              // Fully sensitive: solid marker black-out.
              return (
                <g key={`b-${k}`} transform={rot}>
                  <rect
                    className="kdx-blackout"
                    x={bx}
                    y={cy1 - 11}
                    width={bw}
                    height={20}
                    rx={2}
                    filter={`url(#${fid})`}
                  />
                </g>
              );
            })}
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
