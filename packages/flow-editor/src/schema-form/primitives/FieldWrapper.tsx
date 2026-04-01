import { useState } from "react";
import type { ToolDocParam } from "../types";
import { theme } from "../utils";
import { Md } from "./Markdown";
import { PresetDot } from "./PresetDot";

export function FieldWrapper({
  label,
  description,
  compact,
  isModified,
  docParam,
  children,
  vertical: _vertical,
  disabled,
  error,
}: {
  label: string;
  description?: string;
  compact: boolean;
  isModified?: boolean;
  docParam?: ToolDocParam;
  children: React.ReactNode;
  /** When true, label is positioned above the input instead of beside. */
  vertical?: boolean;
  /** Visual disabled state (reduces opacity). */
  disabled?: boolean;
  /** Validation error message from Zod. */
  error?: string;
}) {
  const [showMore, setShowMore] = useState(false);
  // Accept both "description" and "help" field names (okapi-bridge docs use "help")
  const docDesc = docParam?.description || docParam?.help;
  const hasNotes = docParam?.notes && docParam.notes.length > 0;
  const hasDeps = docParam?.dependsOn && docParam.dependsOn.length > 0;
  const hasExtra = hasNotes || hasDeps || docParam?.introducedIn;

  return (
    <div style={{ display: "flex", gap: 16, opacity: disabled ? 0.5 : 1, transition: "opacity 150ms" }}>
      {/* Left: label + control */}
      <div style={{ flex: "1 1 0%", minWidth: 0, display: "flex", flexDirection: "column", gap: 3 }}>
        <div
          style={{
            fontSize: 11,
            color: theme.fgSecondary,
            fontWeight: 500,
          }}
        >
          <PresetDot visible={!!isModified} />
          {label}
        </div>
        {description && !compact && !docDesc && (
          <div style={{ fontSize: 10, color: theme.fgMuted, lineHeight: 1.3, marginBottom: 2 }}>
            <Md text={description} style={{ fontSize: 10, color: theme.fgMuted, lineHeight: "1.3" }} />
          </div>
        )}
        {children}
        {error && (
          <div style={{ fontSize: 10, color: "#ef4444", marginTop: 2, fontWeight: 500 }}>
            {error}
          </div>
        )}
      </div>

      {/* Right: doc description */}
      {docDesc && !compact && (
        <div style={{ flex: "0 0 42%", minWidth: 0, paddingTop: 18 }}>
          <div
            style={{
              fontSize: 10,
              color: theme.fgMuted,
              lineHeight: 1.5,
              display: showMore ? "block" : "-webkit-box",
              WebkitLineClamp: showMore ? undefined : 3,
              WebkitBoxOrient: "vertical" as const,
              overflow: showMore ? "visible" : "hidden",
              overflowWrap: "break-word",
              wordBreak: "break-word",
            }}
          >
            <Md text={docDesc} style={{ fontSize: 10, color: theme.fgMuted, lineHeight: "1.5" }} />
          </div>
          {(hasExtra || (docDesc && docDesc.length > 120)) && (
            <div style={{ textAlign: "right" }}>
              <button
                onClick={() => setShowMore((v) => !v)}
                style={{
                  background: "none",
                  border: "none",
                  padding: 0,
                  marginTop: 3,
                  fontSize: 9,
                  color: theme.ring,
                  fontWeight: 600,
                  cursor: "pointer",
                }}
              >
                {showMore ? "Show less" : "Show more"}
              </button>
            </div>
          )}
          {showMore && hasNotes && (
            <div style={{ marginTop: 6, display: "flex", flexDirection: "column", gap: 3 }}>
              {docParam!.notes!.map((note, i) => (
                <div
                  key={i}
                  style={{
                    fontSize: 9,
                    color: theme.fgMuted,
                    fontStyle: "italic",
                    lineHeight: 1.4,
                    paddingLeft: 6,
                    borderLeft: `2px solid color-mix(in oklch, ${theme.accent} 30%, transparent)`,
                  }}
                >
                  <Md text={note} style={{ fontSize: 9, color: theme.fgMuted }} />
                </div>
              ))}
            </div>
          )}
          {showMore && hasDeps && (
            <div style={{ marginTop: 4, display: "flex", flexWrap: "wrap", gap: 3 }}>
              {docParam!.dependsOn!.map((dep, i) => (
                <span
                  key={i}
                  style={{
                    fontSize: 9,
                    padding: "1px 5px",
                    borderRadius: 3,
                    background: `color-mix(in oklch, ${theme.ring} 8%, transparent)`,
                    color: theme.fgMuted,
                  }}
                >
                  Requires <code style={{ fontWeight: 600 }}>{dep.property}</code> {dep.condition}
                </span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
