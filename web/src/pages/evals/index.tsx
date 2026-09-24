import { Fragment } from "react";
import type { ReactElement } from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import {
  data,
  bandPath,
  bandTally,
  staleCount,
  layerByID,
  Pill,
  s,
  statusLabel,
  statusMeans,
  statusTone,
  STATUSES,
  tone,
  type BandInfo,
} from "./_shared";

// Summarize evaluation coverage by band; each band links to detailed results.

function BandCard({ b }: { b: BandInfo }): ReactElement {
  const counts = bandTally(b);
  const layers = b.layers.map((id) => layerByID.get(id)).filter((l) => Boolean(l));
  const total = Object.values(counts).reduce((a, n) => a + n, 0);

  return (
    <Link
      to={bandPath(b.id)}
      style={{
        display: "block",
        padding: "1.2rem 1.4rem",
        border: "1px solid var(--ifm-color-emphasis-300)",
        borderRadius: 8,
        background: "var(--ifm-background-surface-color)",
        color: "inherit",
        textDecoration: "none",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          gap: ".7rem",
          flexWrap: "wrap",
          marginBottom: ".6rem",
        }}
      >
        <span style={{ fontSize: "1.3rem", fontWeight: 650, letterSpacing: "-.015em" }}>
          {b.title}
        </span>
        <span style={s.pills}>
          {STATUSES.map((k) => [k, counts[k]] as const)
            .filter(([, n]) => n > 0)
            .map(([k, n]) => (
              <Pill key={k} text={`${n} ${statusLabel[k]}`} t={statusTone[k]} />
            ))}
        </span>
      </div>

      <div style={{ ...s.bandGrid, maxWidth: "70ch" }}>
        <span style={s.bandK}>Subject</span>
        <span>{b.subject}</span>
        <span style={s.bandK}>Evidence</span>
        <span>{b.evidence}</span>
        <span style={s.bandK}>Gates CI</span>
        <span>{b.gates}</span>
      </div>

      <div
        style={{
          marginTop: ".9rem",
          fontSize: ".8rem",
          color: "var(--ifm-color-emphasis-700)",
          fontFamily: "var(--ifm-font-family-monospace)",
        }}
      >
        {total} eval{total === 1 ? "" : "s"} across {layers.map((l) => l?.series).join(", ")} · open
        →
      </div>
    </Link>
  );
}

export default function Evals(): ReactElement {
  const c = data.coverage;
  const stale = staleCount(data.evals);

  return (
    <Layout
      title="Tests and Evals"
      description="Every measurement kapi publishes, in three bands by what is under test, including the layers nothing measures yet."
    >
      <main className="container margin-vert--lg">
        <h1>Tests and Evals</h1>
        <p style={s.lede}>
          Results are grouped by what is under test: engine correctness, model output and agent
          behaviour. Engine tests assert expected results, model evaluations estimate quality, and
          agent evaluations score individual scenarios. Each group follows the relevant
          architecture.
        </p>

        <div style={s.strip}>
          {STATUSES.filter((k) => c.byStatus[k] > 0).map((k) => (
            <div key={k} style={s.stat}>
              <span style={{ ...s.statN, color: tone[statusTone[k]].fg }}>{c.byStatus[k]}</span>
              <span style={s.statL}>{statusLabel[k]}</span>
            </div>
          ))}
          <div style={s.stat}>
            <span style={{ ...s.statN, color: stale + c.undated ? tone.gap.fg : undefined }}>
              {stale + c.undated}
            </span>
            <span style={s.statL}>stale or undated</span>
          </div>
          <div style={{ ...s.stat, marginLeft: "auto" }}>
            <span style={{ ...s.statN, color: c.layersUnmeasured ? tone.gap.fg : undefined }}>
              {c.layersUnmeasured}/{data.layers.length}
            </span>
            <span style={s.statL}>layers without measurements</span>
          </div>
        </div>

        <div style={{ display: "grid", gap: "1rem", margin: "1.6rem 0 2rem" }}>
          {data.bands.map((b) => (
            <BandCard key={b.id} b={b} />
          ))}
        </div>

        <h2>What the statuses mean</h2>
        <div style={{ ...s.bandGrid, gridTemplateColumns: "7.5rem 1fr", maxWidth: "64rem" }}>
          {STATUSES.map((k) => (
            <Fragment key={k}>
              <span>
                <Pill text={statusLabel[k]} t={statusTone[k]} />
              </span>
              <span style={{ color: "var(--ifm-color-emphasis-800)" }}>{statusMeans[k]}</span>
            </Fragment>
          ))}
        </div>
      </main>
    </Layout>
  );
}
