import { Fragment, useState } from "react";
import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import index from "./_index.json";

// The /evals cover page.
//
// Three bands, split by what an eval has under test, because the subject
// decides what its numbers can mean: kapi's own code is deterministic and can
// gate a build; a model's output is sampled and can only be tracked; an agent's
// behaviour is stochastic in a third way and is scored per scenario.
//
// Inside a band the structure is the architecture, the same six AD series the
// contributor docs use, so an eval sits beside the document describing what it
// measures and a series with nothing behind it shows as a hole in the
// architecture rather than as an absence from a list.
//
// Short text, dense data. Everything a reader might weigh is a field on a card;
// the long-form argument belongs in the dashboards the cards link to.
//
// Regenerate with `make eval-index`.

interface BandInfo {
  id: string;
  title: string;
  subject: string;
  evidence: string;
  gates: string;
  layers: string[];
}
interface Layer {
  id: string;
  band: string;
  series: string;
  title: string;
  scope: string;
  rests?: string;
  ad: string;
  evals: string[];
}
interface Eval {
  id: string;
  title: string;
  method: "deterministic" | "labelled" | "judged" | "benchmark" | "comparative" | "scenario";
  status: "measured" | "partial" | "unvalidated" | "absent";
  spends?: boolean;
  local?: boolean;
  corpus: string;
  covers: string;
  misses?: string;
  reproduce?: string;
  settings?: string;
  data?: string;
  page?: string;
  validation?: string;
  fresh?: { date?: string; undated?: boolean };
}
interface Index {
  _note: string;
  bands: BandInfo[];
  layers: Layer[];
  evals: Eval[];
  coverage: {
    measured: number;
    partial: number;
    unvalidated: number;
    absent: number;
    layersUnmeasured: number;
    perBand: Record<string, number>;
    undated: number;
  };
}

const data = index as Index;
const evalByID = new Map(data.evals.map((e) => [e.id, e]));
const layerByID = new Map(data.layers.map((l) => [l.id, l]));

const mono = "var(--ifm-font-family-monospace)";

// Infima's `-contrast-foreground` tokens invert with the theme; `-darker` does
// not, and mixing them puts mid-red on dark red for anyone in dark mode.
const tone = {
  ok: {
    fg: "var(--ifm-color-success-contrast-foreground)",
    bg: "var(--ifm-color-success-contrast-background)",
  },
  warn: {
    fg: "var(--ifm-color-warning-contrast-foreground)",
    bg: "var(--ifm-color-warning-contrast-background)",
  },
  gap: {
    fg: "var(--ifm-color-danger-contrast-foreground)",
    bg: "var(--ifm-color-danger-contrast-background)",
  },
  flat: { fg: "var(--ifm-color-emphasis-800)", bg: "var(--ifm-color-emphasis-200)" },
} as const;
type Tone = keyof typeof tone;

const statusTone: Record<Eval["status"], Tone> = {
  measured: "ok",
  partial: "warn",
  unvalidated: "warn",
  absent: "gap",
};

// The status id is "absent", which is right for a registry and wrong for a
// reader: it invites "absent from what?". One map keeps every surface saying
// "not measured" instead.
const statusLabel: Record<Eval["status"], string> = {
  measured: "measured",
  partial: "partial",
  unvalidated: "unvalidated",
  absent: "not measured",
};

// Mirrors StaleAfterDays in scripts/evalindex. Six weeks is roughly how long it
// takes this repo to move enough that a number is worth re-earning.
const STALE_AFTER_DAYS = 42;

// Age is computed here rather than stored, so it is right every morning. The
// generator records only the date; a baked-in age would be a committed file
// that goes wrong overnight.
function ageDays(f?: { date?: string }): number | undefined {
  if (!f?.date) return undefined;
  const t = Date.parse(f.date);
  if (Number.isNaN(t)) return undefined;
  return Math.max(0, Math.floor((Date.now() - t) / 86_400_000));
}

function freshnessBadge(e: Eval): ReactElement | null {
  if (!e.data) return null;
  if (e.fresh?.undated) return <Pill text="undated" t="gap" />;
  const age = ageDays(e.fresh);
  if (age === undefined) return null;
  return (
    <Pill
      text={`${age}d old`}
      t={age > STALE_AFTER_DAYS ? "gap" : age > STALE_AFTER_DAYS / 2 ? "warn" : "ok"}
    />
  );
}

/** How many published datasets have gone stale as of right now. */
function staleCount(evals: Eval[]): number {
  return evals.filter((e) => {
    const age = ageDays(e.fresh);
    return e.data && age !== undefined && age > STALE_AFTER_DAYS;
  }).length;
}

const statusMeans: Record<Eval["status"], string> = {
  measured: "runs, data committed, numbers can be read as they stand",
  partial: "runs, and covers less than the layer needs",
  unvalidated: "produces numbers that should not yet be relied on",
  absent: "nothing measures this",
};

