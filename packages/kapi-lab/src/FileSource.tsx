import React, { useRef } from "react";
import { Upload } from "lucide-react";
import { SAMPLES } from "./samples";
import styles from "./styles.module.css";

export interface FileSourceValue {
  filename: string;
  content: string;
  label: string;
}

interface FileSourceProps {
  value: FileSourceValue | null;
  onChange: (v: FileSourceValue) => void;
  /** Restrict the offered samples to these ids (default: all). */
  sampleIds?: string[];
}

// FileSource lets a learner pick a bundled sample or upload their own file. It
// reports the raw text + filename; the explorer decides what to do with it.
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
      onChange({ filename: file.name, content: String(reader.result ?? ""), label: file.name });
    };
    reader.readAsText(file);
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
