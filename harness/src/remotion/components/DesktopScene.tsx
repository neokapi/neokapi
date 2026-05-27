import React from "react";
import {
  AbsoluteFill,
  OffthreadVideo,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import type { Screencast, ScreencastBeat, ZoomRect } from "../../types.ts";
import { theme } from "./theme.ts";
import type { ThemeMode } from "./theme.ts";

const Light: React.FC<{ c: string }> = ({ c }) => (
  <span style={{ width: 13, height: 13, borderRadius: 13, background: c, display: "inline-block", boxShadow: "0 0 0 0.5px rgba(0,0,0,0.18)" }} />
);

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;

// How the camera gets from one beat's composed shot to the next.
//   glide — quick smooth reframe to the new angle, then hold (premium, calm)
//   cut   — editorial cut to the new angle on each beat (punchy, keynote)
//   orbit — one continuous orbiting take, no per-beat reframes (cinematic)
// Swapped per preview render; the chosen one becomes the committed default.
export type CameraStyle = "glide" | "cut" | "orbit";
// `as` (not a typed const) so TS keeps the union type and doesn't narrow the
// branch comparisons below to the literal initializer.
const CAMERA_STYLE = "glide" as CameraStyle;
const GLIDE_FRAMES = 18; // ~0.6s reframe at 30fps

// Virtual-camera pose: how the window card sits in the 3D canvas.
interface Cam {
  s: number; // dolly (scale)
  tx: number; // pan x (px)
  ty: number; // pan y (px)
  yaw: number; // deg about Y
  pitch: number; // deg about X
}

/**
 * The composed "shot" for a beat: the window seen from one distinct angle, held
 * for the whole beat. Variety comes from the ANGLE, not from zooming in and back
 * out. Region beats lean toward and gently PUSH into the region (a lean-in, not
 * a hard zoom — scale is capped so text stays legible); full beats present the
 * whole window from a wider angle. The lean direction alternates per beat and
 * the yaw/pitch magnitude varies, so consecutive shots show the window from
 * clearly different perspectives.
 */
function shotPose(zoom: ZoomRect | null, bw: number, bh: number, availW: number, availH: number, idx: number): Cam {
  const alt = idx % 2 === 0 ? 1 : -1;
  const yawMag = 5 + 3 * Math.abs(Math.sin(idx)); // 5..8, varied per beat (gentle tilt)
  const pitchBase = 2 + 2 * Math.sin(idx * 1.3); // ~0..4, varied per beat
  if (!zoom) {
    // Wide hero: essentially the whole window, lightly turned. Kept near 1.0 so
    // the full screen reads; the frosted backdrop fills the small margin.
    return { s: 1.05, tx: 0, ty: 0, yaw: alt * yawMag, pitch: clamp(pitchBase + 1, 0, 6) };
  }
  // Gentle lean toward the region — a soft push that keeps MOST of the window in
  // frame (not a crop-in zoom). Capped low so the whole screen stays readable.
  const s = clamp(Math.min(availW / (zoom.w * bw), availH / (zoom.h * bh)), 1.05, 1.28);
  const cx = zoom.x + zoom.w / 2;
  const cy = zoom.y + zoom.h / 2;
  const tx = -s * (cx - 0.5) * bw;
  const ty = -s * (cy - 0.5) * bh;
  const yaw = clamp(alt * yawMag + (0.5 - cx) * 7, -10, 10);
  const pitch = clamp(pitchBase + (0.5 - cy) * 4, -2, 7);
  return { s, tx, ty, yaw, pitch };
}

/**
 * One Kapi Desktop walkthrough beat, presented as a window in a 3D canvas. Each
 * beat is a single composed shot from a distinct angle (see shotPose), held with
 * a slow breathing drift — variety comes from re-framing the window between
 * beats, not from zooming the picture in and out. The transition between shots
 * is set by CAMERA_STYLE (glide / cut / orbit).
 */
export const DesktopScene: React.FC<{
  demoId: string;
  screencast: Screencast;
  themeMode: ThemeMode;
  beat: ScreencastBeat;
  /** The previous beat's data, so glide mode can ease from its shot. */
  prevBeat: ScreencastBeat | null;
  sceneIndex: number;
  /** This scene's start frame in the whole composition, so camera drift is a
   *  continuous function of global time (no phase jump at scene boundaries). */
  globalFrom: number;
  caption: string;
  sceneDurationFrames: number;
}> = ({ demoId, screencast, themeMode, beat, prevBeat, sceneIndex, globalFrom, caption, sceneDurationFrames }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  // ── Window / body geometry (fit the screencast aspect into the canvas) ──
  const FW = 1920;
  const FH = 1080;
  const padTop = 64;
  const padSide = 150;
  const capBand = 150;
  const availW = FW - 2 * padSide;
  const availH = FH - padTop - capBand;
  const aspect = screencast.width / screencast.height;
  let bw = availW;
  let bh = bw / aspect;
  if (bh > availH) {
    bh = availH;
    bw = bh * aspect;
  }

  // ── Video slice: play [tStart,tEnd] across this scene's frames ──
  // EXACT rate (no clamp): the source advances tStart→tEnd precisely over the
  // scene, so the last frame here is the next beat's tStart (beats are
  // contiguous) — the recording is one continuous take with no jump at the
  // boundary. A clamp would over/under-run the slice and make the screencast
  // jump backward/forward at scene cuts (the "clip at an unnatural place").
  const sliceSec = Math.max(0.1, beat.tEnd - beat.tStart);
  const sceneSec = Math.max(0.1, sceneDurationFrames / fps);
  const playbackRate = Math.max(0.05, sliceSec / sceneSec);
  const startFrom = Math.round(beat.tStart * fps);
  const videoSrc = staticFile(`${demoId}/${screencast.video[themeMode]}`);

  // ── Camera: one composed shot per beat, held with a slow breathing drift.
  // Variety is in the ANGLE between beats; the transition is set by CAMERA_STYLE.
  // `back` overshoots its target slightly then settles — the glide reframe lands
  // with a bit of motion rather than a flat ease.
  const back = (t: number) => {
    t = clamp(t, 0, 1);
    const c1 = 0.9; // gentle overshoot (~5%)
    const c3 = c1 + 1;
    return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2);
  };
  const p = clamp(frame / sceneDurationFrames, 0, 1);
  // Global time → drift is continuous across scene boundaries (no phase reset).
  const tSec = (globalFrom + frame) / fps;
  const driftYaw = 0.9 * Math.sin(tSec * 0.5);
  const driftPitch = 0.5 * Math.sin(tSec * 0.4);
  const driftZ = 16 * Math.sin(tSec * 0.42);

  const shot = shotPose(beat.zoom, bw, bh, availW, availH, sceneIndex);

  let cam: Cam;
  if (CAMERA_STYLE === "orbit") {
    // One continuous orbiting take: a global yaw/pitch swing, plus a soft push
    // toward the active region that eases to zero at the beat edges — so the
    // pose is continuous across every boundary (no cut, no reframe).
    const baseYaw = 10 * Math.sin(tSec * 0.16);
    const basePitch = 4 + 3 * Math.sin(tSec * 0.13 + 1);
    const baseS = 1.05;
    const pushEnv = Math.sin(clamp(p, 0, 1) * Math.PI); // 0 at beat edges, 1 mid-beat
    if (beat.zoom) {
      const z = beat.zoom;
      const target = clamp(Math.min(availW / (z.w * bw), availH / (z.h * bh)), 1, 1.4);
      const s = lerp(baseS, target, pushEnv * 0.85);
      const cx = z.x + z.w / 2;
      const cy = z.y + z.h / 2;
      cam = {
        s,
        tx: -s * (cx - 0.5) * bw * pushEnv,
        ty: -s * (cy - 0.5) * bh * pushEnv,
        yaw: baseYaw + (0.5 - cx) * 8 * pushEnv + driftYaw,
        pitch: basePitch + (0.5 - cy) * 4 * pushEnv + driftPitch,
      };
    } else {
      cam = { s: baseS, tx: 0, ty: 0, yaw: baseYaw + driftYaw, pitch: basePitch + driftPitch };
    }
  } else if (CAMERA_STYLE === "cut") {
    // Editorial cut: hold this beat's composed shot for the whole beat. The
    // angle simply differs across the boundary, so it reads as an edit.
    cam = { s: shot.s, tx: shot.tx, ty: shot.ty, yaw: shot.yaw + driftYaw, pitch: shot.pitch + driftPitch };
  } else {
    // Glide: ease from the PREVIOUS beat's shot to this one over GLIDE_FRAMES,
    // then hold. Both sides of a boundary equal the previous beat's shot, so the
    // reframe happens just AFTER the cut — continuous, then one smooth move.
    const from = prevBeat
      ? shotPose(prevBeat.zoom, bw, bh, availW, availH, sceneIndex - 1)
      : { s: shot.s * 0.92, tx: shot.tx * 0.85, ty: shot.ty * 0.85, yaw: shot.yaw * 0.3, pitch: shot.pitch * 0.5 };
    const g = back(frame / GLIDE_FRAMES);
    cam = {
      s: lerp(from.s, shot.s, g),
      tx: lerp(from.tx, shot.tx, g),
      ty: lerp(from.ty, shot.ty, g),
      yaw: lerp(from.yaw, shot.yaw, g) + driftYaw,
      pitch: lerp(from.pitch, shot.pitch, g) + driftPitch,
    };
  }

  const capOpacity = spring({ frame: frame - 4, fps, config: { damping: 200 } });

  // The window itself does NOT fade — the screencast is one continuous take
  // (contiguous beats) and the camera sits at the same full pose on both sides
  // of every boundary, so the tour flows without a cut. Only the caption text
  // crossfades so the lower-third doesn't pop between scenes.
  const FADE = 8;
  const capFade = Math.min(clamp(frame / FADE, 0, 1), clamp((sceneDurationFrames - frame) / FADE, 0, 1));

  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans }}>
      {/* Frosted backdrop: a blurred, dimmed copy of the same frame fills the
          whole frame behind the card, so an angled or pushed-in window never
          leaves a flat empty void beside it — it floats on a soft continuation
          of its own content. */}
      <AbsoluteFill style={{ overflow: "hidden" }}>
        <OffthreadVideo
          src={videoSrc}
          startFrom={startFrom}
          playbackRate={playbackRate}
          muted
          style={{
            position: "absolute",
            inset: 0,
            width: "100%",
            height: "100%",
            objectFit: "cover",
            transform: "scale(1.3)",
            filter: "blur(40px) saturate(0.9) brightness(0.66)",
          }}
        />
        <AbsoluteFill style={{ background: "radial-gradient(130% 130% at 50% 40%, rgba(8,11,18,0.16), rgba(4,6,12,0.68))" }} />
      </AbsoluteFill>

      {/* 3D canvas: the window is a card; the transform IS the camera move. */}
      <AbsoluteFill
        style={{
          perspective: 2200,
          perspectiveOrigin: "50% 42%",
          paddingTop: padTop,
          alignItems: "center",
        }}
      >
        <div
          style={{
            width: bw,
            height: bh,
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
          <OffthreadVideo src={videoSrc} startFrom={startFrom} playbackRate={playbackRate} muted style={{ position: "absolute", top: 0, left: 0, width: bw, height: bh }} />
          {/* Inline macOS traffic lights on the app's own top bar — part of the card. */}
          <div style={{ position: "absolute", top: 17, left: 20, display: "flex", gap: 9 }}>
            <Light c="#ff5f57" />
            <Light c="#febc2e" />
            <Light c="#28c840" />
          </div>
        </div>
      </AbsoluteFill>

      {/* caption lower-third (flat, below the canvas) */}
      <AbsoluteFill style={{ top: undefined, bottom: 0, height: capBand, flexDirection: "row", alignItems: "center", justifyContent: "center" }}>
        {caption ? (
          <div
            style={{
              maxWidth: 1500,
              padding: "16px 32px",
              background: "rgba(8,11,19,0.9)",
              border: "1px solid rgba(255,255,255,0.14)",
              borderRadius: 14,
              color: "#f4f7ff",
              fontSize: 29,
              lineHeight: 1.3,
              fontWeight: 500,
              textAlign: "center",
              opacity: capOpacity * capFade,
              transform: `translateY(${interpolate(capOpacity, [0, 1], [10, 0])}px)`,
            }}
          >
            {caption}
          </div>
        ) : null}
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
