import { useState } from "react";
import type { TranslateResult } from "../types/api";

export function TranslatePanel() {
  const [inputPath, setInputPath] = useState("");
  const [outputPath, setOutputPath] = useState("");
  const [sourceLang, setSourceLang] = useState("en");
  const [targetLang, setTargetLang] = useState("");
  const [llmProvider, setProvider] = useState("anthropic");
  const [apiKey, setApiKey] = useState("");
  const [model, setModel] = useState("");
  const [result, setResult] = useState<TranslateResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleTranslate = async () => {
    setError(null);
    setResult(null);
    setLoading(true);

    try {
      const formData = new FormData();
      formData.append("input_path", inputPath);
      if (outputPath) formData.append("output_path", outputPath);
      formData.append("source_lang", sourceLang);
      formData.append("target_lang", targetLang);
      formData.append("provider", llmProvider);
      if (apiKey) formData.append("api_key", apiKey);
      if (model) formData.append("model", model);

      const res = await fetch("/api/v1/translate", {
        method: "POST",
        body: formData,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || res.statusText);
      }

      const data = (await res.json()) as TranslateResult;
      setResult(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: 16 }}>AI Translation</h2>
      <div style={{ display: "flex", flexDirection: "column", gap: 12, maxWidth: 500 }}>
        <label style={labelStyle}>
          Input File
          <input
            type="text"
            value={inputPath}
            onChange={(e) => setInputPath(e.target.value)}
            placeholder="/path/to/input.html"
            style={inputStyle}
          />
        </label>
        <label style={labelStyle}>
          Output File (optional)
          <input
            type="text"
            value={outputPath}
            onChange={(e) => setOutputPath(e.target.value)}
            placeholder="Auto-generated from input"
            style={inputStyle}
          />
        </label>
        <div style={{ display: "flex", gap: 12 }}>
          <label style={{ ...labelStyle, flex: 1 }}>
            Source
            <input
              type="text"
              value={sourceLang}
              onChange={(e) => setSourceLang(e.target.value)}
              style={inputStyle}
            />
          </label>
          <label style={{ ...labelStyle, flex: 1 }}>
            Target
            <input
              type="text"
              value={targetLang}
              onChange={(e) => setTargetLang(e.target.value)}
              placeholder="fr"
              style={inputStyle}
            />
          </label>
        </div>
        <label style={labelStyle}>
          Provider
          <select
            value={llmProvider}
            onChange={(e) => setProvider(e.target.value)}
            style={inputStyle}
          >
            <option value="anthropic">Anthropic (Claude)</option>
            <option value="openai">OpenAI</option>
            <option value="ollama">Ollama (Local)</option>
          </select>
        </label>
        <label style={labelStyle}>
          API Key
          <input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="sk-..."
            style={inputStyle}
          />
        </label>
        <label style={labelStyle}>
          Model (optional)
          <input
            type="text"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="Default model for provider"
            style={inputStyle}
          />
        </label>
        <button onClick={handleTranslate} disabled={loading} style={buttonStyle}>
          {loading ? "Translating..." : "Translate"}
        </button>
        {result && (
          <div style={successStyle}>
            Translated {result.block_count} block(s) to {result.output_path}
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
