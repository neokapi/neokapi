import React from "react";
import { AbsoluteFill, Img, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { theme, KAPI, BOWRAIN } from "./theme.ts";

/** The product badge: the bowrain logo for bowrain demos, else the neokapi mascot,
 *  each centered on a white rounded badge. */
const Badge: React.FC<{ size?: number; brand?: Brand }> = ({ size = 128, brand = "claude" }) => {
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

type Brand = "claude" | "kapi" | "desktop" | "bowrain";

const Lockup: React.FC<{ size?: number; brand?: Brand }> = ({ size = 1, brand = "claude" }) => (
  <div style={{ display: "flex", alignItems: "center", gap: 16 * size, fontSize: 30 * size, fontWeight: 700, letterSpacing: 0.4 }}>
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

const Chip: React.FC<{ label: string; delay: number }> = ({ label, delay }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const s = spring({ frame: frame - delay, fps, config: { damping: 200 } });
  return (
    <span style={{ padding: "10px 20px", borderRadius: 999, background: "rgba(122,162,255,0.10)", border: `1px solid ${theme.toolBorder}`, color: theme.accent, fontSize: 24, fontWeight: 500, opacity: s, transform: `translateY(${interpolate(s, [0, 1], [12, 0])}px)` }}>
      {label}
    </span>
  );
};

export const TitleCard: React.FC<{ title: string; subtitle: string; tagline?: string; aspects: string[]; brand?: Brand }> = ({
  title,
  subtitle,
  tagline,
  aspects,
  brand = "claude",
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  const sub = spring({ frame: frame - 8, fps, config: { damping: 200 } });
  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans, justifyContent: "center", alignItems: "center", padding: 120, textAlign: "center" }}>
      <div style={{ opacity: intro, transform: `translateY(${interpolate(intro, [0, 1], [16, 0])}px)`, marginBottom: 28 }}>
        <Badge size={132} brand={brand} />
      </div>
      <div style={{ opacity: spring({ frame: frame - 2, fps, config: { damping: 200 } }), marginBottom: 40 }}>
        <Lockup size={1.15} brand={brand} />
      </div>
      <div style={{ fontSize: 88, fontWeight: 750, color: theme.text, letterSpacing: -0.02 * 88, lineHeight: 1.05, opacity: intro, transform: `translateY(${interpolate(intro, [0, 1], [26, 0])}px)`, maxWidth: 1500 }}>
        {title}
      </div>
      <div style={{ fontSize: 38, color: theme.dim, marginTop: 26, maxWidth: 1300, lineHeight: 1.35, opacity: sub }}>{subtitle}</div>
      {tagline ? <div style={{ fontSize: 26, color: theme.accent2, marginTop: 30, opacity: sub, fontStyle: "italic" }}>{tagline}</div> : null}
      <div style={{ display: "flex", gap: 16, marginTop: 56, flexWrap: "wrap", justifyContent: "center", maxWidth: 1400 }}>
        {aspects.slice(0, 6).map((a, i) => (
          <Chip key={a} label={a} delay={18 + i * 4} />
        ))}
      </div>
    </AbsoluteFill>
  );
};

export const OutroCard: React.FC<{ title: string; tagline?: string; aspects: string[]; brand?: Brand }> = ({ title, tagline, aspects, brand = "claude" }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans, justifyContent: "center", alignItems: "center", padding: 120, textAlign: "center" }}>
      <div style={{ fontSize: 30, color: theme.dim, opacity: intro, letterSpacing: 4, textTransform: "uppercase" }}>What you saw</div>
      <div style={{ fontSize: 64, fontWeight: 720, color: theme.text, marginTop: 18, maxWidth: 1400, lineHeight: 1.1, opacity: intro, transform: `translateY(${interpolate(intro, [0, 1], [20, 0])}px)` }}>
        {title}
      </div>
      <div style={{ display: "flex", gap: 16, marginTop: 44, flexWrap: "wrap", justifyContent: "center", maxWidth: 1400 }}>
        {aspects.slice(0, 6).map((a, i) => (
          <Chip key={a} label={a} delay={10 + i * 4} />
        ))}
      </div>
      {tagline ? <div style={{ fontSize: 30, color: theme.accent2, marginTop: 52, opacity: spring({ frame: frame - 24, fps, config: { damping: 200 } }), maxWidth: 1200, lineHeight: 1.4 }}>{tagline}</div> : null}
      <div style={{ marginTop: 60, display: "flex", flexDirection: "column", alignItems: "center", gap: 18, opacity: spring({ frame: frame - 30, fps, config: { damping: 200 } }) }}>
        <Badge size={104} brand={brand} />
        <Lockup brand={brand} />
      </div>
      <div style={{ marginTop: 16, color: theme.faint, fontSize: 22, opacity: spring({ frame: frame - 34, fps, config: { damping: 200 } }) }}>
        {brand === "bowrain" ? "the team localization platform · bowrain.cloud" : "the open localization engine · neokapi.github.io"}
      </div>
    </AbsoluteFill>
  );
};
