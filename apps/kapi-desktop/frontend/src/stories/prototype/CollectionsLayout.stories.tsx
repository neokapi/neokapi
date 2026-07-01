import type { Meta, StoryObj } from "@storybook/react-vite";
import { PieChart, Pie, Cell, ResponsiveContainer } from "recharts";
import { Badge } from "@neokapi/ui-primitives";
import { ChevronRight, Layers, Pencil, Play } from "lucide-react";

// ── Prototype: revised collection-row layouts for issue #1068 review ──────────
// Presentational only (mock data, no backend) so we can compare options in
// screenshots before wiring the winner into CollectionsPanel.

const CHART_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
];

interface Coll {
  name: string;
  files: number;
  blocks: number;
  cov: Record<string, number>;
}

const LANGS5 = ["de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"];
const COLLS5: Coll[] = [
  {
    name: "Website",
    files: 7,
    blocks: 245,
    cov: { "de-DE": 245, "fr-FR": 191, "ja-JP": 110, "nb-NO": 100, "ar-SA": 0 },
  },
  {
    name: "Online Store",
    files: 5,
    blocks: 349,
    cov: { "de-DE": 349, "fr-FR": 349, "ja-JP": 175, "nb-NO": 175, "ar-SA": 0 },
  },
  {
    name: "Contracts",
    files: 2,
    blocks: 80,
    cov: { "de-DE": 80, "fr-FR": 0, "ja-JP": 0, "nb-NO": 0, "ar-SA": 0 },
  },
  {
    name: "Templates",
    files: 2,
    blocks: 25,
    cov: { "de-DE": 25, "fr-FR": 12, "ja-JP": 0, "nb-NO": 0, "ar-SA": 0 },
  },
];

const LANGS8 = ["de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA", "es-ES", "pt-BR", "zh-CN"];
const COLLS8: Coll[] = COLLS5.map((c) => ({
  ...c,
  cov: {
    ...c.cov,
    "es-ES": Math.round(c.blocks * 0.6),
    "pt-BR": Math.round(c.blocks * 0.3),
    "zh-CN": 0,
  },
}));

const pct = (c: Coll, l: string) => (c.blocks ? Math.round(((c.cov[l] ?? 0) / c.blocks) * 100) : 0);
const avgPct = (c: Coll, langs: string[]) =>
  langs.length ? Math.round(langs.reduce((s, l) => s + pct(c, l), 0) / langs.length) : 0;

