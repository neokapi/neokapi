import React from "react";
import { AbsoluteFill, Img, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { theme, KAPI, BOWRAIN } from "./theme.ts";
import { OUTRO_FS, POINTER_FS, SAFE_X, SAFE_Y, SUBTITLE_FS, TITLE_FS } from "./layout.ts";

export type Brand = "claude" | "kapi" | "desktop" | "bowrain";

/** The product badge: the bowrain logo for bowrain demos, else the neokapi mascot,
 *  each centered on a white rounded badge. */
const Badge: React.FC<{ size?: number; brand?: Brand }> = ({ size = 96, brand = "claude" }) => {
  const isBowrain = brand === "bowrain";
  return (
    <div
      style={{
        width: size,
        height: size,
        borderRadius: size,
        background: "#fff",
        display: "grid",
        placeItems: "center",
        overflow: "hidden",
        boxShadow: "0 18px 44px rgba(0,0,0,0.45), 0 0 0 1px rgba(255,255,255,0.12)",
      }}
    >
      <Img
        src={staticFile(isBowrain ? "bowrain-logo.png" : "mascot.png")}
        style={isBowrain ? { width: "100%", height: "100%", objectFit: "cover" } : { width: "84%", height: "84%", objectFit: "contain" }}
      />
    </div>
  );
};

const Lockup: React.FC<{ size?: number; brand?: Brand }> = ({ size = 1, brand = "claude" }) => (
  <div style={{ display: "flex", alignItems: "center", gap: 16 * size, fontSize: 34 * size, fontWeight: 700, letterSpacing: 0.4 }}>
    {brand === "bowrain" ? (
      <span style={{ color: BOWRAIN }}>Bowrain</span>
    ) : (
      <span style={{ color: KAPI }}>kapi</span>
    )}
    {brand === "claude" ? (
      <>
        <span style={{ color: theme.faint, fontWeight: 400 }}>×</span>
        <span style={{ color: theme.text }}>Claude&nbsp;Code</span>
      </>
    ) : brand === "desktop" ? (
      <>
        <span style={{ color: theme.faint, fontWeight: 400 }}>·</span>
        <span style={{ color: theme.text, fontWeight: 600 }}>Desktop</span>
      </>
    ) : brand === "bowrain" ? null : (
      <>
        <span style={{ color: theme.faint, fontWeight: 400 }}>·</span>
        <span style={{ color: theme.text, fontWeight: 600 }}>toolbox</span>
      </>
    )}
  </div>
);

/** The opening card: the claim, and one line under it. */
export const TitleCard: React.FC<{ title: string; subtitle: string; brand?: Brand }> = ({ title, subtitle, brand = "claude" }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  const sub = spring({ frame: frame - 8, fps, config: { damping: 200 } });
  return (
    <AbsoluteFill
      style={{
        background: theme.bgGrad,
        fontFamily: theme.fontSans,
        justifyContent: "center",
        alignItems: "center",
        padding: `${SAFE_Y}px ${SAFE_X}px`,
        textAlign: "center",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 22, opacity: intro, translate: `0px ${interpolate(intro, [0, 1], [16, 0])}px`, marginBottom: 44 }}>
        <Badge size={96} brand={brand} />
        <Lockup size={1.1} brand={brand} />
      </div>
      <div
        style={{
          fontSize: TITLE_FS,
          fontWeight: 750,
          color: theme.text,
          letterSpacing: -0.02 * TITLE_FS,
          lineHeight: 1.05,
          maxWidth: 1920 - 2 * SAFE_X,
          opacity: intro,
          translate: `0px ${interpolate(intro, [0, 1], [26, 0])}px`,
        }}
      >
        {title}
      </div>
      <div style={{ fontSize: SUBTITLE_FS, color: theme.dim, marginTop: 30, maxWidth: 1920 - 2 * SAFE_X, lineHeight: 1.3, opacity: sub }}>{subtitle}</div>
    </AbsoluteFill>
  );
};

/** The closing card: one instruction, and one pointer under it. */
export const OutroCard: React.FC<{ line: string; pointer: string; brand?: Brand }> = ({ line, pointer, brand = "claude" }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  const second = spring({ frame: frame - 10, fps, config: { damping: 200 } });
  return (
    <AbsoluteFill
      style={{
        background: theme.bgGrad,
        fontFamily: theme.fontSans,
        justifyContent: "center",
        alignItems: "center",
        padding: `${SAFE_Y}px ${SAFE_X}px`,
        textAlign: "center",
      }}
    >
      <div
        style={{
          fontSize: OUTRO_FS,
          fontWeight: 700,
          color: theme.text,
          lineHeight: 1.15,
          maxWidth: 1920 - 2 * SAFE_X,
          opacity: intro,
          translate: `0px ${interpolate(intro, [0, 1], [20, 0])}px`,
        }}
      >
        {line}
      </div>
      {pointer ? (
        <div
          style={{
            marginTop: 44,
            padding: "16px 34px",
            borderRadius: 16,
            background: theme.toolBg,
            border: `1px solid ${theme.toolBorder}`,
            color: theme.accent,
            fontFamily: theme.fontMono,
            fontSize: POINTER_FS,
            lineHeight: 1.3,
            maxWidth: 1920 - 2 * SAFE_X,
            opacity: second,
            translate: `0px ${interpolate(second, [0, 1], [12, 0])}px`,
          }}
        >
          {pointer}
        </div>
      ) : null}
      <div style={{ marginTop: 72, opacity: spring({ frame: frame - 20, fps, config: { damping: 200 } }) }}>
        <Lockup size={0.95} brand={brand} />
      </div>
    </AbsoluteFill>
  );
};
