import { useEffect, useState } from "react";
import { t } from "@neokapi/kapi-react/runtime";

/*
  Visualization layout (desktop):

  ┌─ app.json  ─┐         ┌─────────┐        ┌── Worker 1 ──┐        ┌─────────┐       ┌─ app_fr.json
  │  ui.html    │──▸ Reader│  chan ◉ │──▸ TM  │── Worker 2 ──│──▸ QA │  chan ◉ │──▸ Writer ── app_de.json
  └─ docs.md   ─┘         └─────────┘        └── Worker 3 ──┘        └─────────┘       └─ app_ja.json
                                                  ▲ fan-out ▲
*/

const INPUT_FILES = ["app.json", "ui.html", "docs.md"];
const OUTPUT_FILES = ["app_fr.json", "app_de.json", "app_ja.json"];
const WORKERS = ["Worker 1", "Worker 2", "Worker 3", "Worker 4"];

export function Pipeline() {
  const [activeWorker, setActiveWorker] = useState(0);

  useEffect(() => {
    const timer = setInterval(() => setActiveWorker((w) => (w + 1) % WORKERS.length), 800);
    return () => clearInterval(timer);
  }, []);

  return (
    <section id="pipeline" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            Built to{" "}
            <span className="bg-gradient-to-r from-accent-cyan to-brand-400 bg-clip-text text-transparent">
              process at scale
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Your content moves through a pipeline of tools — read, translate, check, write — that
            run in parallel across files and languages, so large jobs finish fast.
          </p>
        </div>

        {/* SVG Pipeline Visualization */}
        <div className="overflow-x-auto pb-4">
          <div className="mx-auto min-w-[800px] max-w-[960px]">
            <svg viewBox="0 0 960 340" className="w-full" xmlns="http://www.w3.org/2000/svg">
              <defs>
                <filter id="glow-teal">
                  <feGaussianBlur stdDeviation="3" result="blur" />
                  <feMerge>
                    <feMergeNode in="blur" />
                    <feMergeNode in="SourceGraphic" />
                  </feMerge>
                </filter>
                <filter id="glow-cyan">
                  <feGaussianBlur stdDeviation="2.5" result="blur" />
                  <feMerge>
                    <feMergeNode in="blur" />
                    <feMergeNode in="SourceGraphic" />
                  </feMerge>
                </filter>
                <linearGradient id="grad-teal" x1="0" y1="0" x2="1" y2="0">
                  <stop offset="0%" stopColor="#25c2a0" stopOpacity="0.6" />
                  <stop offset="100%" stopColor="#25c2a0" stopOpacity="0.15" />
                </linearGradient>
                <linearGradient id="grad-cyan" x1="0" y1="0" x2="1" y2="0">
                  <stop offset="0%" stopColor="#06b6d4" stopOpacity="0.5" />
                  <stop offset="100%" stopColor="#06b6d4" stopOpacity="0.15" />
                </linearGradient>
              </defs>

              {/* ─── Input Files (left) ─── */}
              {INPUT_FILES.map((file, i) => {
                const y = 120 + i * 44;
                return (
                  <g key={file}>
                    <rect
                      x="10"
                      y={y}
                      width="90"
                      height="32"
                      rx="6"
                      fill="#0f1f1b"
                      stroke="#1e3a32"
                      strokeWidth="1"
                    />
                    <text
                      x="55"
                      y={y + 20}
                      textAnchor="middle"
                      fill="#7aebd4"
                      fontSize="10"
                      fontFamily="Source Code Pro, monospace"
                    >
                      {file}
                    </text>
                  </g>
                );
              })}

              {/* ─── Lines: inputs -> Reader ─── */}
              {INPUT_FILES.map((_, i) => {
                const y = 136 + i * 44;
                return (
                  <g key={`in-line-${i}`}>
                    <line
                      x1="100"
                      y1={y}
                      x2="140"
                      y2={170}
                      stroke="#25c2a0"
                      strokeWidth="1"
                      opacity="0.25"
                    />
                    <circle r="2.5" fill="#25c2a0" opacity="0">
                      <animateMotion
                        dur="2s"
                        begin={`${i * 0.4}s`}
                        repeatCount="indefinite"
                        path={`M100,${y} L140,170`}
                      />
                      <animate
                        attributeName="opacity"
                        values="0;0.8;0.8;0"
                        keyTimes="0;0.15;0.8;1"
                        dur="2s"
                        begin={`${i * 0.4}s`}
                        repeatCount="indefinite"
                      />
                    </circle>
                  </g>
                );
              })}

              {/* ─── Reader Stage ─── */}
              <rect
                x="140"
                y="140"
                width="100"
                height="60"
                rx="10"
                fill="#0f1f1b"
                stroke="#25c2a0"
                strokeWidth="1.5"
                opacity="0.9"
              />
              <text
                x="190"
                y="166"
                textAnchor="middle"
                fill="#fff"
                fontSize="13"
                fontFamily="Outfit, sans-serif"
                fontWeight="600"
              >
                {t("Reader")}
              </text>
              <text
                x="190"
                y="184"
                textAnchor="middle"
                fill="#4fddbf"
                fontSize="9"
                fontFamily="Source Code Pro, monospace"
                translate="no"
              >
                DataFormat
              </text>

              {/* ─── Channel: Reader -> Terminology ─── */}
              <line x1="240" y1="170" x2="290" y2="170" stroke="#1e3a32" strokeWidth="1.5" />
              <text
                x="265"
                y="162"
                textAnchor="middle"
                fill="#1e3a32"
                fontSize="8"
                fontFamily="Source Code Pro, monospace"
              ></text>
              <circle r="3" fill="#25c2a0" filter="url(#glow-teal)" opacity="0">
                <animateMotion
                  dur="1.5s"
                  begin="0s"
                  repeatCount="indefinite"
                  path="M240,170 L290,170"
                />
                <animate
                  attributeName="opacity"
                  values="0;1;1;0"
                  keyTimes="0;0.1;0.8;1"
                  dur="1.5s"
                  begin="0s"
                  repeatCount="indefinite"
                />
              </circle>
              <circle r="3" fill="#25c2a0" filter="url(#glow-teal)" opacity="0">
                <animateMotion
                  dur="1.5s"
                  begin="0.75s"
                  repeatCount="indefinite"
                  path="M240,170 L290,170"
                />
                <animate
                  attributeName="opacity"
                  values="0;1;1;0"
                  keyTimes="0;0.1;0.8;1"
                  dur="1.5s"
                  begin="0.75s"
                  repeatCount="indefinite"
                />
              </circle>

              {/* ─── Terminology Stage ─── */}
              <rect
                x="290"
                y="140"
                width="100"
                height="60"
                rx="10"
                fill="#0f1f1b"
                stroke="#f59e0b"
                strokeWidth="1.5"
                opacity="0.9"
              />
              <text
                x="340"
                y="166"
                textAnchor="middle"
                fill="#fff"
                fontSize="13"
                fontFamily="Outfit, sans-serif"
                fontWeight="600"
              >
                {t("Terminology")}
              </text>
              <text
                x="340"
                y="184"
                textAnchor="middle"
                fill="#f59e0b"
                fontSize="9"
                fontFamily="Source Code Pro, monospace"
                translate="no"
              >
                Tool
              </text>

              {/* ─── Channel: Terminology -> Fan-out ─── */}
              <line x1="390" y1="170" x2="430" y2="170" stroke="#1e3a32" strokeWidth="1.5" />
              <text
                x="410"
                y="162"
                textAnchor="middle"
                fill="#1e3a32"
                fontSize="8"
                fontFamily="Source Code Pro, monospace"
              ></text>

              {/* ─── Fan-out lines to workers ─── */}
              {WORKERS.map((_, i) => {
                const wy = 90 + i * 52;
                return (
                  <g key={`fan-${i}`}>
                    <line
                      x1="430"
                      y1="170"
                      x2="460"
                      y2={wy + 16}
                      stroke="#06b6d4"
                      strokeWidth="1"
                      opacity="0.3"
                    />
                    <line
                      x1="580"
                      y1={wy + 16}
                      x2="610"
                      y2="170"
                      stroke="#06b6d4"
                      strokeWidth="1"
                      opacity="0.3"
                    />

                    {/* Animated dots on fan-out path */}
                    <circle r="2.5" fill="#06b6d4" filter="url(#glow-cyan)" opacity="0">
                      <animateMotion
                        dur="2.4s"
                        begin={`${i * 0.5}s`}
                        repeatCount="indefinite"
                        path={`M430,170 L460,${wy + 16} L580,${wy + 16} L610,170`}
                      />
                      <animate
                        attributeName="opacity"
                        values="0;0.9;0.9;0"
                        keyTimes="0;0.08;0.88;1"
                        dur="2.4s"
                        begin={`${i * 0.5}s`}
                        repeatCount="indefinite"
                      />
                    </circle>
                  </g>
                );
              })}

              {/* ─── Fan-out label ─── */}
              <text
                x="520"
                y="52"
                textAnchor="middle"
                fill="#06b6d4"
                fontSize="9"
                fontFamily="Source Code Pro, monospace"
                opacity="0.6"
              >
                {t("in parallel")}
              </text>

              {/* ─── Worker boxes ─── */}
              {WORKERS.map((label, i) => {
                const wy = 90 + i * 52;
                const isActive = i === activeWorker;
                return (
                  <g key={label}>
                    <rect
                      x="460"
                      y={wy}
                      width="120"
                      height="32"
                      rx="8"
                      fill={isActive ? "rgba(6,182,212,0.08)" : "#0a1614"}
                      stroke={isActive ? "#06b6d4" : "#162b25"}
                      strokeWidth={isActive ? 1.5 : 1}
                    />
                    {/* Activity pulse */}
                    {isActive && (
                      <rect
                        x="460"
                        y={wy}
                        width="120"
                        height="32"
                        rx="8"
                        fill="none"
                        stroke="#06b6d4"
                        strokeWidth="1"
                        opacity="0.3"
                      >
                        <animate
                          attributeName="opacity"
                          values="0.3;0;0.3"
                          dur="1.6s"
                          repeatCount="indefinite"
                        />
                      </rect>
                    )}
                    <text
                      x="520"
                      y={wy + 14}
                      textAnchor="middle"
                      fill={isActive ? "#fff" : "#94a3b8"}
                      fontSize="11"
                      fontFamily="Outfit, sans-serif"
                      fontWeight="500"
                    >
                      {t("AI Translate")}
                    </text>
                    <text
                      x="520"
                      y={wy + 26}
                      textAnchor="middle"
                      fill={isActive ? "#06b6d4" : "#1e3a32"}
                      fontSize="8"
                      fontFamily="Source Code Pro, monospace"
                      translate="no"
                    >
                      worker {i + 1}
                    </text>
                  </g>
                );
              })}

              {/* ─── Channel: Fan-in -> QA ─── */}
              <line x1="610" y1="170" x2="650" y2="170" stroke="#1e3a32" strokeWidth="1.5" />
              <text
                x="630"
                y="162"
                textAnchor="middle"
                fill="#1e3a32"
                fontSize="8"
                fontFamily="Source Code Pro, monospace"
              ></text>

              {/* ─── QA Stage ─── */}
              <rect
                x="650"
                y="140"
                width="80"
                height="60"
                rx="10"
                fill="#0f1f1b"
                stroke="#33925d"
                strokeWidth="1.5"
                opacity="0.9"
              />
              <text
                x="690"
                y="166"
                textAnchor="middle"
                fill="#fff"
                fontSize="13"
                fontFamily="Outfit, sans-serif"
                fontWeight="600"
              >
                {t("QA")}
              </text>
              <text
                x="690"
                y="184"
                textAnchor="middle"
                fill="#33925d"
                fontSize="9"
                fontFamily="Source Code Pro, monospace"
                translate="no"
              >
                Tool
              </text>

              {/* ─── Channel: QA -> Writer ─── */}
              <line x1="730" y1="170" x2="770" y2="170" stroke="#1e3a32" strokeWidth="1.5" />
              <text
                x="750"
                y="162"
                textAnchor="middle"
                fill="#1e3a32"
                fontSize="8"
                fontFamily="Source Code Pro, monospace"
              ></text>
              <circle r="3" fill="#25c2a0" filter="url(#glow-teal)" opacity="0">
                <animateMotion
                  dur="1.5s"
                  begin="0.3s"
                  repeatCount="indefinite"
                  path="M730,170 L770,170"
                />
                <animate
                  attributeName="opacity"
                  values="0;1;1;0"
                  keyTimes="0;0.1;0.8;1"
                  dur="1.5s"
                  begin="0.3s"
                  repeatCount="indefinite"
                />
              </circle>
              <circle r="3" fill="#25c2a0" filter="url(#glow-teal)" opacity="0">
                <animateMotion
                  dur="1.5s"
                  begin="1.05s"
                  repeatCount="indefinite"
                  path="M730,170 L770,170"
                />
                <animate
                  attributeName="opacity"
                  values="0;1;1;0"
                  keyTimes="0;0.1;0.8;1"
                  dur="1.5s"
                  begin="1.05s"
                  repeatCount="indefinite"
                />
              </circle>

              {/* ─── Writer Stage ─── */}
              <rect
                x="770"
                y="140"
                width="80"
                height="60"
                rx="10"
                fill="#0f1f1b"
                stroke="#25c2a0"
                strokeWidth="1.5"
                opacity="0.9"
              />
              <text
                x="810"
                y="166"
                textAnchor="middle"
                fill="#fff"
                fontSize="13"
                fontFamily="Outfit, sans-serif"
                fontWeight="600"
              >
                {t("Writer")}
              </text>
              <text
                x="810"
                y="184"
                textAnchor="middle"
                fill="#4fddbf"
                fontSize="9"
                fontFamily="Source Code Pro, monospace"
                translate="no"
              >
                DataFormat
              </text>

              {/* ─── Lines: Writer -> outputs ─── */}
              {OUTPUT_FILES.map((file, i) => {
                const y = 120 + i * 44;
                return (
                  <g key={file}>
                    <line
                      x1="850"
                      y1={170}
                      x2="870"
                      y2={y + 16}
                      stroke="#25c2a0"
                      strokeWidth="1"
                      opacity="0.25"
                    />
                    <rect
                      x="870"
                      y={y}
                      width="85"
                      height="32"
                      rx="6"
                      fill="#0f1f1b"
                      stroke="#1e3a32"
                      strokeWidth="1"
                    />
                    <text
                      x="912"
                      y={y + 20}
                      textAnchor="middle"
                      fill="#7aebd4"
                      fontSize="10"
                      fontFamily="Source Code Pro, monospace"
                    >
                      {file}
                    </text>
                    <circle r="2.5" fill="#25c2a0" opacity="0">
                      <animateMotion
                        dur="2s"
                        begin={`${i * 0.4 + 0.5}s`}
                        repeatCount="indefinite"
                        path={`M850,170 L870,${y + 16}`}
                      />
                      <animate
                        attributeName="opacity"
                        values="0;0.8;0.8;0"
                        keyTimes="0;0.15;0.8;1"
                        dur="2s"
                        begin={`${i * 0.4 + 0.5}s`}
                        repeatCount="indefinite"
                      />
                    </circle>
                  </g>
                );
              })}

              {/* ─── Concurrency labels ─── */}
              {/* File concurrency bracket */}
              <line
                x1="6"
                y1="115"
                x2="6"
                y2="210"
                stroke="#25c2a0"
                strokeWidth="1"
                opacity="0.2"
              />
              <text
                x="4"
                y="268"
                fill="#25c2a0"
                fontSize="8"
                fontFamily="Source Code Pro, monospace"
                opacity="0.4"
                transform="rotate(-90, 4, 268)"
              >
                {t("files in parallel")}
              </text>

              <line
                x1="958"
                y1="115"
                x2="958"
                y2="210"
                stroke="#25c2a0"
                strokeWidth="1"
                opacity="0.2"
              />
              <text
                x="956"
                y="268"
                fill="#25c2a0"
                fontSize="8"
                fontFamily="Source Code Pro, monospace"
                opacity="0.4"
                transform="rotate(-90, 956, 268)"
              >
                {t("3 locales")}
              </text>

              {/* Dashed bracket around workers */}
              <rect
                x="450"
                y="60"
                width="140"
                height="218"
                rx="12"
                fill="none"
                stroke="#06b6d4"
                strokeWidth="1"
                strokeDasharray="4 3"
                opacity="0.15"
              />
            </svg>
          </div>
        </div>
      </div>
    </section>
  );
}