const s: Record<string, CSSProperties> = {
  lede: { fontSize: "1.05rem", lineHeight: 1.6, maxWidth: "68ch" },
  strip: {
    display: "flex",
    flexWrap: "wrap",
    gap: "1.6rem",
    padding: "1.1rem 1.3rem",
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    background: "var(--ifm-background-surface-color)",
    margin: "1.5rem 0",
  },
  stat: { display: "flex", flexDirection: "column", gap: ".15rem" },
  statN: { fontSize: "1.6rem", fontWeight: 650, lineHeight: 1, fontVariantNumeric: "tabular-nums" },
  statL: {
    fontSize: ".7rem",
    letterSpacing: ".1em",
    textTransform: "uppercase",
    color: "var(--ifm-color-emphasis-700)",
  },
  band: { marginTop: "3rem" },
  bandHead: {
    borderTop: "3px solid var(--ifm-color-emphasis-800)",
    paddingTop: ".9rem",
    marginBottom: "1.2rem",
  },
  bandTitle: { fontSize: "1.55rem", fontWeight: 660, letterSpacing: "-.02em", margin: "0 0 .5rem" },
  bandGrid: {
    display: "grid",
    gridTemplateColumns: "6rem 1fr",
    gap: ".3rem 1rem",
    fontSize: ".88rem",
    lineHeight: 1.5,
    maxWidth: "76ch",
  },
  bandK: {
    color: "var(--ifm-color-emphasis-700)",
    fontSize: ".7rem",
    letterSpacing: ".08em",
    textTransform: "uppercase",
    paddingTop: ".18rem",
  },
  layer: { marginTop: "1.8rem" },
  layerHead: { display: "flex", alignItems: "baseline", gap: ".6rem", flexWrap: "wrap" },
  series: {
    fontFamily: mono,
    fontSize: ".78rem",
    fontWeight: 700,
    padding: ".1rem .4rem",
    borderRadius: 3,
    background: "var(--ifm-color-emphasis-200)",
    color: "var(--ifm-color-emphasis-800)",
  },
  layerTitle: { fontSize: "1.1rem", fontWeight: 640, margin: 0 },
  scope: {
    color: "var(--ifm-color-emphasis-800)",
    fontSize: ".9rem",
    lineHeight: 1.5,
    maxWidth: "72ch",
    margin: ".35rem 0 .8rem",
  },
  rests: { fontSize: ".8rem", color: "var(--ifm-color-emphasis-700)", fontStyle: "italic" },
  cards: { display: "grid", gap: ".55rem" },
  row: {
    display: "grid",
    gridTemplateColumns: "1fr auto",
    gap: ".8rem",
    alignItems: "center",
    width: "100%",
    textAlign: "left",
    padding: ".65rem .85rem",
    border: "1px solid var(--ifm-color-emphasis-300)",
    background: "var(--ifm-background-surface-color)",
    cursor: "pointer",
    font: "inherit",
    color: "inherit",
  },
  rowAbsent: { borderStyle: "dashed", background: "transparent" },
  pills: { display: "flex", gap: ".35rem", flexWrap: "wrap" },
  pill: {
    fontFamily: mono,
    fontSize: ".65rem",
    letterSpacing: ".07em",
    textTransform: "uppercase",
    padding: ".16rem .45rem",
    borderRadius: 4,
    whiteSpace: "nowrap",
  },
  detail: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderTop: "none",
    borderRadius: "0 0 6px 6px",
    padding: ".9rem 1rem",
    display: "grid",
    gridTemplateColumns: "6rem 1fr",
    gap: ".4rem 1rem",
    fontSize: ".88rem",
    lineHeight: 1.5,
  },
  k: {
    color: "var(--ifm-color-emphasis-700)",
    fontSize: ".7rem",
    letterSpacing: ".08em",
    textTransform: "uppercase",
    paddingTop: ".15rem",
  },
  cmd: { fontFamily: mono, fontSize: ".8rem", wordBreak: "break-all" },
};

function Pill({ text, t }: { text: string; t: Tone }): ReactElement {
  return <span style={{ ...s.pill, color: tone[t].fg, background: tone[t].bg }}>{text}</span>;
}

