import React from "react";
import type { Branding } from "../schema";

interface BrandedFrameProps {
  title?: string;
  branding: Branding;
  children: React.ReactNode;
}

export const BrandedFrame: React.FC<BrandedFrameProps> = ({ title, branding, children }) => {
  const radius = branding.cornerRadius;

  return (
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        backgroundColor: branding.backgroundColor,
        padding: 32,
        boxSizing: "border-box",
      }}
    >
      {/* Title bar */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "10px 16px",
          backgroundColor: "#1e1e2e",
          borderTopLeftRadius: radius,
          borderTopRightRadius: radius,
          flexShrink: 0,
        }}
      >
        {/* Window buttons */}
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
              color: "#a0a0b0",
              fontFamily: "Inter, system-ui, sans-serif",
              fontWeight: 500,
              marginRight: 52, // Balance the dots
            }}
          >
            {title}
          </span>
        )}
      </div>
      {/* Content area */}
      <div
        style={{
          flex: 1,
          overflow: "hidden",
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
