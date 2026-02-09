import { useEffect, useRef } from "react";
import { Events } from "@wailsio/runtime";
import type { ProjectInfo } from "../types/api";

interface ProjectViewProps {
  project: ProjectInfo;
  onBack: () => void;
  onOpenFile: (fileName: string) => void;
  onAddFiles: (filePaths: string[]) => void;
  onAddFilesDialog: () => void;
  onRemoveFile: (fileName: string) => void;
  onSave: () => void;
  onOpenTM: () => void;
  onOpenTerms: () => void;
}

export function ProjectView({
  project,
  onBack,
  onOpenFile,
  onAddFiles,
  onAddFilesDialog,
  onRemoveFile,
  onSave,
  onOpenTM,
  onOpenTerms,
}: ProjectViewProps) {
  const dropRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Wails v3: listen for files-dropped custom event emitted from Go backend
    const cancel = Events.On("files-dropped", (event: { data: unknown }) => {
      const paths = event.data as string[];
      if (paths && paths.length > 0) {
        onAddFiles(paths);
      }
    });
    return () => { cancel(); };
  }, [onAddFiles]);

  const totalBlocks = project.items.reduce((sum, f) => sum + f.block_count, 0);
  const totalWords = project.items.reduce((sum, f) => sum + f.word_count, 0);

  const formatIcon = (format: string) => {
    const icons: Record<string, string> = {
      html: "&#127760;",
      xml: "&#128196;",
      json: "&#123;&#125;",
      yaml: "&#128203;",
      plaintext: "&#128221;",
      po: "&#128172;",
      properties: "&#9881;",
      markdown: "&#128195;",
      csv: "&#128202;",
      xliff: "&#128257;",
      xliff2: "&#128257;",
    };
    return icons[format] || "&#128196;";
  };

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 24 }}>
        <button onClick={onBack} style={backBtnStyle} data-testid="back-to-projects">
          &#8592; Projects
        </button>
        <h2 style={{ margin: 0, flex: 1 }}>{project.name}</h2>
        <button onClick={onOpenTerms} style={tmBtnStyle} data-testid="open-terms-btn">
          Terminology
        </button>
        <button onClick={onOpenTM} style={tmBtnStyle} data-testid="open-tm-btn">
          Translation Memory
        </button>
        <button onClick={onSave} style={saveBtnStyle} data-testid="save-project-btn">
          Save
        </button>
      </div>

      <div style={{ display: "flex", gap: 16, marginBottom: 24 }}>
        <div style={statStyle}>
          <div style={{ fontSize: 24, fontWeight: 700 }}>{project.items.length}</div>
          <div style={{ fontSize: 12, color: "var(--text-secondary)" }}>Files</div>
        </div>
        <div style={statStyle}>
          <div style={{ fontSize: 24, fontWeight: 700 }}>{totalBlocks}</div>
          <div style={{ fontSize: 12, color: "var(--text-secondary)" }}>Blocks</div>
        </div>
        <div style={statStyle}>
          <div style={{ fontSize: 24, fontWeight: 700 }}>{totalWords}</div>
          <div style={{ fontSize: 12, color: "var(--text-secondary)" }}>Words</div>
        </div>
        <div style={statStyle}>
          <div style={{ fontSize: 14, fontWeight: 600 }}>
            {project.source_locale} &#8594; {project.target_locales.join(", ")}
          </div>
          <div style={{ fontSize: 12, color: "var(--text-secondary)" }}>Languages</div>
        </div>
      </div>

      {/* File drop zone */}
      <div
        ref={dropRef}
        style={dropZoneStyle}
        data-testid="file-drop-zone"
        data-file-drop-target
      >
        <span style={{ fontSize: 32, opacity: 0.3 }}>&#128230;</span>
        <span style={{ color: "var(--text-secondary)", fontSize: 13 }}>
          Drag and drop files here to add them to the project
        </span>
        <button
          onClick={onAddFilesDialog}
          style={addFilesBtnStyle}
          data-testid="add-files-btn"
        >
          Add Files
        </button>
      </div>

      {/* File list */}
      {project.items.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <table style={tableStyle}>
            <thead>
              <tr>
                <th style={thStyle}>File</th>
                <th style={thStyle}>Format</th>
                <th style={{ ...thStyle, textAlign: "right" }}>Blocks</th>
                <th style={{ ...thStyle, textAlign: "right" }}>Words</th>
                <th style={{ ...thStyle, width: 80 }}></th>
              </tr>
            </thead>
            <tbody>
              {project.items.map((f) => (
                <tr
                  key={f.name}
                  style={rowStyle}
                  data-testid={`file-row-${f.name}`}
                >
                  <td style={tdStyle}>
                    <button
                      onClick={() => onOpenFile(f.name)}
                      style={fileBtnStyle}
                      data-testid={`open-file-${f.name}`}
                    >
                      <span dangerouslySetInnerHTML={{ __html: formatIcon(f.format) }} />
                      {" "}{f.name}
                    </button>
                  </td>
                  <td style={tdStyle}>
                    <span style={badgeStyle}>{f.format}</span>
                  </td>
                  <td style={{ ...tdStyle, textAlign: "right" }}>{f.block_count}</td>
                  <td style={{ ...tdStyle, textAlign: "right" }}>{f.word_count}</td>
                  <td style={{ ...tdStyle, textAlign: "right" }}>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onRemoveFile(f.name);
                      }}
                      style={removeBtnStyle}
                      data-testid={`remove-file-${f.name}`}
                    >
                      &#10005;
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

const backBtnStyle: React.CSSProperties = {
  padding: "6px 12px",
  backgroundColor: "var(--bg-tertiary)",
  color: "var(--text-primary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
};

const tmBtnStyle: React.CSSProperties = {
  padding: "8px 16px",
  backgroundColor: "var(--bg-tertiary)",
  color: "var(--text-primary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
  fontWeight: 600,
};

const saveBtnStyle: React.CSSProperties = {
  padding: "8px 16px",
  backgroundColor: "var(--accent)",
  color: "#fff",
  border: "none",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
  fontWeight: 600,
};

const statStyle: React.CSSProperties = {
  backgroundColor: "var(--bg-secondary)",
  border: "1px solid var(--border)",
  borderRadius: 8,
  padding: "12px 20px",
  textAlign: "center",
  flex: 1,
};

const dropZoneStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: 8,
  padding: 32,
  border: "2px dashed var(--border)",
  borderRadius: 8,
  backgroundColor: "var(--bg-secondary)",
  cursor: "default",
};

const addFilesBtnStyle: React.CSSProperties = {
  marginTop: 8,
  padding: "6px 16px",
  backgroundColor: "var(--accent)",
  color: "#fff",
  border: "none",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
  fontWeight: 600,
};

const tableStyle: React.CSSProperties = {
  width: "100%",
  borderCollapse: "collapse",
  backgroundColor: "var(--bg-secondary)",
  borderRadius: 8,
  overflow: "hidden",
};

const thStyle: React.CSSProperties = {
  padding: "10px 16px",
  textAlign: "left",
  fontSize: 12,
  fontWeight: 600,
  color: "var(--text-secondary)",
  borderBottom: "1px solid var(--border)",
  textTransform: "uppercase",
  letterSpacing: 0.5,
};

const tdStyle: React.CSSProperties = {
  padding: "10px 16px",
  fontSize: 14,
  borderBottom: "1px solid var(--border)",
};

const rowStyle: React.CSSProperties = {
  transition: "background-color 0.1s ease",
};

const fileBtnStyle: React.CSSProperties = {
  background: "none",
  border: "none",
  color: "var(--accent)",
  cursor: "pointer",
  fontSize: 14,
  padding: 0,
  textDecoration: "none",
};

const badgeStyle: React.CSSProperties = {
  padding: "2px 8px",
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 4,
  fontSize: 12,
  color: "var(--text-secondary)",
};

const removeBtnStyle: React.CSSProperties = {
  background: "none",
  border: "none",
  color: "var(--text-secondary)",
  cursor: "pointer",
  fontSize: 14,
  padding: "4px 8px",
  borderRadius: 4,
};
