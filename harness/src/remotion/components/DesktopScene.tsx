import React from "react";
import { AbsoluteFill, Freeze, Sequence, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { Audio, Video } from "@remotion/media";
import { mouseClick } from "@remotion/sfx";
import type { Screencast, ScreencastBeat, ZoomRect } from "../../types.ts";
import { theme } from "./theme.ts";
import type { ThemeMode } from "./theme.ts";
import { FRAME_W, type SceneLayout } from "./layout.ts";
import { BoxMark } from "./Marks.tsx";

const Light: React.FC<{ c: string }> = ({ c }) => (
  <span style={{ width: 13, height: 13, borderRadius: 13, background: c, display: "inline-block", boxShadow: "0 0 0 0.5px rgba(0,0,0,0.18)" }} />
);

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;

/** Slowest the picture may play before the last frame is held instead. */
export const MIN_PLAYBACK_RATE = 0.8;
/** Fastest the picture may play to fit a scene shorter than its beat. */
export const MAX_PLAYBACK_RATE = 2;
/** Furthest a crop pushes into the window on its own; `zoom` can take it to 3. */
export const MAX_CROP_SCALE = 2.5;
/** A region this close to the whole window is shown as the whole window. */
const FULL_WINDOW_FIT = 1.15;

export interface PlayPlan {
  rate: number;
  /** Frames the picture moves; the rest of the scene holds the last of them. */
  playFrames: number;
  heldFrames: number;
}

/**
 * How a beat's slice fills its scene. Near real time it plays at its natural
 * rate (never slower than MIN_PLAYBACK_RATE, never faster than MAX); when the
 * narration outruns the picture, the picture plays at 1x and its last frame
 * holds for the remainder, so a cursor never crawls.
 */
export function planPlayback(sliceSec: number, sceneFrames: number, fps: number): PlayPlan {
  const slice = Math.max(0.1, sliceSec);
  const sceneSec = Math.max(1 / fps, sceneFrames / fps);
  const ratio = slice / sceneSec;
  if (ratio >= MIN_PLAYBACK_RATE) {
    return { rate: Math.min(MAX_PLAYBACK_RATE, ratio), playFrames: sceneFrames, heldFrames: 0 };
  }
  const playFrames = Math.max(1, Math.min(sceneFrames, Math.round(slice * fps)));
  return { rate: 1, playFrames, heldFrames: sceneFrames - playFrames };
}

/** Scene frames at which the recorded clicks inside the played slice land. */
export function clickFrames(clicks: number[] | undefined, beat: Pick<ScreencastBeat, "tStart" | "tEnd">, plan: PlayPlan, fps: number): number[] {
  if (!clicks || clicks.length === 0) return [];
  return clicks
    .filter((c) => c >= beat.tStart && c < beat.tEnd)
    .map((c) => Math.round(((c - beat.tStart) / plan.rate) * fps))
    .filter((f) => f >= 0 && f < plan.playFrames);
}

// Virtual-camera pose: how the window card sits in the 3D canvas.
interface Cam {
  s: number; // dolly (scale)
  tx: number; // pan x (px)
  ty: number; // pan y (px)
  yaw: number; // deg about Y
  pitch: number; // deg about X
}

interface Geo {
  bw: number;
  bh: number;
  bar: number;
  cardW: number;
  cardH: number;
  cardCx: number;
  cardCy: number;
  areaW: number;
  areaH: number;
  areaCx: number;
  areaCy: number;
}

/**
 * The composed shot for a beat. A full-window beat shows the whole window at
 * 1.0, lightly turned; a region beat crops to its region: the scale that fits
 * the region into the crop area (above the captions), clamped to 1 to 2.5,
 * times the beat's own `zoom`, and the region's centre brought to the area's
 * centre. The tilt shrinks with the crop so a pushed-in window stays flat.
 */
function shotPose(region: ZoomRect | null, zoomMul: number, geo: Geo, idx: number): Cam {
  const alt = idx % 2 === 0 ? 1 : -1;
  const yawMag = 3 + 2 * Math.abs(Math.sin(idx));
  const pitchBase = 1 + 1.5 * Math.sin(idx * 1.3);
  const fit = region ? Math.min(geo.areaW / (region.w * geo.bw), geo.areaH / (region.h * geo.bh)) : 1;
  if (!region || fit < FULL_WINDOW_FIT) {
    const s = clamp(Math.max(1, fit) * zoomMul, 1, 3);
    return { s, tx: 0, ty: 0, yaw: (alt * yawMag) / s, pitch: clamp(pitchBase + 1, 0, 4) / s };
  }
  const s = clamp(clamp(fit, 1, MAX_CROP_SCALE) * zoomMul, 1, 3);
  const cx = region.x + region.w / 2;
  const cy = region.y + region.h / 2;
  const ox = cx * geo.bw - geo.cardW / 2;
  const oy = geo.bar + cy * geo.bh - geo.cardH / 2;
  return {
    s,
    tx: geo.areaCx - geo.cardCx - s * ox,
    ty: geo.areaCy - geo.cardCy - s * oy,
    yaw: clamp(alt * yawMag + (0.5 - cx) * 4, -6, 6) / s,
    pitch: clamp(pitchBase + (0.5 - cy) * 3, -2, 5) / s,
  };
}

const GLIDE_FRAMES = 18; // ~0.6s reframe at 30fps

/**
 * One desktop walkthrough beat, presented as a window in a 3D canvas. The
 * camera glides from the previous beat's shot to this one over GLIDE_FRAMES,
 * then holds with a slow breathing drift. The picture plays its slice at a
 * natural rate and holds its last frame when the narration is longer.
 */
export const DesktopScene: React.FC<{
  demoId: string;
  screencast: Screencast;
  themeMode: ThemeMode;
  beat: ScreencastBeat;
  /** The previous beat's data, so the camera can ease from its shot. */
  prevBeat: ScreencastBeat | null;
  sceneIndex: number;
  /** This scene's start frame in the whole composition, so camera drift is a
   *  continuous function of global time (no phase jump at scene boundaries). */
  globalFrom: number;
  sceneDurationFrames: number;
  layout: SceneLayout;
  /** An authored crop box, over the recorded one. */
  crop?: ZoomRect;
  /** An authored multiplier on the crop's fitted scale. */
  zoom?: number;
  /** An authored box to draw around, over the recorded one. */
  highlight?: ZoomRect;
}> = ({ demoId, screencast, themeMode, beat, prevBeat, sceneIndex, globalFrom, sceneDurationFrames, layout, crop, zoom, highlight }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  // ── Window / body geometry (fit the screencast aspect into the picture box) ──
  // Web demos capture a browser SPA with no native window chrome, so they are
  // framed inside a browser top bar (traffic lights + an address pill) and the
  // capture sits BELOW it. Native-app demos keep their own title-bar gutter,
  // so they get the lightweight dots overlay instead.
  const isWeb = demoId.startsWith("bowrain-web");
  const bar = isWeb ? 44 : 0;
  const boxW = layout.width;
  const boxH = layout.picBottom - layout.picTop;
  const aspect = screencast.width / screencast.height;
  let bw = boxW;
  let bh = bw / aspect;
  if (bh + bar > boxH) {
    bh = boxH - bar;
    bw = bh * aspect;
  }
  const cardW = bw;
  const cardH = bh + bar;
  const cardLeft = (FRAME_W - cardW) / 2;
  const cardTop = layout.picTop + (boxH - cardH) / 2;
  const areaH = layout.stackBottom - layout.picTop;
  const geo: Geo = {
    bw,
    bh,
    bar,
    cardW,
    cardH,
    cardCx: cardLeft + cardW / 2,
    cardCy: cardTop + cardH / 2,
    areaW: layout.width,
    areaH,
    areaCx: FRAME_W / 2,
    areaCy: layout.picTop + areaH / 2,
  };

  // ── Playback: the slice at a natural rate, or at 1x with the last frame held ──
  const sliceSec = Math.max(0.1, beat.tEnd - beat.tStart);
  const plan = planPlayback(sliceSec, sceneDurationFrames, fps);
  const trimBefore = Math.round(beat.tStart * fps);
  const videoSrc = staticFile(`${demoId}/${screencast.video[themeMode]}`);
  const held = plan.heldFrames > 0;
  const clicks = clickFrames(screencast.clicks?.[themeMode], beat, plan, fps);

  // ── Camera: glide from the previous shot to this one, then breathe ──
  const back = (t: number) => {
    t = clamp(t, 0, 1);
    const c1 = 0.9; // gentle overshoot (~5%)
    const c3 = c1 + 1;
    return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2);
  };
  const tSec = (globalFrom + frame) / fps;
  const driftYaw = 0.7 * Math.sin(tSec * 0.5);
  const driftPitch = 0.4 * Math.sin(tSec * 0.4);
  const driftZ = 12 * Math.sin(tSec * 0.42);

  const region = crop ?? beat.zoom;
  const shot = shotPose(region, zoom ?? 1, geo, sceneIndex);
  const from = prevBeat
    ? shotPose(prevBeat.zoom, 1, geo, sceneIndex - 1)
    : { s: Math.max(1, shot.s * 0.94), tx: shot.tx * 0.85, ty: shot.ty * 0.85, yaw: shot.yaw * 0.3, pitch: shot.pitch * 0.5 };
  const g = back(frame / GLIDE_FRAMES);
  const cam: Cam = {
    s: lerp(from.s, shot.s, g),
    tx: lerp(from.tx, shot.tx, g),
    ty: lerp(from.ty, shot.ty, g),
    yaw: lerp(from.yaw, shot.yaw, g) + driftYaw / shot.s,
    pitch: lerp(from.pitch, shot.pitch, g) + driftPitch / shot.s,
  };

  const mark = highlight ?? beat.highlight ?? null;

  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans }}>
      {/* Frosted backdrop: a blurred, dimmed copy of the same frame fills the
          whole frame behind the card, so an angled or pushed-in window never
          leaves a flat empty void beside it. */}
      <AbsoluteFill style={{ overflow: "hidden" }}>
        <Freeze frame={plan.playFrames - 1} active={held ? (f) => f >= plan.playFrames : false}>
          <Video
            src={videoSrc}
            trimBefore={trimBefore}
            playbackRate={plan.rate}
            muted
            objectFit="cover"
            name="backdrop"
            style={{
              position: "absolute",
              inset: 0,
              width: "100%",
              height: "100%",
              scale: "1.3",
              filter: "blur(40px) saturate(0.9) brightness(0.66)",
            }}
          />
        </Freeze>
        <AbsoluteFill style={{ background: "radial-gradient(130% 130% at 50% 40%, rgba(8,11,18,0.16), rgba(4,6,12,0.68))" }} />
      </AbsoluteFill>

      {/* 3D canvas: the window is a card; the transform IS the camera move. */}
      <AbsoluteFill style={{ perspective: 2200, perspectiveOrigin: "50% 42%" }}>
        <div
          style={{
            position: "absolute",
            left: cardLeft,
            top: cardTop,
            width: cardW,
            height: cardH,
            transformStyle: "preserve-3d",
            transformOrigin: "center center",
            transform: `translate3d(${cam.tx}px, ${cam.ty}px, ${driftZ}px) rotateX(${cam.pitch}deg) rotateY(${cam.yaw}deg) scale(${cam.s})`,
            borderRadius: 14,
            overflow: "hidden",
            background: theme.termBg,
            border: `1px solid ${theme.panelBorder}`,
            boxShadow: "0 50px 120px rgba(0,0,0,0.65), 0 8px 28px rgba(0,0,0,0.45)",
          }}
        >
          {isWeb ? (
            // Browser chrome: traffic lights + an address pill, with the captured
            // web app placed BELOW it so the dots never overlap app content.
            <div
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: cardW,
                height: bar,
                display: "flex",
                alignItems: "center",
                background: theme.chrome,
                borderBottom: `1px solid ${theme.panelBorder}`,
              }}
            >
              <div style={{ display: "flex", gap: 9, marginLeft: 20 }}>
                <Light c="#ff5f57" />
                <Light c="#febc2e" />
                <Light c="#28c840" />
              </div>
              <div
                style={{
                  flex: 1,
                  margin: "0 22px 0 20px",
                  height: 24,
                  borderRadius: 12,
                  background: theme.resultBg,
                  border: `1px solid ${theme.panelBorder}`,
                  display: "flex",
                  alignItems: "center",
                  paddingLeft: 14,
                  color: theme.dim,
                  fontSize: 13,
                  fontFamily: theme.fontSans,
                }}
              >
                bowrain.cloud
              </div>
            </div>
          ) : null}
          <Freeze frame={plan.playFrames - 1} active={held ? (f) => f >= plan.playFrames : false}>
            <Video
              src={videoSrc}
              trimBefore={trimBefore}
              playbackRate={plan.rate}
              muted
              objectFit="fill"
              name={`beat:${beat.id}`}
              style={{ position: "absolute", top: bar, left: 0, width: bw, height: bh }}
            />
          </Freeze>
          {mark ? <BoxMark left={mark.x * bw} top={bar + mark.y * bh} width={mark.w * bw} height={mark.h * bh} /> : null}
          {!isWeb && (
            // Native-app demos: macOS traffic lights overlaid on the app's own
            // title-bar gutter (kapi-desktop reserves pl-16; bowrain-desktop via
            // the bw-desktop-mac marker).
            <div style={{ position: "absolute", top: 17, left: 20, display: "flex", gap: 9 }}>
              <Light c="#ff5f57" />
              <Light c="#febc2e" />
              <Light c="#28c840" />
            </div>
          )}
        </div>
      </AbsoluteFill>

      {/* The recorded clicks, as the sound of a click where the ripple blooms. */}
      {clicks.map((f, i) => (
        <Sequence key={`click-${i}`} from={f} layout="none" name="click">
          <Audio src={mouseClick} volume={0.45} name="click" />
        </Sequence>
      ))}
    </AbsoluteFill>
  );
};
