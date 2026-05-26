import React from "react";
import "./diagram.css";

/*
  ArchitectureDiagram — the hero for the framework Architecture page.

  It tells the whole story in one frame, left to right:

      sources ─▶ Reader ─chan─▶ Annotate ─chan─▶ ⟨fan-out⟩ AI Translate ⟨fan-in⟩
              ─chan─▶ QA ─chan─▶ Writer ─▶ targets

  with three things layered on:
    · resources (TM, termbase) feed annotate/translate from above;
    · the gRPC plugin band (Okapi bridge, kapi-sat, remote) feeds reader,
      annotate and a tool stage from below;
    · concurrency is shown literally — each stage is a goroutine joined by
      `chan Part`, the translate stage fans out across N goroutines with an
      ordered fan-in, and the whole pipeline is replicated for documents run
      in parallel (the ghost lanes behind it).

  Pure SVG + CSS: it renders server-side and re-themes for light/dark with no
  JS. Motion is opt-in via `animated` and is fully suppressed under
  prefers-reduced-motion (see diagram.css).
*/

const VIEW_W = 1000;
const VIEW_H = 520;
const CY = 196; // pipeline centerline

const WORKERS = [0, 1, 2, 3];
const WORKER_X = 440;
const WORKER_W = 152;
const WORKER_H = 28;
const WORKER_GAP = 10;
const workerY = (i: number) => 125 + i * (WORKER_H + WORKER_GAP);

const SOURCES = ["app.json", "page.html", "guide.docx", "strings.xml"];
const TARGETS = ["app.fr.json", "app.de.json", "app.ja.json"];
const sourceY = (i: number) => 112 + i * 40;
const targetY = (i: number) => 152 + i * 40;

interface DotProps {
  path: string;
  dur: number;
  begin: number;
  cls: string;
  r?: number;
}

/** A single Part travelling along a channel (SMIL animateMotion). */
function FlowDot({ path, dur, begin, cls, r = 3 }: DotProps): React.ReactElement {
  return (
    <circle r={r} className={`kdx-dot ${cls}`}>
      <animateMotion dur={`${dur}s`} begin={`${begin}s`} repeatCount="indefinite" path={path} />
      <animate
        attributeName="opacity"
        values="0;1;1;0"
        keyTimes="0;0.12;0.85;1"
        dur={`${dur}s`}
        begin={`${begin}s`}
        repeatCount="indefinite"
      />
    </circle>
  );
}

/** A pipeline stage: titled, role-colored box with a mono subtitle. */
function Stage({
  x,
  w,
  role,
  title,
  sub,
}: {
  x: number;
  w: number;
  role: "io" | "annotate" | "translate" | "qa";
  title: string;
  sub: string;
}): React.ReactElement {
  const cx = x + w / 2;
  return (
    <g>
      <rect x={x} y={164} width={w} height={64} rx={10} className={`kdx-box kdx-box--${role}`} />
      <text x={cx} y={192} textAnchor="middle" fontSize={13.5} className="kdx-label">
        {title}
      </text>
      <text x={cx} y={210} textAnchor="middle" fontSize={9.5} className={`kdx-sub kdx-sub--${role}`}>
        {sub}
      </text>
    </g>
  );
}

/** A channel hop: short line + "chan Part" caption above it. */
function Channel({ x1, x2 }: { x1: number; x2: number }): React.ReactElement {
  return (
    <g>
      <line x1={x1} y1={CY} x2={x2} y2={CY} className="kdx-channel" />
      <text x={(x1 + x2) / 2} y={CY - 8} textAnchor="middle" fontSize={7.5} className="kdx-chan">
        chan
      </text>
    </g>
  );
}

export interface ArchitectureDiagramProps {
  /** Animate the flowing Parts and the fan-out pulse. Default true. */
  animated?: boolean;
}

