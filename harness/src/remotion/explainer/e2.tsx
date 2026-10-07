/**
 * E2 "Communication is context": the seven beats, drawn from the approved
 * storyboard. Each beat changes one thing, in step with the words over it:
 *
 *   title      the title settles
 *   rename     the word flips: Workspaces is struck, Spaces arrives
 *   surfaces   the five cards fan out, each as it is named
 *   one-rule   one rule sweeps across them and three cards turn red
 *   map        the cards settle onto the map and the rules attach
 *   assistant  the rules that hold are handed over, the text is written, the check passes
 *   end        the end card
 *
 * Coordinates are the storyboard's (a 1600x900 stage).
 */
import React from "react";
import { useCurrentFrame } from "remotion";
import { theme } from "../components/theme.ts";
import { type BeatProps, Eyebrow, Rise, Stage, cardFill, drawn, ease, lerp, ruleFill, type } from "./primitives.tsx";

// ── The five surfaces ───────────────────────────────────────────────────────

interface Surface {
  name: string;
  audience: string;
  before: [string, string];
  /** The text once "Workspaces" is replaced with "Spaces" everywhere. */
  replaced: [string, string];
  /** Whether that replacement is wrong here. */
  broken: boolean;
  mono?: boolean;
  note?: string;
  /** The map: its column label, its rule, and whether it sits before the rename. */
  column: string;
  rule: string;
  beforeRename: boolean;
}

const SURFACES: Surface[] = [
  { name: "Help article", audience: "Customers", before: ["Create a Space for", "your team."], replaced: ["Create a Space for", "your team."], broken: false, column: "Help", rule: "say Spaces", beforeRename: false },
  { name: "API reference", audience: "Developers", before: ["POST /v1/workspaces", ""], replaced: ["POST /v1/spaces", ""], broken: true, mono: true, column: "API", rule: "keep workspaces", beforeRename: false },
  { name: "Changelog", audience: "Everyone", before: ["2.4: Workspaces can", "be archived."], replaced: ["2.4: Spaces can", "be archived."], broken: true, column: "Changelog", rule: "keep as shipped", beforeRename: true },
  { name: "Migration guide", audience: "Developers", before: ["Workspaces are now", "called Spaces."], replaced: ["Spaces are now", "called Spaces."], broken: true, column: "Migration", rule: "use both names", beforeRename: false },
  { name: "Support reply", audience: "Customers", before: ["Your Spaces are", "under Settings."], replaced: ["Your Spaces are", "under Settings."], broken: false, note: "Reply to: “my workspaces?”", column: "Support", rule: "recognise, never use", beforeRename: false },
];

const CARD_W = 280;
const CARD_H = 420;
const CARD_Y = 230;
const slotX = (i: number) => 60 + i * 300;

/**
 * One surface card at (x, y). `bad` (0 to 1) turns it red with a cross;
 * `ok` (0 to 1) gives it a tick; `replaced` (0 to 1) crossfades its text to
 * the rule's replacement.
 */
