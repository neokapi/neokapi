import { useState } from "react";
import type { ProjectInfo } from "../types/api";

interface ProjectDashboardProps {
  projects: ProjectInfo[];
  onCreateProject: (name: string, sourceLang: string, targetLangs: string[]) => void;
  onOpenProject: (project: ProjectInfo) => void;
  onOpenKaz: () => void;
}

export function ProjectDashboard({
  projects,
  onCreateProject,
  onOpenProject,
  onOpenKaz,
}: ProjectDashboardProps) {
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState("");
  const [sourceLang, setSourceLang] = useState("en");
  const [targetLangs, setTargetLangs] = useState("fr");

  const handleCreate = () => {
    if (!name.trim()) return;
    const langs = targetLangs.split(",").map((l) => l.trim()).filter(Boolean);
    if (langs.length === 0) return;
    onCreateProject(name.trim(), sourceLang.trim(), langs);
    setShowCreate(false);
    setName("");
    setTargetLangs("fr");
  };

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 24 }}>
        <h2 style={{ margin: 0 }}>Translation Projects</h2>
        <div style={{ display: "flex", gap: 8 }}>
          <button onClick={onOpenKaz} style={secondaryBtnStyle} data-testid="open-kaz-btn">
            Open a Project
          </button>
          <button onClick={() => setShowCreate(true)} style={btnStyle} data-testid="new-project-btn">
            New Project
          </button>
        </div>
      </div>

      {showCreate && (
        <div style={dialogStyle} data-testid="create-project-dialog">
          <h3 style={{ marginTop: 0, marginBottom: 16 }}>Create Translation Project</h3>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <label style={labelStyle}>
              Project Name
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="My Translation Project"
                style={inputStyle}
                data-testid="project-name-input"
                autoFocus
              />
            </label>
            <div style={{ display: "flex", gap: 12 }}>
              <label style={{ ...labelStyle, flex: 1 }}>
                Source Language
                <input
                  type="text"
                  value={sourceLang}
                  onChange={(e) => setSourceLang(e.target.value)}
                  style={inputStyle}
                  data-testid="source-lang-input"
                />
              </label>
              <label style={{ ...labelStyle, flex: 1 }}>
                Target Languages
                <input
                  type="text"
                  value={targetLangs}
                  onChange={(e) => setTargetLangs(e.target.value)}
                  placeholder="fr, de, ja"
                  style={inputStyle}
                  data-testid="target-langs-input"
                />
              </label>
            </div>
            <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
              <button onClick={() => setShowCreate(false)} style={secondaryBtnStyle}>
                Cancel
              </button>
              <button onClick={handleCreate} style={btnStyle} data-testid="create-project-submit">
                Create
              </button>
            </div>
          </div>
        </div>
      )}

      {projects.length === 0 && !showCreate && (
        <div style={emptyStyle} data-testid="empty-projects">
          <div style={{ fontSize: 48, marginBottom: 16, opacity: 0.3 }}>&#128194;</div>
          <p style={{ color: "var(--text-secondary)", margin: 0 }}>
            No projects yet. Create a new project or open a .kaz package.
          </p>
        </div>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))", gap: 16 }}>
        {projects.map((p) => (
          <div
            key={p.id}
            onClick={() => onOpenProject(p)}
            style={cardStyle}
            data-testid={`project-card-${p.id}`}
          >
            <h3 style={{ margin: "0 0 8px 0", fontSize: 16 }}>{p.name}</h3>
            <div style={{ fontSize: 13, color: "var(--text-secondary)", marginBottom: 8 }}>
              {p.source_locale} &#8594; {p.target_locales.join(", ")}
            </div>
            <div style={{ fontSize: 12, color: "var(--text-secondary)" }}>
              {p.items.length} file{p.items.length !== 1 ? "s" : ""}
              {p.path && <span> &middot; {p.path.split("/").pop()}</span>}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

const btnStyle: React.CSSProperties = {
  padding: "8px 16px",
  backgroundColor: "var(--accent)",
  color: "#fff",
  border: "none",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
  fontWeight: 600,
};

const secondaryBtnStyle: React.CSSProperties = {
  padding: "8px 16px",
  backgroundColor: "var(--bg-tertiary)",
  color: "var(--text-primary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
};

const dialogStyle: React.CSSProperties = {
  backgroundColor: "var(--bg-secondary)",
  border: "1px solid var(--border)",
  borderRadius: 8,
  padding: 24,
  marginBottom: 24,
};

const emptyStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: 48,
  backgroundColor: "var(--bg-secondary)",
  borderRadius: 8,
  border: "1px dashed var(--border)",
};

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

const cardStyle: React.CSSProperties = {
  backgroundColor: "var(--bg-secondary)",
  border: "1px solid var(--border)",
  borderRadius: 8,
  padding: 16,
  cursor: "pointer",
  transition: "border-color 0.15s ease",
};
