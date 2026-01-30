import { useFormats, useTools, useFlows } from "../hooks/useApi";

export function InfoPage() {
  const { formats, loading: fmtLoading, error: fmtError } = useFormats();
  const { tools, loading: toolLoading, error: toolError } = useTools();
  const { flows, loading: flowLoading, error: flowError } = useFlows();

  const loading = fmtLoading || toolLoading || flowLoading;
  const error = fmtError || toolError || flowError;

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 32 }}>
      {/* Formats section */}
      <section>
        <h2 style={{ marginBottom: 8 }}>Formats</h2>
        <p style={{ color: "var(--text-secondary)", marginBottom: 12 }}>
          {formats.length} format(s) registered
        </p>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid var(--border)" }}>
              <th style={thStyle}>Format</th>
              <th style={thStyle}>Read</th>
              <th style={thStyle}>Write</th>
            </tr>
          </thead>
          <tbody>
            {formats.map((f) => (
              <tr key={f.name} style={{ borderBottom: "1px solid var(--border)" }}>
                <td style={tdStyle}>{f.name}</td>
                <td style={tdStyle}><Badge ok={f.has_reader} /></td>
                <td style={tdStyle}><Badge ok={f.has_writer} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {/* Tools section */}
      <section>
        <h2 style={{ marginBottom: 8 }}>Tools</h2>
        <p style={{ color: "var(--text-secondary)", marginBottom: 12 }}>
          {tools.length} tool(s) available
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {tools.map((t) => (
            <div
              key={t.name}
              style={{
                padding: "12px 16px",
                backgroundColor: "var(--bg-secondary)",
                borderRadius: 8,
                border: "1px solid var(--border)",
              }}
            >
              <div style={{ fontWeight: 600, marginBottom: 4 }}>{t.name}</div>
              <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
                {t.description}
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Flows section */}
      <section>
        <h2 style={{ marginBottom: 8 }}>Flows</h2>
        <p style={{ color: "var(--text-secondary)", marginBottom: 12 }}>
          {flows.length} flow(s) available
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {flows.map((f) => (
            <div
              key={f.name}
              style={{
                padding: "12px 16px",
                backgroundColor: "var(--bg-secondary)",
                borderRadius: 8,
                border: "1px solid var(--border)",
              }}
            >
              <div style={{ fontWeight: 600, marginBottom: 4 }}>{f.name}</div>
              <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
                {f.description}
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

function Badge({ ok }: { ok: boolean }) {
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 4,
        fontSize: 12,
        backgroundColor: ok ? "rgba(34,197,94,0.15)" : "rgba(239,68,68,0.15)",
        color: ok ? "var(--success)" : "var(--error)",
      }}
    >
      {ok ? "Yes" : "No"}
    </span>
  );
}

const thStyle: React.CSSProperties = {
  textAlign: "left",
  padding: "10px 12px",
  color: "var(--text-secondary)",
  fontSize: 12,
  textTransform: "uppercase",
  letterSpacing: 0.5,
};

const tdStyle: React.CSSProperties = {
  padding: "10px 12px",
  fontSize: 14,
};