const SurfaceCard: React.FC<{ s: Surface; x: number; y: number; bad?: number; ok?: number; replaced?: number; contentOpacity?: number }> = ({
  s,
  x,
  y,
  bad = 0,
  ok = 0,
  replaced = 0,
  contentOpacity = 1,
}) => {
  const lineStyle = (k: 0 | 1, red: boolean) => {
    const base = s.mono && k === 0 ? { ...type.mono(), fontSize: 19 } : type.body();
    return red ? { ...base, fill: theme.red } : base;
  };
  const lines = (text: [string, string], red: boolean, opacity: number) => (
    <g opacity={opacity}>
      <text x={20} y={120} {...lineStyle(0, red)}>
        {text[0]}
      </text>
      <text x={20} y={156} {...lineStyle(1, red)}>
        {text[1]}
      </text>
    </g>
  );
  return (
    <g transform={`translate(${x} ${y})`}>
      <rect width={CARD_W} height={CARD_H} rx={16} {...cardFill()} />
      <rect width={CARD_W} height={CARD_H} rx={16} fill={theme.redSoft} stroke={theme.red} strokeWidth={3} opacity={bad} />
      <rect width={CARD_W} height={56} rx={16} fill={theme.chrome} />
      <rect y={40} width={CARD_W} height={16} fill={theme.chrome} />
      <g opacity={contentOpacity}>
        <Eyebrow x={20} y={37} text={s.name} />
        {/* The old text leaves before the new arrives, so the two never overlap. */}
        {lines(s.before, false, Math.max(0, 1 - 2 * replaced))}
        {lines(s.replaced, s.broken && bad > 0.5, Math.max(0, 2 * replaced - 1))}
        {s.note ? (
          <text x={20} y={200} {...type.s()} fontSize={18}>
            {s.note}
          </text>
        ) : null}
        <text x={20} y={CARD_H - 28} {...type.s()}>
          For: {s.audience}
        </text>
        <text x={CARD_W - 24} y={CARD_H - 24} textAnchor="end" {...type.m()} fill={theme.red} opacity={bad}>
          {"✕"}
        </text>
        <text x={CARD_W - 24} y={CARD_H - 24} textAnchor="end" {...type.m()} fill={theme.green} opacity={ok}>
          {"✓"}
        </text>
      </g>
    </g>
  );
};

// ── 1 · Title ────────────────────────────────────────────────────────────────

export const Title: React.FC<BeatProps> = () => {
  const f = useCurrentFrame();
  return (
    <Stage>
      <Rise p={ease(f, 6, 24)}>
        <text x={800} y={420} textAnchor="middle" {...type.xl()}>
          Communication
        </text>
      </Rise>
      <Rise p={ease(f, 18, 24)}>
        <text x={800} y={520} textAnchor="middle" {...type.xl()}>
          is <tspan fill={theme.accent}>context</tspan>.
        </text>
      </Rise>
      <Eyebrow x={800} y={800} anchor="middle" text="kapi" opacity={ease(f, 40, 24)} />
    </Stage>
  );
};

// ── 2 · The rename: the word flips ──────────────────────────────────────────

export const Rename: React.FC<BeatProps> = ({ cue }) => {
  const f = useCurrentFrame();
  const word = cue("Workspaces", 74);
  const flip = cue("called", 132);
  const spaces = cue("Spaces", 149);
  const simple = cue("Simple", 176);
  return (
    <Stage>
      <Eyebrow x={800} y={200} anchor="middle" text="A decision" opacity={ease(f, 4, 20)} />
      <Rise p={ease(f, word - 6, 20)}>
        <text x={520} y={480} textAnchor="middle" {...type.l()}>
          Workspaces
        </text>
      </Rise>
      <line x1={340} y1={458} x2={700} y2={458} stroke={theme.markStroke} strokeWidth={6} strokeLinecap="round" {...drawn(ease(f, flip, 14), 360)} />
      <path
        d="M760 455 L880 455 M850 425 L885 455 L850 485"
        stroke={theme.accent}
        strokeWidth={5}
        fill="none"
        strokeLinecap="round"
        strokeLinejoin="round"
        {...drawn(ease(f, flip + 8, 16), 220)}
      />
      <g opacity={ease(f, spaces - 4, 16)} transform={`translate(${(1 - ease(f, spaces - 4, 16)) * -24} 0)`}>
        <text x={1110} y={480} textAnchor="middle" {...type.l()} fill={theme.accent}>
          Spaces
        </text>
      </g>
      <Rise p={ease(f, simple - 4, 20)}>
        <text x={800} y={680} textAnchor="middle" {...type.s()}>
          Simple, until you look at where the word sits.
        </text>
      </Rise>
    </Stage>
  );
};

// ── 3 · Five surfaces: the cards fan out ────────────────────────────────────

/** The card each word names, in the order the narration names them. */
const NAMED = ["Help", "API", "changelog", "migration", "Support"];
const NAMED_FALLBACK = [6, 84, 148, 217, 278];
const DECK_X = slotX(2);
const DECK_STEP = 12;

