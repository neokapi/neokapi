import { usePlugins } from "../hooks/useApi";

export function PluginPage() {
  const { plugins, pluginDir, loading, error } = usePlugins();

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading plugins...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

  return (
    <div>
      <h2 style={{ marginBottom: 8 }}>Plugins</h2>
      <p style={{ color: "var(--text-secondary)", marginBottom: 16, fontSize: 13 }}>
        Plugin directory: <code style={{ fontSize: 12 }}>{pluginDir || "(not configured)"}</code>
      </p>

      {plugins.length === 0 ? (
        <div
          data-testid="plugins-empty"
          style={{
            padding: "24px 16px",
            backgroundColor: "var(--bg-secondary)",
            borderRadius: 8,
            border: "1px solid var(--border)",
            textAlign: "center",
            color: "var(--text-secondary)",
          }}
        >
          <p style={{ marginBottom: 8 }}>No plugins loaded.</p>
          <p style={{ fontSize: 13 }}>
            Place plugin binaries or bridge descriptors in the plugin directory to extend
            available formats and tools.
          </p>
        </div>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid var(--border)" }}>
              <th style={thStyle}>Name</th>
              <th style={thStyle}>Type</th>
              <th style={thStyle}>Formats</th>
            </tr>
          </thead>
          <tbody>
            {plugins.map((p) => (
              <tr key={p.name} style={{ borderBottom: "1px solid var(--border)" }}>
                <td style={tdStyle}>{p.name}</td>
                <td style={tdStyle}>
                  <span
                    style={{
                      display: "inline-block",
                      padding: "2px 8px",
                      borderRadius: 4,
                      fontSize: 12,
                      backgroundColor: "rgba(96,165,250,0.15)",
                      color: "var(--accent)",
                    }}
                  >
                    {p.type}
                  </span>
                </td>
                <td style={tdStyle}>{p.formats.length > 0 ? p.formats.join(", ") : "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
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
