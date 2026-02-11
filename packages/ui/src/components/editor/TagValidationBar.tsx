import type { TagValidationResult } from "./tagSemantics";

interface TagValidationBarProps {
  validation: TagValidationResult | null;
}

/**
 * Compact bar displaying tag validation errors and warnings.
 * Red row for errors (missing tags, unpaired tags), yellow for warnings (extra tags).
 */
export function TagValidationBar({ validation }: TagValidationBarProps) {
  if (!validation || (validation.errors.length === 0 && validation.warnings.length === 0)) {
    return null;
  }

  return (
    <div style={containerStyle}>
      {validation.errors.map((err, i) => (
        <div key={`e-${i}`} style={errorRowStyle}>
          <span style={iconStyle}>&#9888;</span>
          {err.message}
        </div>
      ))}
      {validation.warnings.map((warn, i) => (
        <div key={`w-${i}`} style={warningRowStyle}>
          <span style={iconStyle}>&#9432;</span>
          {warn.message}
        </div>
      ))}
    </div>
  );
}

const containerStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 2,
  marginTop: 4,
};

const baseRowStyle: React.CSSProperties = {
  fontSize: 11,
  padding: "2px 8px",
  borderRadius: 3,
  display: "flex",
  alignItems: "center",
  gap: 4,
};

const errorRowStyle: React.CSSProperties = {
  ...baseRowStyle,
  backgroundColor: "rgba(239, 68, 68, 0.1)",
  color: "rgb(220, 38, 38)",
  border: "1px solid rgba(239, 68, 68, 0.25)",
};

const warningRowStyle: React.CSSProperties = {
  ...baseRowStyle,
  backgroundColor: "rgba(234, 179, 8, 0.1)",
  color: "rgb(161, 98, 7)",
  border: "1px solid rgba(234, 179, 8, 0.25)",
};

const iconStyle: React.CSSProperties = {
  fontSize: 12,
  flexShrink: 0,
};