export const Surfaces: React.FC<BeatProps> = ({ cue }) => {
  const f = useCurrentFrame();
  // Departure progress of each card from the deck to its own slot.
  const p = NAMED.map((w, i) => ease(f, cue(w, NAMED_FALLBACK[i]!) - 2, 22));
  // A card still in the deck sits one step behind every card above it that has not left yet.
  const depth = (i: number) => p.slice(0, i).reduce((d, pj) => d + (1 - pj), 0);
  // Paint the deck bottom-up: the last-named card first, the first-named on top.
  const order = [4, 3, 2, 1, 0];
  return (
    <Stage>
      <Eyebrow x={800} y={140} anchor="middle" text="Same word, five places, five rules" opacity={ease(f, 2, 18)} />
      {order.map((i) => {
        const d = depth(i);
        const x = lerp(DECK_X + d * DECK_STEP, slotX(i), p[i]!);
        const y = lerp(CARD_Y + d * DECK_STEP, CARD_Y, p[i]!);
        return <SurfaceCard key={i} s={SURFACES[i]!} x={x} y={y} />;
      })}
    </Stage>
  );
};

// ── 4 · One rule fails: the rule sweeps, three cards turn red ───────────────

export const OneRule: React.FC<BeatProps> = ({ cue }) => {
  const f = useCurrentFrame();
  const bar = ease(f, cue("Write", 0) + 4, 18);
  const sweepStart = cue("and", 48);
  const sweepDur = 52;
  const sweep = ease(f, sweepStart, sweepDur);
  const edge = 40 + 1520 * sweep;
  const verdict = ease(f, sweepStart + sweepDur + 8, 18);
  return (
    <Stage>
      <g opacity={bar} transform={`translate(0 ${(1 - bar) * -18})`}>
        <rect x={40} y={96} width={1520} height={64} rx={12} {...ruleFill()} />
        <text x={800} y={139} textAnchor="middle" {...type.m()} fill={theme.accent}>
          {"Replace “Workspaces” with “Spaces” everywhere"}
        </text>
      </g>
      <rect x={40} y={190} width={1520 * sweep} height={560} fill={theme.markStroke} opacity={0.16} />
      {SURFACES.map((s, i) => {
        const centre = slotX(i) + CARD_W / 2;
        // Each card answers the rule as the sweep's edge passes its centre.
        const passed = edge >= centre ? 1 : 0;
        const at = sweepStart + Math.round((sweepDur * (centre - 40)) / 1520);
        const turn = passed ? ease(f, at, 10) : 0;
        return <SurfaceCard key={i} s={s} x={slotX(i)} y={CARD_Y} replaced={turn} bad={s.broken ? turn : 0} ok={s.broken ? 0 : turn} />;
      })}
      <Rise p={verdict}>
        <text x={800} y={820} textAnchor="middle" {...type.m()} fill={theme.red}>
          Wrong in three of five places
        </text>
      </Rise>
    </Stage>
  );
};

// ── 5 · Where words sit: the cards settle onto the map, the rules attach ────

const MAP_W = 230;
const MAP_H = 120;
const colX = (i: number) => 330 + i * 262;
/** Before the rename, after it, and the line between, on the map's time axis. */
const ROW_BEFORE = 250;
const ROW_AFTER = 520;
const ROW_MID = 385 - MAP_H / 2;

