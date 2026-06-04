import React from "react";
import {
  AbsoluteFill,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
  type CalculateMetadataFunction,
} from "remotion";
import { TransitionSeries, springTiming } from "@remotion/transitions";
import { fade } from "@remotion/transitions/fade";
import { slide } from "@remotion/transitions/slide";
import type { Screencast, ScreencastBeat } from "../../types.ts";
import { BOWRAIN, FPS, HEIGHT, WIDTH, setTheme, theme, type ThemeMode } from "../components/theme.ts";
import { TitleCard, OutroCard } from "../components/Cards.tsx";
import { DesktopScene } from "../components/DesktopScene.tsx";

// ── Sizzle: a single bowrain landing hero that montages the feature screencasts.
// Title card → N feature clips (each a framed beat from an existing bowrain demo,
// reusing DesktopScene's 3D window + camera) with a modern kinetic lower-third,
// stitched with @remotion/transitions: a fade into/out of the title/outro and a
// directional slide between feature clips. Silent — it reads as a reel.
//
// @remotion/transitions owns the overlap/timeline math (each Transition overlaps
// the adjacent Sequences by `CROSS` frames), so the composition's duration is
// `Σ(segment durations) − (number of transitions) × CROSS` — computed once in
// sizzleCalcMeta and mirrored by TransitionSeries at render.

const CROSS = 16; // transition length in frames (~0.53s) — uniform, so the math stays simple
const TITLE_SEC = 2.7;
const OUTRO_SEC = 3.2;

/** One montage clip: a beat from an existing bowrain demo + the headline shown over it. */
interface ClipPlan {
  demoId: string;
  beatId: string;
  /** Cap the slice (and thus the on-screen time) so each clip stays punchy. */
  maxDurSec?: number;
  title: string;
  subtitle: string;
}

/** Resolved at metadata time: the plan + the loaded screencast + the (capped) beat. */
interface ResolvedClip extends ClipPlan {
  screencast: Screencast;
  beat: ScreencastBeat;
  durationFrames: number;
}

export interface SizzleProps {
  id: string;
  themeMode?: ThemeMode;
  stamp?: string;
  clips?: ResolvedClip[];
  title?: string;
  subtitle?: string;
  tagline?: string;
  aspects?: string[];
  outroTitle?: string;
  outroTagline?: string;
  [key: string]: unknown;
}

// The authored reel. Beats/ids verified against each demo's screencast.json.
const PLAN: ClipPlan[] = [
  { demoId: "bowrain-desktop-dashboard", beatId: "projects", maxDurSec: 3.0, title: "Your localization platform", subtitle: "Every project, in one home." },
  { demoId: "bowrain-web-editor", beatId: "split", maxDurSec: 4.0, title: "One shared editor", subtitle: "Source and target, side by side." },
  { demoId: "bowrain-web-collaboration", beatId: "teammate-joins", maxDurSec: 4.0, title: "Real-time collaboration", subtitle: "Your team translates together, live." },
  { demoId: "bowrain-web-governance", beatId: "tm-search", maxDurSec: 3.8, title: "Memory & terminology", subtitle: "Consistency, shared and enforced." },
  { demoId: "bowrain-web-review", beatId: "review", maxDurSec: 4.0, title: "Review & approval", subtitle: "Nothing ships unchecked." },
  { demoId: "bowrain-web-correction-loop", beatId: "promote", maxDurSec: 3.8, title: "Corrections become checks", subtitle: "Quality that compounds." },
];

const META = {
  title: "Bowrain",
  subtitle: "Govern AI-translated content, together.",
  tagline: "Shared memory, terminology, and checks that learn from every correction.",
  aspects: ["Shared editor", "Real-time collaboration", "Terminology", "Review", "Quality checks"],
  outroTitle: "The team localization platform",
  outroTagline: "Built on the open kapi framework.",
};

