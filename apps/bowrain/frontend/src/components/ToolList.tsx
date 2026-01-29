import { useTools } from "../hooks/useApi";

export function ToolList() {
  const { tools, loading, error } = useTools();

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading tools...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

  return (
    <div>
      <h2 style={{ marginBottom: 16 }}>Available Tools</h2>
      <p style={{ color: "var(--text-secondary)", marginBottom: 16 }}>
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
    </div>
  );
}
