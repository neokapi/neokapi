import { useFlows } from "../hooks/useApi";

export function FlowList() {
  const { flows, loading, error } = useFlows();

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading flows...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

  return (
    <div>
      <h2 style={{ marginBottom: 16 }}>Available Flows</h2>
      <p style={{ color: "var(--text-secondary)", marginBottom: 16 }}>
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
    </div>
  );
}
