import { useFormats } from "../hooks/useApi";

export function FormatList() {
  const { formats, loading, error } = useFormats();

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading formats...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

  return (
    <div>
      <h2 style={{ marginBottom: 16 }}>Registered Formats</h2>
      <p style={{ color: "var(--text-secondary)", marginBottom: 16 }}>
        {formats.length} format(s) available
      </p>
      <table
        style={{
          width: "100%",
          borderCollapse: "collapse",
        }}
      >
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
              <td style={tdStyle}>
                <Badge ok={f.has_reader} />
              </td>
              <td style={tdStyle}>
                <Badge ok={f.has_writer} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
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