/** Shared color-coded "cake" (block distribution per collection) + legend. */
function Cake({ colls, langs }: { colls: Coll[]; langs: string[] }) {
  const total = colls.reduce((s, c) => s + c.blocks, 0);
  const data = colls.map((c, i) => ({ name: c.name, value: c.blocks, fill: CHART_COLORS[i] }));
  // Project-wide coverage per language (across collections).
  const cov = langs.map((l) => {
    let t = 0;
    let d = 0;
    for (const c of colls) {
      t += c.cov[l] ?? 0;
      d += c.blocks;
    }
    return { l, p: d ? Math.round((t / d) * 100) : 0 };
  });
  return (
    <div className="mb-3 grid grid-cols-[auto_1fr] items-center gap-6 rounded-lg border border-border p-4">
      <div className="flex items-center gap-3">
        <div className="h-28 w-28 shrink-0">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie
                data={data}
                dataKey="value"
                nameKey="name"
                innerRadius="56%"
                outerRadius="100%"
                paddingAngle={2}
                strokeWidth={0}
              >
                {data.map((d) => (
                  <Cell key={d.name} fill={d.fill} />
                ))}
              </Pie>
            </PieChart>
          </ResponsiveContainer>
        </div>
        <ul className="space-y-1 text-xs">
          <li className="font-medium">{total} blocks</li>
          {colls.map((c, i) => (
            <li key={c.name} className="flex items-center gap-1.5">
              <span
                className="size-2 shrink-0 rounded-[2px]"
                style={{ background: CHART_COLORS[i] }}
              />
              <span className="text-muted-foreground">{c.name}</span>
              <span className="tabular-nums">{c.blocks}</span>
            </li>
          ))}
        </ul>
      </div>
      <div className="space-y-1.5">
        <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Coverage across collections
        </div>
        <div className="flex flex-wrap gap-x-6 gap-y-1.5">
          {cov.map((x) => (
            <span key={x.l} className="flex min-w-40 flex-1 items-center gap-2">
              <span className="w-14 text-xs text-muted-foreground">{x.l}</span>
              <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-accent">
                <span
                  className="block h-full rounded-full bg-primary"
                  style={{ width: `${x.p}%` }}
                />
              </span>
              <span className="w-9 text-right text-[11px] tabular-nums text-muted-foreground">
                {x.p}%
              </span>
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

function Dot({ i }: { i: number }) {
  return (
    <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: CHART_COLORS[i] }} />
  );
}
function NameCell({ c, i }: { c: Coll; i: number }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <ChevronRight size={13} className="shrink-0 text-muted-foreground" />
      <Dot i={i} />
      <Layers size={13} className="shrink-0 text-primary" />
      <span className="truncate text-sm font-medium">{c.name}</span>
    </span>
  );
}
function Actions() {
  return (
    <span className="flex items-center justify-end gap-1 text-muted-foreground">
      <Play size={12} />
      <Pencil size={12} />
    </span>
  );
}
function coverageTint(p: number) {
  // 0% → muted, 100% → primary.
  return `color-mix(in oklch, var(--primary) ${p}%, var(--muted))`;
}

/** OPTION A — one column per language: a mini bar + %. Clean for ≤4 langs. */
function OptionA({ colls, langs }: { colls: Coll[]; langs: string[] }) {
  const cols = `16px minmax(120px,1.5fr) 48px 60px repeat(${langs.length}, minmax(64px,1fr)) 44px`;
  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <div
        className="grid items-center gap-x-3 border-b border-border px-3 py-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground"
        style={{ gridTemplateColumns: cols }}
      >
        <span />
        <span>Collection</span>
        <span className="text-right">Files</span>
        <span className="text-right">Blocks</span>
        {langs.map((l) => (
          <span key={l} className="text-center normal-case" translate="no">
            {l}
          </span>
        ))}
        <span />
      </div>
      {colls.map((c, i) => (
        <div
          key={c.name}
          className="grid items-center gap-x-3 border-b border-border px-3 py-2.5 last:border-0"
          style={{ gridTemplateColumns: cols }}
        >
          <NameCell c={c} i={i} />
          <span />
          <span className="text-right text-xs tabular-nums text-muted-foreground">{c.files}</span>
          <span className="text-right text-xs tabular-nums">{c.blocks}</span>
          {langs.map((l) => (
            <span key={l} className="flex flex-col items-center gap-1">
              <span className="h-1.5 w-full overflow-hidden rounded-full bg-accent">
                <span
                  className="block h-full rounded-full bg-primary"
                  style={{ width: `${pct(c, l)}%` }}
                />
              </span>
              <span className="text-[10px] tabular-nums text-muted-foreground">{pct(c, l)}%</span>
            </span>
          ))}
          <Actions />
        </div>
      ))}
    </div>
  );
}

/** OPTION B — coverage heatmap matrix: compact colored % tiles per language. */
function OptionB({ colls, langs }: { colls: Coll[]; langs: string[] }) {
  const cols = `16px minmax(120px,1.5fr) 48px 60px repeat(${langs.length}, minmax(46px,1fr)) 44px`;
  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <div
        className="grid items-center gap-x-2 border-b border-border px-3 py-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground"
        style={{ gridTemplateColumns: cols }}
      >
        <span />
        <span>Collection</span>
        <span className="text-right">Files</span>
        <span className="text-right">Blocks</span>
        {langs.map((l) => (
          <span key={l} className="text-center normal-case" translate="no">
            {l.split("-")[0]}
          </span>
        ))}
        <span />
      </div>
      {colls.map((c, i) => (
        <div
          key={c.name}
          className="grid items-center gap-x-2 border-b border-border px-3 py-2 last:border-0"
          style={{ gridTemplateColumns: cols }}
        >
          <NameCell c={c} i={i} />
          <span />
          <span className="text-right text-xs tabular-nums text-muted-foreground">{c.files}</span>
          <span className="text-right text-xs tabular-nums">{c.blocks}</span>
          {langs.map((l) => {
            const p = pct(c, l);
            return (
              <span
                key={l}
                className="flex h-6 items-center justify-center rounded text-[10px] font-medium tabular-nums"
                style={{
                  background: coverageTint(p),
                  color: p > 55 ? "var(--primary-foreground)" : "var(--muted-foreground)",
                }}
                title={`${l}: ${p}%`}
              >
                {p}
              </span>
            );
          })}
          <Actions />
        </div>
      ))}
    </div>
  );
}

