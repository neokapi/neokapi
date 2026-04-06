import React from "react";
import type { Branding } from "../schema";

interface TerminalFrameProps {
  title?: string;
  branding: Branding;
  children: React.ReactNode;
}

export const TerminalFrame: React.FC<TerminalFrameProps> = ({ title, branding, children }) => {
  const radius = branding.cornerRadius;

  return (
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        backgroundColor: "#0d1117",
        padding: 32,
        boxSizing: "border-box",
      }}
    >
      {/* Terminal title bar */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "10px 16px",
          backgroundColor: "#161b22",
          borderTopLeftRadius: radius,
          borderTopRightRadius: radius,
          flexShrink: 0,
        }}
      >
        {/* Terminal dots */}
        <div style={{ display: "flex", gap: 8 }}>
          <div
            style={{
              width: 12,
              height: 12,
              borderRadius: 6,
              backgroundColor: "#ff5f57",
            }}
          />
          <div
            style={{
              width: 12,
              height: 12,
              borderRadius: 6,
              backgroundColor: "#ffbd2e",
            }}
          />
          <div
            style={{
              width: 12,
              height: 12,
              borderRadius: 6,
              backgroundColor: "#28c840",
            }}
          />
        </div>
        {title && (
          <span
            style={{
              flex: 1,
              textAlign: "center",
              fontSize: 14,
              color: "#8b949e",
              fontFamily: "'SF Mono', 'Fira Code', monospace",
              fontWeight: 500,
              marginRight: 52,
            }}
          >
            {title}
          </span>
        )}
      </div>
      {/* Terminal content */}
      <div
        style={{
          flex: 1,
          overflow: "hidden",
          backgroundColor: "#0d1117",
          borderBottomLeftRadius: radius,
          borderBottomRightRadius: radius,
          position: "relative",
        }}
      >
        {children}
      </div>
    </div>
  );
};