export const MapBeat: React.FC<BeatProps> = ({ cue }) => {
  const f = useCurrentFrame();
  const move = ease(f, cue("where", 20) - 2, 36);
  const who = ease(f, cue("who", 90), 16);
  const where = ease(f, cue("where", 120, 2) - 2, 18);
  const when = ease(f, cue("when", 165) - 2, 28);
  const attach = cue("belongs", 203);
  const footer = ease(f, cue("place", 227), 20);
  return (
    <Stage>
      <Eyebrow x={800} y={92} anchor="middle" text="Where each piece of content sits" opacity={move} />
      {/* The time axis: before and after the rename. */}
      <g opacity={when}>
        <text x={70} y={270} {...type.s()}>
          Before
        </text>
        <Eyebrow x={70} y={300} text="the rename" size={13} />
        <text x={70} y={560} {...type.s()}>
          After
        </text>
        <Eyebrow x={70} y={590} text="the rename" size={13} />
        <line x1={230} y1={410} x2={1540} y2={410} stroke={theme.panelBorder} strokeWidth={2} />
      </g>
      {/* Where it appears: one column per surface. */}
      {SURFACES.map((s, i) => (
        <g key={s.column} opacity={where}>
          <line x1={colX(i)} y1={160} x2={colX(i)} y2={700} stroke={theme.panelBorder} strokeWidth={2} strokeDasharray="6 10" />
          <text x={colX(i)} y={760} textAnchor="middle" {...type.m()}>
            {s.column}
          </text>
        </g>
      ))}
      {SURFACES.map((s, i) => {
        // From the row of cards to the middle of its column, then to its row once time is drawn.
        const row = s.beforeRename ? ROW_BEFORE : ROW_AFTER;
        const x = lerp(slotX(i), colX(i) - MAP_W / 2, move);
        const y = lerp(lerp(CARD_Y, ROW_MID, move), row, when);
        const w = lerp(CARD_W, MAP_W, move);
        const h = lerp(CARD_H, MAP_H, move);
        const pill = ease(f, attach + i * 6, 12);
        return (
          <g key={s.name}>
            <g transform={`translate(${x} ${y})`}>
              <clipPath id={`e2-map-card-${i}`}>
                <rect width={w} height={h} rx={lerp(16, 14, move)} />
              </clipPath>
              <rect width={w} height={h} rx={lerp(16, 14, move)} {...cardFill()} />
              {/* The surface card it was, fading as it shrinks onto the map. */}
              <g clipPath={`url(#e2-map-card-${i})`} opacity={1 - ease(f, cue("where", 20) - 2, 24)}>
                <SurfaceCard s={s} x={0} y={0} />
              </g>
              <rect width={w} height={h} rx={lerp(16, 14, move)} fill="none" stroke={theme.panelBorder} strokeWidth={2} />
              <text x={w / 2} y={40} textAnchor="middle" {...type.s()} opacity={who}>
                {s.audience}
              </text>
              <g opacity={pill} transform={`translate(${w / 2} ${82}) scale(${lerp(0.85, 1, pill)}) translate(${-w / 2} ${-82})`}>
                <rect x={w / 2 - 105} y={60} width={210} height={44} rx={22} {...ruleFill()} />
                <text x={w / 2} y={90} textAnchor="middle" fontSize={19} fontWeight={600} fill={theme.accent}>
                  {s.rule}
                </text>
              </g>
            </g>
          </g>
        );
      })}
      <Rise p={footer}>
        <text x={800} y={840} textAnchor="middle" {...type.s()}>
          {"Who it is for · where it appears · when"}
        </text>
      </Rise>
    </Stage>
  );
};

// ── 6 · Writing with the rules ──────────────────────────────────────────────

/** The article's three lines, the new name marked; written out a character at a time. */
const ARTICLE: Array<Array<{ text: string; accent?: boolean }>> = [
  [{ text: "To work together, create a" }],
  [{ text: "Space", accent: true }, { text: " for your team and invite" }],
  [{ text: "people to it." }],
];

const Typed: React.FC<{ shown: number }> = ({ shown }) => {
  let left = shown;
  return (
    <>
      {ARTICLE.map((line, li) => (
        <text key={li} x={110} y={260 + li * 40} {...type.body()}>
          {line.map((seg, si) => {
            const take = Math.max(0, Math.min(seg.text.length, left));
            left -= seg.text.length;
            if (take <= 0) return null;
            return (
              <tspan key={si} {...(seg.accent ? { fill: theme.accent, fontWeight: 600 } : {})}>
                {seg.text.slice(0, take)}
              </tspan>
            );
          })}
        </text>
      ))}
    </>
  );
};

const ARTICLE_CHARS = ARTICLE.flat().reduce((n, s) => n + s.text.length, 0);