function EvalRow({ e }: { e: Eval }): ReactElement {
  const [open, setOpen] = useState(false);
  const absent = e.status === "absent";
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        style={{
          ...s.row,
          ...(absent ? s.rowAbsent : {}),
          borderRadius: open ? "6px 6px 0 0" : 6,
        }}
      >
        <span style={{ display: "flex", alignItems: "baseline", gap: ".45rem" }}>
          <span aria-hidden style={{ fontFamily: mono, opacity: 0.45 }}>
            {open ? "−" : "+"}
          </span>
          <span style={{ fontWeight: 600 }}>{e.title}</span>
        </span>
        <span style={s.pills}>
          <Pill text={e.method} t="flat" />
          {e.local && <Pill text="local only" t="flat" />}
          {e.spends && <Pill text="spends" t="flat" />}
          {freshnessBadge(e)}
          <Pill text={statusLabel[e.status]} t={statusTone[e.status]} />
        </span>
      </button>
      {open && (
        <div style={{ ...s.detail, borderStyle: absent ? "dashed" : "solid", borderTop: "none" }}>
          <span style={s.k}>Covers</span>
          <span>{e.covers}</span>
          {e.misses && (
            <>
              <span style={s.k}>Misses</span>
              <span style={{ color: tone.gap.fg }}>{e.misses}</span>
            </>
          )}
          <span style={s.k}>Corpus</span>
          <span>{e.corpus}</span>
          {e.settings && (
            <>
              <span style={s.k}>Settings</span>
              <span
                style={e.settings.startsWith("Not recorded") ? { color: tone.gap.fg } : undefined}
              >
                {e.settings}
              </span>
            </>
          )}
          {e.validation && (
            <>
              <span style={s.k}>Validation</span>
              <span>{e.validation}</span>
            </>
          )}
          {e.reproduce && (
            <>
              <span style={s.k}>Reproduce</span>
              <code style={s.cmd}>{e.reproduce}</code>
            </>
          )}
          {e.fresh?.date && (
            <>
              <span style={s.k}>Measured</span>
              <span
                style={
                  (ageDays(e.fresh) ?? 0) > STALE_AFTER_DAYS ? { color: tone.gap.fg } : undefined
                }
              >
                {e.fresh.date.slice(0, 10)} · {ageDays(e.fresh)} days ago
              </span>
            </>
          )}
          {e.fresh?.undated && (
            <>
              <span style={s.k}>Measured</span>
              <span style={{ color: tone.gap.fg }}>
                The dataset records no date, so its age cannot be shown.
              </span>
            </>
          )}
          {e.page && (
            <>
              <span style={s.k}>Results</span>
              <Link to={e.page}>{e.page}</Link>
            </>
          )}
        </div>
      )}
    </div>
  );
}

function LayerBlock({ l }: { l: Layer }): ReactElement {
  const list = l.evals.map((id) => evalByID.get(id)).filter((e): e is Eval => Boolean(e));
  const unmeasured = list.every((e) => e.status === "absent");
  return (
    <section style={s.layer}>
      <div style={s.layerHead}>
        <span style={s.series}>{l.series}</span>
        <h3 style={s.layerTitle}>{l.title}</h3>
        <Link to={l.ad} style={{ fontSize: ".78rem" }}>
          architecture
        </Link>
        {unmeasured && <Pill text="nothing measured" t="gap" />}
      </div>
      <p style={s.scope}>
        {l.scope}
        {l.rests && <span style={s.rests}> Rests on {l.rests}.</span>}
      </p>
      <div style={s.cards}>
        {list.map((e) => (
          <EvalRow key={e.id} e={e} />
        ))}
      </div>
    </section>
  );
}

export default function Evals(): ReactElement {
  const c = data.coverage;
  const total = c.measured + c.partial + c.unvalidated + c.absent;
  const stale = staleCount(data.evals);

  return (
    <Layout
      title="Tests and Evals"
      description="Every measurement kapi publishes, grouped by the layer it measures, including the layers nothing measures yet."
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

        <div
          style={{
            ...s.bandGrid,
            gridTemplateColumns: "7.5rem 1fr",
            maxWidth: "64rem",
            marginBottom: "1rem",
          }}
        >
          {(Object.keys(statusMeans) as Eval["status"][]).map((k) => (
            <Fragment key={k}>
              <span>
                <Pill text={statusLabel[k]} t={statusTone[k]} />
              </span>
              <span style={{ color: "var(--ifm-color-emphasis-800)" }}>{statusMeans[k]}</span>
            </Fragment>
          ))}
        </div>
        <p style={{ ...s.lede, fontSize: ".92rem" }}>
          {c.absent} of {total} are not built. Those rows carry the same card as a built one, minus
          the data: a gap a reader cannot see is a gap they will assume is covered. Each card's age
          is read from its own dataset, not typed here, because a hand-written date is a date nobody
          updates.
        </p>

        {data.bands.map((b) => (
          <section key={b.id} style={s.band}>
            <div style={s.bandHead}>
              <h2 style={s.bandTitle}>{b.title}</h2>
              <div style={s.bandGrid}>
                <span style={s.bandK}>Subject</span>
                <span>{b.subject}</span>
                <span style={s.bandK}>Evidence</span>
                <span>{b.evidence}</span>
                <span style={s.bandK}>Gates CI</span>
                <span>{b.gates}</span>
              </div>
            </div>
            {b.layers
              .map((id) => layerByID.get(id))
              .filter((l): l is Layer => Boolean(l))
              .map((l) => (
                <LayerBlock key={l.id} l={l} />
              ))}
          </section>
        ))}
      </main>
    </Layout>
  );
}
