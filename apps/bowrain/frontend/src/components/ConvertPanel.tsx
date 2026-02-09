import { useState } from "react";
import type { ConvertResult } from "../types/api";
import { LocaleSelect } from "./LocaleSelect";

export function ConvertPanel() {
  const [inputPath, setInputPath] = useState("");
  const [outputPath, setOutputPath] = useState("");
  const [sourceLang, setSourceLang] = useState("en");
  const [targetLang, setTargetLang] = useState("");
  const [result, setResult] = useState<ConvertResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleConvert = async () => {
    setError(null);
    setResult(null);
    setLoading(true);

    try {
      const formData = new FormData();
      formData.append("input_path", inputPath);
      formData.append("output_path", outputPath);
      formData.append("source_lang", sourceLang);
      if (targetLang) formData.append("target_lang", targetLang);

      const res = await fetch("/api/v1/convert", {
        method: "POST",
        body: formData,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || res.statusText);
      }

      const data = (await res.json()) as ConvertResult;
      setResult(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: 16 }}>Convert File</h2>
      <div style={{ display: "flex", flexDirection: "column", gap: 12, maxWidth: 500 }}>
        <label style={labelStyle}>
          Input File
          <input
            type="text"
            value={inputPath}
            onChange={(e) => setInputPath(e.target.value)}
            placeholder="/path/to/input.json"
            style={inputStyle}
          />
        </label>
        <label style={labelStyle}>
          Output File
          <input
            type="text"
            value={outputPath}
            onChange={(e) => setOutputPath(e.target.value)}
            placeholder="/path/to/output.xlf"
            style={inputStyle}
          />
        </label>
        <div style={{ display: "flex", gap: 12 }}>
          <label style={{ ...labelStyle, flex: 1 }}>
            Source Language
            <LocaleSelect
              value={sourceLang}
              onChange={setSourceLang}
              data-testid="convert-source-lang"
            />
          </label>
          <label style={{ ...labelStyle, flex: 1 }}>
            Target Language
            <LocaleSelect
              value={targetLang}
              onChange={setTargetLang}
              data-testid="convert-target-lang"
            />
          </label>
        </div>
        <button onClick={handleConvert} disabled={loading} style={buttonStyle}>
          {loading ? "Converting..." : "Convert"}
        </button>
        {result && (
          <div style={successStyle}>
            Converted {result.part_count} part(s) to {result.output_path}
          </div>
        )}
        {error && <div style={errorStyle}>{error}</div>}
      </div>
    </div>
  );
}

const labelStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 4,
  fontSize: 13,
  color: "var(--text-secondary)",
};

const inputStyle: React.CSSProperties = {
  padding: "8px 12px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  color: "var(--text-primary)",
  fontSize: 14,
  outline: "none",
};

const buttonStyle: React.CSSProperties = {
  padding: "10px 20px",
  backgroundColor: "var(--accent)",
  color: "#fff",
  border: "none",
  borderRadius: 6,
  fontSize: 14,
  cursor: "pointer",
  fontWeight: 600,
};

const successStyle: React.CSSProperties = {
  padding: "10px 12px",
  backgroundColor: "rgba(34,197,94,0.1)",
  border: "1px solid var(--success)",
  borderRadius: 6,
  color: "var(--success)",
  fontSize: 13,
};

const errorStyle: React.CSSProperties = {
  padding: "10px 12px",
  backgroundColor: "rgba(239,68,68,0.1)",
  border: "1px solid var(--error)",
  borderRadius: 6,
  color: "var(--error)",
  fontSize: 13,
};