export const Assistant: React.FC<BeatProps> = ({ cue }) => {
  const f = useCurrentFrame();
  const rules = ease(f, cue("hands", 84) - 2, 18);
  const writeStart = cue("that", 117) + 2;
  const written = Math.round(ARTICLE_CHARS * Math.min(1, Math.max(0, (f - writeStart) / 40)));
  const reply = ease(f, cue("and", 156, 1) - 4, 16);
  const check = ease(f, cue("checks", 167), 14);
  const api = ease(f, cue("them", 212) + 4, 20);
  return (
    <Stage>
      <rect x={60} y={70} width={1480} height={760} rx={18} {...cardFill()} />
      <rect x={60} y={70} width={1480} height={56} rx={18} fill={theme.chrome} />
      <rect x={60} y={110} width={1480} height={16} fill={theme.chrome} />
      <circle cx={100} cy={98} r={8} fill={theme.red} />
      <circle cx={128} cy={98} r={8} fill={theme.markStroke} />
      <circle cx={156} cy={98} r={8} fill={theme.green} />
      <text x={800} y={105} textAnchor="middle" {...type.s()}>
        Kapi Desktop
      </text>
      <Eyebrow x={110} y={190} text="help/getting-started.md" />
      <Typed shown={written} />
      <g opacity={check} transform={`translate(${384} ${450}) scale(${lerp(0.94, 1, check)}) translate(${-384} ${-450})`}>
        <rect x={104} y={420} width={560} height={60} rx={12} fill={theme.panel} stroke={theme.green} strokeWidth={3} />
        <text x={130} y={459} {...type.m()} fill={theme.green}>
          {"✓  Checked against the rules here"}
        </text>
      </g>
      <line x1={760} y1={126} x2={760} y2={830} stroke={theme.panelBorder} strokeWidth={2} />
      <Eyebrow x={800} y={190} text="Assistant" />
      <g opacity={rules} transform={`translate(${(1 - rules) * 30} 0)`}>
        <rect x={800} y={220} width={690} height={150} rx={14} {...ruleFill()} />
        <text x={826} y={262} {...type.s()}>
          Rules where this text sits
        </text>
        <text x={826} y={304} {...type.body()}>
          {"Customers · help article · after the rename"}
        </text>
        <text x={826} y={344} {...type.body()} fill={theme.accent} fontWeight={600}>
          {"Say “Spaces”"}
        </text>
      </g>
      <Rise p={reply}>
        <text x={826} y={430} {...type.body()}>
          Rewrote the setup steps for the
        </text>
        <text x={826} y={470} {...type.body()}>
          new name. One change, checked.
        </text>
      </Rise>
      <Rise p={api}>
        <text x={800} y={760} {...type.s()}>
          {"Same request on the API page: “workspaces” stays."}
        </text>
      </Rise>
    </Stage>
  );
};

// ── 7 · End card ─────────────────────────────────────────────────────────────

export const End: React.FC<BeatProps> = ({ cue }) => {
  const f = useCurrentFrame();
  const second = ease(f, cue("Context", 66) - 4, 20);
  const pointer = ease(f, cue("Read", 142) - 4, 20);
  return (
    <Stage>
      <Rise p={ease(f, 2, 20)}>
        <text x={800} y={360} textAnchor="middle" {...type.l()}>
          One decision, many rules.
        </text>
      </Rise>
      <Rise p={second}>
        <text x={800} y={450} textAnchor="middle" {...type.l()}>
          <tspan fill={theme.accent}>Context</tspan> decides where each belongs.
        </text>
      </Rise>
      <Rise p={pointer}>
        <rect x={520} y={560} width={560} height={80} rx={40} {...ruleFill()} />
        <text x={800} y={611} textAnchor="middle" {...type.m()} fill={theme.accent}>
          {"Read how context works →"}
        </text>
        <text x={800} y={700} textAnchor="middle" {...type.s()}>
          neokapi.github.io/kapi/context
        </text>
      </Rise>
    </Stage>
  );
};

/** The diagrams demo.yaml names, by id. */
export const E2_BEATS: Record<string, React.FC<BeatProps>> = {
  title: Title,
  rename: Rename,
  surfaces: Surfaces,
  "one-rule": OneRule,
  map: MapBeat,
  assistant: Assistant,
  end: End,
};
