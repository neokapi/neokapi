"use client";

const userName = "Sam";

const notes = [
  { id: 1, title: "Q3 launch plan", excerpt: "Rollout timeline, owners, and the comms checklist." },
  { id: 2, title: "Design review", excerpt: "Tighten the empty states and ship the new icon set." },
  { id: 3, title: "Reading list", excerpt: "Three papers on incremental computation to get through." },
];

const wrap: React.CSSProperties = {
  minHeight: "100vh",
  margin: 0,
  fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, sans-serif",
  background: "#f6f7fb",
  color: "#1a1f2e",
  padding: "56px 64px",
};

export default function Home() {
  return (
    <main style={wrap}>
      <header style={{ marginBottom: 32 }}>
        <h1 style={{ fontSize: 40, fontWeight: 700, margin: "0 0 6px" }}>Your notes</h1>
        <p style={{ fontSize: 18, color: "#5b647a", margin: 0 }}>Welcome back, {userName}.</p>
      </header>

      <section style={{ display: "flex", gap: 12, marginBottom: 28 }}>
        <button
          onClick={() => alert("New note")}
          style={{ background: "#4f46e5", color: "#fff", border: 0, borderRadius: 10, padding: "12px 20px", fontSize: 16, fontWeight: 600, cursor: "pointer" }}
        >
          New note
        </button>
        <div style={{ flex: 1, background: "#fff", border: "1px solid #e6e8f0", borderRadius: 10, padding: "12px 16px", color: "#9aa3b8", fontSize: 16 }}>
          Search your notes
        </div>
      </section>

      <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 14 }}>
        {notes.map((n) => (
          <li key={n.id} style={{ background: "#fff", border: "1px solid #e6e8f0", borderRadius: 14, padding: "20px 24px" }}>
            <div style={{ fontSize: 18, fontWeight: 600 }}>{n.title}</div>
            <div style={{ fontSize: 15, color: "#5b647a", marginTop: 4 }}>{n.excerpt}</div>
          </li>
        ))}
      </ul>

      <footer style={{ marginTop: 40, color: "#9aa3b8", fontSize: 14 }}>
        Lumen Notes keeps your ideas in sync across every device.
      </footer>
    </main>
  );
}