/** OPTION C — aggregate bar + avg% + per-language dots (scales to any count). */
function OptionC({ colls, langs }: { colls: Coll[]; langs: string[] }) {
  const cols = `16px minmax(120px,1.4fr) 48px 60px 130px 40px minmax(90px,1fr) 44px`;
  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <div
        className="grid items-center gap-x-3 border-b border-border px-3 py-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground"
        style={{ gridTemplateColumns: cols }}
      >
        <span />
        <span>Collection</span>
        <span className="text-right">Files</span>
        <span className="text-right">Blocks</span>
        <span>Coverage</span>
        <span className="text-right">Avg</span>
        <span>Languages</span>
        <span />
      </div>
      {colls.map((c, i) => {
        const a = avgPct(c, langs);
        return (
          <div
            key={c.name}
            className="grid items-center gap-x-3 border-b border-border px-3 py-2.5 last:border-0"
            style={{ gridTemplateColumns: cols }}
          >
            <NameCell c={c} i={i} />
            <span />
            <span className="text-right text-xs tabular-nums text-muted-foreground">{c.files}</span>
            <span className="text-right text-xs tabular-nums">{c.blocks}</span>
            <span className="h-1.5 w-full overflow-hidden rounded-full bg-accent">
              <span className="block h-full rounded-full bg-primary" style={{ width: `${a}%` }} />
            </span>
            <span className="text-right text-[11px] tabular-nums text-muted-foreground">{a}%</span>
            <span
              className="flex flex-wrap items-center gap-1"
              title={langs.map((l) => `${l} ${pct(c, l)}%`).join(" · ")}
            >
              {langs.map((l) => {
                const p = pct(c, l);
                return (
                  <span
                    key={l}
                    className="size-3 rounded-full"
                    style={{ background: coverageTint(p), outline: "1px solid var(--border)" }}
                    title={`${l}: ${p}%`}
                  />
                );
              })}
            </span>
            <Actions />
          </div>
        );
      })}
    </div>
  );
}

function Section({
  title,
  note,
  children,
}: {
  title: string;
  note: string;
  children: React.ReactNode;
}) {
  return (
    <div className="mb-6">
      <div className="mb-1.5 flex items-baseline gap-2">
        <Badge variant="secondary">{title}</Badge>
        <span className="text-xs text-muted-foreground">{note}</span>
      </div>
      {children}
    </div>
  );
}

function Comparison({ colls, langs }: { colls: Coll[]; langs: string[] }) {
  return (
    <div className="max-w-[1100px] p-6">
      <Cake colls={colls} langs={langs} />
      <Section
        title="Option A — per-language columns"
        note="mini bar + % per language; cleanest for 1–4 languages"
      >
        <OptionA colls={colls} langs={langs} />
      </Section>
      <Section
        title="Option B — coverage heatmap"
        note="compact % tiles, colour = coverage; tolerates more languages"
      >
        <OptionB colls={colls} langs={langs} />
      </Section>
      <Section
        title="Option C — aggregate bar + language dots"
        note="one bar + avg%, dots colour-coded by coverage; scales to any count"
      >
        <OptionC colls={colls} langs={langs} />
      </Section>
    </div>
  );
}

const meta: Meta = {
  title: "Prototype/Collections Layout (#1068)",
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj;

/** Five languages (KapiMart) — the common case. */
export const FiveLanguages: Story = {
  render: () => <Comparison colls={COLLS5} langs={LANGS5} />,
};

/** Eight languages — how each option handles "many languages". */
export const EightLanguages: Story = {
  render: () => <Comparison colls={COLLS8} langs={LANGS8} />,
};
