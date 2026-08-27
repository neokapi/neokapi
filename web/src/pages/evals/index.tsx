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
  tone,
  type BandInfo,
  type Eval,
} from "./_shared";

// The /evals cover page: a menu, not a catalogue.
//
// It used to carry every band, every layer and eighteen expandable cards, and
// it was too much to take in at once. What a reader needs here is which three
// kinds of evidence exist, how much of each is real, and a way in. The detail
// lives one click away, on a page per band, which is also the unit anyone
// actually arrives with: someone asking how fast the engine is was not also
// asking about judge validation.

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
          {(Object.keys(statusMeans) as Eval["status"][])
            .map((k) => [k, counts[k]] as const)
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
  const total = c.measured + c.partial + c.unvalidated + c.absent;
  const stale = staleCount(data.evals);

  return (
    <Layout
      title="Tests and Evals"
      description="Every measurement kapi publishes, in three bands by what is under test, including the layers nothing measures yet."
    >
      <main className="container margin-vert--lg">
        <h1>Tests and Evals</h1>
        <p style={s.lede}>
          Three bands, by what is under test. kapi's own code can be asserted. A model's output can
          only be estimated. An agent's behaviour is scored scenario by scenario. Inside each band
          the structure is the architecture.
        </p>

        <div style={s.strip}>
          <div style={s.stat}>
            <span style={{ ...s.statN, color: tone.ok.fg }}>{c.measured}</span>
            <span style={s.statL}>measured</span>
          </div>
          <div style={s.stat}>
            <span style={{ ...s.statN, color: tone.warn.fg }}>{c.partial}</span>
            <span style={s.statL}>partial</span>
          </div>
          <div style={s.stat}>
            <span style={{ ...s.statN, color: tone.warn.fg }}>{c.unvalidated}</span>
            <span style={s.statL}>unvalidated</span>
          </div>
          <div style={s.stat}>
            <span style={{ ...s.statN, color: tone.gap.fg }}>{c.absent}</span>
            <span style={s.statL}>not measured</span>
          </div>
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
            <span style={s.statL}>layers with nothing behind them</span>
          </div>
        </div>

        <div style={{ display: "grid", gap: "1rem", margin: "1.6rem 0 2rem" }}>
          {data.bands.map((b) => (
            <BandCard key={b.id} b={b} />
          ))}
        </div>

        <h2>What the statuses mean</h2>
        <div style={{ ...s.bandGrid, gridTemplateColumns: "7.5rem 1fr", maxWidth: "64rem" }}>
          {(Object.keys(statusMeans) as Eval["status"][]).map((k) => (
            <Fragment key={k}>
              <span>
                <Pill text={statusLabel[k]} t={statusTone[k]} />
              </span>
              <span style={{ color: "var(--ifm-color-emphasis-800)" }}>{statusMeans[k]}</span>
            </Fragment>
          ))}
        </div>

        <p style={{ ...s.lede, fontSize: ".92rem", marginTop: "1.2rem" }}>
          {c.absent} of {total} are not built. Those rows carry the same card as a built one, minus
          the data: a gap a reader cannot see is a gap they will assume is covered. Each card's age
          is read from its own dataset, not typed here, because a hand-written date is a date nobody
          updates.
        </p>
        <p style={{ ...s.lede, fontSize: ".92rem" }}>
          Every card states what it <strong>misses</strong> as well as what it covers, and gives the
          command that reproduces it. Across more than 50,000 published evaluation records surveyed
          in 2026, 96.5% were missing at least one field needed to re-run them. A number nobody can
          reproduce is an assertion with a table around it.
        </p>
      </main>
    </Layout>
  );
}
