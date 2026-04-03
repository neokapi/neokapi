import type { SpanInfo } from "../../types/api";
import { codedTextToHtml } from "./tagSemantics";

interface InlinePreviewProps {
  codedText: string;
  spans: SpanInfo[];
}

/**
 * Live preview strip showing the formatted target text.
 * Uses whitelist-based HTML generation for safe rendering.
 */
export function InlinePreview({ codedText, spans }: InlinePreviewProps) {
  if (!codedText) return null;

  const html = codedTextToHtml(codedText, spans);

  return (
    <div style={containerStyle}>
      <span style={labelStyle}>Preview:</span>
      <span style={contentStyle} dangerouslySetInnerHTML={{ __html: html }} />
    </div>
  );
}

const containerStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "baseline",
  gap: 6,
  padding: "3px 8px",
  marginTop: 4,
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 3,
  border: "1px solid var(--border)",
  minHeight: 22,
};

const labelStyle: React.CSSProperties = {
  fontSize: 10,
  fontWeight: 600,
  color: "var(--text-secondary)",
  flexShrink: 0,
  textTransform: "uppercase",
  letterSpacing: 0.5,
};

const contentStyle: React.CSSProperties = {
  fontSize: 13,
  lineHeight: 1.4,
  color: "var(--text-primary)",
  wordBreak: "break-word",
};