async function fetchJson<T>(rel: string): Promise<T | null> {
  try {
    const res = await fetch(staticFile(rel));
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

/** Load each clip's screencast.json, pick + cap its beat, and size the timeline. */
export const sizzleCalcMeta: CalculateMetadataFunction<SizzleProps> = async ({ props }) => {
  const mode: ThemeMode = props.themeMode ?? "dark";
  const clips: ResolvedClip[] = [];
  for (const p of PLAN) {
    const sc = await fetchJson<Screencast>(`${p.demoId}/screencast.json`);
    if (!sc) continue;
    const beats = sc.beats[mode] ?? sc.beats.dark ?? [];
    const b = beats.find((x) => x.id === p.beatId) ?? beats[0];
    if (!b) continue;
    const cap = p.maxDurSec ?? 3.8;
    const tEnd = Math.min(b.tEnd, b.tStart + cap);
    const durationFrames = Math.max(Math.round((tEnd - b.tStart) * FPS), Math.round(2.4 * FPS));
    // zoom:null → DesktopScene frames the WHOLE window (its "wide hero" pose, a
    // light tilt) instead of pushing into a per-beat sub-region. For a fast
    // montage that's cleaner and more consistent than region push-ins, which
    // frame some clips into a large empty void.
    clips.push({ ...p, screencast: sc, beat: { id: b.id, tStart: b.tStart, tEnd, zoom: null }, durationFrames });
  }
  const titleFr = Math.round(TITLE_SEC * FPS);
  const outroFr = Math.round(OUTRO_SEC * FPS);
  const segCount = clips.length + 2; // title + clips + outro
  const transitions = segCount - 1;
  const total = titleFr + clips.reduce((n, c) => n + c.durationFrames, 0) + outroFr - transitions * CROSS;
  return {
    durationInFrames: Math.max(FPS, total),
    fps: FPS,
    width: WIDTH,
    height: HEIGHT,
    props: { ...props, clips, ...META },
  };
};

/** Theme-aware bottom scrim so the kinetic lower-third reads regardless of what
 *  the framed window shows behind it (the camera frames each clip differently). */
const BottomScrim: React.FC<{ mode: ThemeMode }> = ({ mode }) => {
  const c = mode === "light" ? "246,248,252" : "6,8,14";
  return (
    <AbsoluteFill
      style={{
        background: `linear-gradient(to top, rgba(${c},0.92) 0%, rgba(${c},0.5) 12%, rgba(${c},0) 26%)`,
        pointerEvents: "none",
      }}
    />
  );
};

/** Modern kinetic lower-third: a bowrain accent rule + big headline + subline,
 *  springing up at clip start and easing out before the transition. */
const ClipText: React.FC<{ title: string; subtitle: string; durationFrames: number }> = ({ title, subtitle, durationFrames }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const inS = spring({ frame: frame - 3, fps, config: { damping: 200 } });
  const outF = interpolate(frame, [durationFrames - CROSS, durationFrames - 4], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const op = Math.min(inS, outF);
  const rise = interpolate(inS, [0, 1], [26, 0]);
  return (
    <AbsoluteFill style={{ justifyContent: "flex-end", alignItems: "flex-start", padding: "0 0 64px 150px", pointerEvents: "none" }}>
      <div style={{ display: "flex", gap: 22, alignItems: "stretch", opacity: op, transform: `translateY(${rise}px)` }}>
        <div style={{ width: 6, borderRadius: 6, background: BOWRAIN, boxShadow: `0 0 24px ${BOWRAIN}` }} />
        <div>
          <div style={{ fontSize: 62, fontWeight: 760, color: theme.text, lineHeight: 1.04, letterSpacing: -0.6, textShadow: "0 2px 28px rgba(0,0,0,0.55)" }}>{title}</div>
          <div style={{ fontSize: 31, fontWeight: 500, color: theme.dim, marginTop: 10, textShadow: "0 2px 18px rgba(0,0,0,0.55)" }}>{subtitle}</div>
        </div>
      </div>
    </AbsoluteFill>
  );
};

const StampOverlay: React.FC<{ stamp: string }> = ({ stamp }) => (
  <AbsoluteFill style={{ justifyContent: "flex-end", alignItems: "flex-end", padding: 14, pointerEvents: "none" }}>
    <div style={{ fontSize: 13, color: theme.faint, fontFamily: theme.fontMono, opacity: 0.55 }}>{stamp}</div>
  </AbsoluteFill>
);

const timing = springTiming({ config: { damping: 200 }, durationInFrames: CROSS });

export const Sizzle: React.FC<SizzleProps> = (props) => {
  const mode: ThemeMode = props.themeMode ?? "dark";
  setTheme(mode);
  const clips = props.clips ?? [];
  const titleFr = Math.round(TITLE_SEC * FPS);
  const outroFr = Math.round(OUTRO_SEC * FPS);

  // Interleave Sequences with Transitions: fade for the title/outro bookends, a
  // directional slide between feature clips (the "clip between features" motion).
  const children: React.ReactNode[] = [];
  children.push(
    <TransitionSeries.Sequence key="title" durationInFrames={titleFr}>
      <TitleCard title={props.title ?? META.title} subtitle={props.subtitle ?? META.subtitle} tagline={props.tagline ?? META.tagline} aspects={props.aspects ?? META.aspects} brand="bowrain" />
    </TransitionSeries.Sequence>,
  );
  let driftOffset = titleFr;
  clips.forEach((c, i) => {
    children.push(
      <TransitionSeries.Transition key={`t-${i}`} presentation={i === 0 ? fade() : slide({ direction: "from-right" })} timing={timing} />,
    );
    const globalFrom = driftOffset;
    children.push(
      <TransitionSeries.Sequence key={`clip-${i}`} durationInFrames={c.durationFrames}>
        <DesktopScene
          demoId={c.demoId}
          screencast={c.screencast}
          themeMode={mode}
          beat={c.beat}
          prevBeat={null}
          sceneIndex={i}
          globalFrom={globalFrom}
          caption=""
          sceneDurationFrames={c.durationFrames}
        />
        <BottomScrim mode={mode} />
        <ClipText title={c.title} subtitle={c.subtitle} durationFrames={c.durationFrames} />
      </TransitionSeries.Sequence>,
    );
    driftOffset += c.durationFrames - CROSS;
  });
  children.push(<TransitionSeries.Transition key="t-outro" presentation={fade()} timing={timing} />);
  children.push(
    <TransitionSeries.Sequence key="outro" durationInFrames={outroFr}>
      <OutroCard title={props.outroTitle ?? META.outroTitle} tagline={props.outroTagline ?? META.outroTagline} aspects={props.aspects ?? META.aspects} brand="bowrain" />
    </TransitionSeries.Sequence>,
  );

  return (
    <AbsoluteFill style={{ background: theme.bg }}>
      <TransitionSeries>{children}</TransitionSeries>
      {props.stamp ? <StampOverlay stamp={props.stamp} /> : null}
    </AbsoluteFill>
  );
};
