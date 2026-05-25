import React, { useRef } from "react";
import { Upload } from "lucide-react";
import { SAMPLES } from "./samples";
import styles from "./styles.module.css";

export interface FileSourceValue {
  filename: string;
  label: string;
  /** Best-effort UTF-8 text, for display and as the source for text samples. */
  content: string;
  /** Raw bytes — set for uploads. When present this is what the engine reads,
   *  so binary formats (.docx, .xlsx, …) survive intact rather than being
   *  corrupted by a text round-trip. Absent for the bundled text samples. */
  bytes?: Uint8Array;
}

const dec = new TextDecoder();

interface FileSourceProps {
  value: FileSourceValue | null;
  onChange: (v: FileSourceValue) => void;
  /** Restrict the offered samples to these ids (default: all). */
  sampleIds?: string[];
}

// FileSource lets a learner pick a bundled sample or upload their own file.
// Uploads are read as raw bytes (not text) so binary formats such as .docx
// survive intact; the explorer writes those bytes straight into the engine.
export default function FileSource({
  value,
  onChange,
  sampleIds,
}: FileSourceProps): React.ReactElement {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const samples = sampleIds ? SAMPLES.filter((s) => sampleIds.includes(s.id)) : SAMPLES;

  function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      const bytes = new Uint8Array(reader.result as ArrayBuffer);
      onChange({ filename: file.name, label: file.name, bytes, content: dec.decode(bytes) });
    };
    reader.readAsArrayBuffer(file);
    e.target.value = "";
  }

  const activeBlurb = samples.find((s) => s.label === value?.label)?.blurb;

  return (
    <div className={styles.fileSource}>
      <div className={styles.fileChips}>
        {samples.map((s) => (
          <button
            key={s.id}
            className={`${styles.fileChip} ${value?.label === s.label ? styles.fileChipActive : ""}`}
            onClick={() => onChange({ filename: s.filename, content: s.content, label: s.label })}
            title={s.blurb}
          >
            {s.label}
          </button>
        ))}
        <button
          className={`${styles.fileChip} ${styles.uploadChip}`}
          onClick={() => fileInputRef.current?.click()}
          title="Upload your own file"
        >
          <Upload size={13} /> Upload…
        </button>
        <input ref={fileInputRef} type="file" onChange={handleUpload} style={{ display: "none" }} />
      </div>
      {activeBlurb && <p className={styles.sampleBlurb}>{activeBlurb}</p>}
    </div>
  );
}
