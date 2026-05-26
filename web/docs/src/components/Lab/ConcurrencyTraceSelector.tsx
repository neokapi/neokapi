import React, { useRef } from "react";
import styles from "./ConcurrencyExplorer.module.css";

interface TraceSelectorProps {
  traces: { name: string; description: string; path: string }[];
  selectedTrace: string;
  onSelect: (path: string) => void;
  onLoadFile: (trace: unknown, fileName: string) => void;
  loadedFileName: string | null;
}

export default function TraceSelector({
  traces,
  selectedTrace,
  onSelect,
  onLoadFile,
  loadedFileName,
}: TraceSelectorProps): React.ReactElement {
  const fileInputRef = useRef<HTMLInputElement>(null);

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const data = JSON.parse(reader.result as string);
        onLoadFile(data, file.name);
      } catch {
        alert("Invalid JSON file");
      }
    };
    reader.readAsText(file);
    // Reset so the same file can be re-selected.
    e.target.value = "";
  }

  return (
    <div className={styles.traceSelector}>
      {traces.map((trace) => (
        <button
          key={trace.path}
          className={`${styles.traceButton} ${selectedTrace === trace.path && !loadedFileName ? styles.traceButtonActive : ""}`}
          onClick={() => onSelect(trace.path)}
          title={trace.description}
        >
          {trace.name}
        </button>
      ))}
      <button
        className={`${styles.traceButton} ${loadedFileName ? styles.traceButtonActive : ""}`}
        onClick={() => fileInputRef.current?.click()}
        title="Load a trace JSON file generated with --trace"
      >
        {loadedFileName ?? "Load file\u2026"}
      </button>
      <input
        ref={fileInputRef}
        type="file"
        accept=".json"
        onChange={handleFileChange}
        style={{ display: "none" }}
      />
    </div>
  );
}