export function ArchitectureDiagram({
  animated = true,
}: ArchitectureDiagramProps): React.ReactElement {
  return (
    <div className={`kdx${animated ? " kdx--animated" : ""}`}>
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ maxWidth: 960 }}>
          <svg
            viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-labelledby="kdx-arch-title kdx-arch-desc"
          >
            <title id="kdx-arch-title">neokapi processing architecture</title>
            <desc id="kdx-arch-desc">
              A streaming pipeline: format readers and writers at the edges, a serial chain of
              annotate, translate and QA tools in the middle with the translate stage fanning out
              across parallel goroutines, translation-memory and termbase resources feeding it from
              above, and a gRPC plugin band (Okapi bridge, kapi-sat segmenter, remote plugins)
              feeding it from below. Each stage is a goroutine joined by Part channels, and the whole
              pipeline runs over many documents in parallel.
            </desc>

            {/* ── document-parallelism: ghost lanes behind the live one ── */}
            <rect x={120} y={80} width={770} height={210} rx={18} className="kdx-ghost" />
            <rect x={110} y={88} width={770} height={210} rx={18} className="kdx-ghost" />
            <rect
              x={100}
              y={96}
              width={770}
              height={210}
              rx={18}
              className="kdx-channel"
              fill="none"
              opacity={0.4}
            />
            <text x={862} y={74} textAnchor="end" fontSize={9} className="kdx-note">
              documents in parallel · MaxConcurrency
            </text>

            {/* ── resources (top) ── */}
            <g>
              {/* termbase over annotate */}
              <rect x={252} y={18} width={124} height={38} rx={8} className="kdx-chip kdx-chip--resource" />
              <text x={314} y={35} textAnchor="middle" fontSize={11} className="kdx-chip-t">
                Termbase
              </text>
              <text x={314} y={48} textAnchor="middle" fontSize={8} className="kdx-chip-sub">
                term-lookup
              </text>
              <path d="M314,56 L314,164" className="kdx-link kdx-link--resource" />

              {/* TM over translate */}
              <rect x={454} y={18} width={124} height={38} rx={8} className="kdx-chip kdx-chip--resource" />
              <text x={516} y={35} textAnchor="middle" fontSize={11} className="kdx-chip-t">
                Translation Memory
              </text>
              <text x={516} y={48} textAnchor="middle" fontSize={8} className="kdx-chip-sub">
                tm-leverage
              </text>
              <path d="M516,56 L516,114" className="kdx-link kdx-link--resource" />
            </g>

            {/* ── sources (left) ── */}
            <text x={60} y={98} textAnchor="middle" fontSize={9.5} className="kdx-cap">
              Sources
            </text>
            {SOURCES.map((f, i) => {
              const y = sourceY(i);
              return (
                <g key={f}>
                  <rect x={14} y={y} width={92} height={28} rx={6} className="kdx-file" />
                  <text x={60} y={y + 18} textAnchor="middle" fontSize={9.5} className="kdx-file-t">
                    {f}
                  </text>
                  <path d={`M106,${y + 14} L116,${CY}`} className="kdx-thin" />
                  <FlowDot path={`M106,${y + 14} L116,${CY}`} dur={2.2} begin={i * 0.4} cls="kdx-dot--io" r={2.4} />
                </g>
              );
            })}

            {/* ── reader ── */}
            <Stage x={116} w={96} role="io" title="Reader" sub="DataFormat" />

            <Channel x1={212} x2={248} />
            <FlowDot path="M212,196 L248,196" dur={1.5} begin={0} cls="kdx-dot--io" />
            <FlowDot path="M212,196 L248,196" dur={1.5} begin={0.75} cls="kdx-dot--io" />

            {/* ── annotate ── */}
            <Stage x={248} w={132} role="annotate" title="Annotate" sub="segment · terms · NER" />

            {/* ── annotate → fan-out ── */}
            <Channel x1={380} x2={412} />

            {/* fan-out lane: bracket, workers, fan-in */}
            <rect x={430} y={114} width={172} height={166} rx={12} className="kdx-bracket" />
            <text x={516} y={106} textAnchor="middle" fontSize={9} className="kdx-note">
              fan-out · N goroutines
            </text>
            <text x={516} y={296} textAnchor="middle" fontSize={9} className="kdx-note">
              ordered fan-in
            </text>

            {WORKERS.map((i) => {
              const wy = workerY(i);
              const inPath = `M412,${CY} L440,${wy + WORKER_H / 2}`;
              const outPath = `M${WORKER_X + WORKER_W},${wy + WORKER_H / 2} L622,${CY}`;
              return (
                <g key={i}>
                  <path d={inPath} className="kdx-thin kdx-thin--t" />
                  <path d={outPath} className="kdx-thin kdx-thin--t" />
                  <rect x={WORKER_X} y={wy} width={WORKER_W} height={WORKER_H} rx={7} className="kdx-worker" />
                  <rect
                    x={WORKER_X}
                    y={wy}
                    width={WORKER_W}
                    height={WORKER_H}
                    rx={7}
                    className="kdx-pulse"
                    style={{ animationDelay: `${i * 0.8}s` }}
                  />
                  <text x={WORKER_X + 12} y={wy + 18} fontSize={10.5} className="kdx-label">
                    AI Translate
                  </text>
                  <text
                    x={WORKER_X + WORKER_W - 10}
                    y={wy + 18}
                    textAnchor="end"
                    fontSize={8}
                    className="kdx-sub kdx-sub--translate"
                  >
                    goroutine {i + 1}
                  </text>
                </g>
              );
            })}

            {/* fan-out flow dots threading two of the worker paths */}
            <FlowDot
              path={`M380,${CY} L412,${CY} L440,${workerY(0) + WORKER_H / 2} L${WORKER_X + WORKER_W},${
                workerY(0) + WORKER_H / 2
              } L622,${CY}`}
              dur={2.6}
              begin={0.2}
              cls="kdx-dot--translate"
              r={2.6}
            />
            <FlowDot
              path={`M380,${CY} L412,${CY} L440,${workerY(2) + WORKER_H / 2} L${WORKER_X + WORKER_W},${
                workerY(2) + WORKER_H / 2
              } L622,${CY}`}
              dur={2.6}
              begin={1.3}
              cls="kdx-dot--translate"
              r={2.6}
            />

            {/* ── fan-in → QA ── */}
            <Channel x1={622} x2={654} />

            {/* ── QA ── */}
            <Stage x={654} w={96} role="qa" title="QA" sub="qa-check · enforce" />

            <Channel x1={750} x2={786} />
            <FlowDot path="M750,196 L786,196" dur={1.5} begin={0.4} cls="kdx-dot--io" />
            <FlowDot path="M750,196 L786,196" dur={1.5} begin={1.15} cls="kdx-dot--io" />

            {/* ── writer ── */}
            <Stage x={786} w={96} role="io" title="Writer" sub="DataFormat" />

            {/* ── targets (right) ── */}
            <text x={940} y={98} textAnchor="middle" fontSize={9.5} className="kdx-cap">
              Targets
            </text>
            {TARGETS.map((f, i) => {
              const y = targetY(i);
              return (
                <g key={f}>
                  <path d={`M882,${CY} L894,${y + 14}`} className="kdx-thin" />
                  <rect x={894} y={y} width={92} height={28} rx={6} className="kdx-file" />
                  <text x={940} y={y + 18} textAnchor="middle" fontSize={9} className="kdx-file-t">
                    {f}
                  </text>
                  <FlowDot path={`M882,${CY} L894,${y + 14}`} dur={2.2} begin={i * 0.4 + 0.6} cls="kdx-dot--io" r={2.4} />
                </g>
              );
            })}

            {/* ── plugin system band (bottom) ── */}
            <rect x={40} y={360} width={920} height={120} rx={14} className="kdx-band" />
            <text x={60} y={382} fontSize={9.5} className="kdx-cap">
              Plugin system · gRPC subprocess bridge
            </text>

            {/* Okapi bridge → Reader */}
            <rect x={92} y={400} width={176} height={48} rx={9} className="kdx-chip" />
            <text x={180} y={420} textAnchor="middle" fontSize={11.5} className="kdx-chip-t">
              Okapi Bridge
            </text>
            <text x={180} y={435} textAnchor="middle" fontSize={8.5} className="kdx-chip-sub">
              Java filters
            </text>
            <path d="M180,400 L164,228" className="kdx-link kdx-link--plugin" />

            {/* kapi-sat → Annotate */}
            <rect x={300} y={400} width={176} height={48} rx={9} className="kdx-chip" />
            <text x={388} y={420} textAnchor="middle" fontSize={11.5} className="kdx-chip-t">
              kapi-sat
            </text>
            <text x={388} y={435} textAnchor="middle" fontSize={8.5} className="kdx-chip-sub">
              ML segmentation
            </text>
            <path d="M388,400 L314,228" className="kdx-link kdx-link--plugin" />

            {/* remote / native → QA (a tool stage) */}
            <rect x={560} y={400} width={200} height={48} rx={9} className="kdx-chip" />
            <text x={660} y={420} textAnchor="middle" fontSize={11.5} className="kdx-chip-t">
              Remote / native plugin
            </text>
            <text x={660} y={435} textAnchor="middle" fontSize={8.5} className="kdx-chip-sub">
              custom tool · format
            </text>
            <path d="M660,400 L702,228" className="kdx-link kdx-link--plugin" />
          </svg>
        </div>
      </div>
    </div>
  );
}

export default ArchitectureDiagram;
