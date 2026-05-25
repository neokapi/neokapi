import React, { useRef, useState } from "react";
import { ChevronDown, ChevronRight, Download, Upload } from "lucide-react";
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

const enc = new TextEncoder();
const dec = new TextDecoder();

interface FileSourceProps {
  value: FileSourceValue | null;
  onChange: (v: FileSourceValue) => void;
  /** Restrict the offered samples to these ids (default: all). */
  sampleIds?: string[];
}

function byteLength(v: FileSourceValue): number {
  return v.bytes ? v.bytes.length : enc.encode(v.content).length;
}

function formatBytes(n: number): string {
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KB`;
}

// A file whose decoded text contains NUL or replacement chars is binary — its
// text view would be garbage, so we offer download instead of a preview.
function looksBinary(v: FileSourceValue): boolean {
  if (!v.bytes) return false;
  // NUL or the Unicode replacement char in the decoded text => binary.
  for (let i = 0; i < v.content.length; i++) {
    const c = v.content.charCodeAt(i);
    if (c === 0 || c === 0xfffd) return true;
  }
  return false;
}

function downloadFile(v: FileSourceValue): void {
  const data = v.bytes ?? enc.encode(v.content);
  // Copy into a fresh ArrayBuffer so the Blob doesn't alias the runtime buffer.
  const blob = new Blob([data.slice()]);
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = v.filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

// FileSource lets a learner pick a bundled sample or upload their own file, and
// — so it's clear what they're working with — peek at the file's contents and
// download it. Uploads are read as raw bytes (not text) so binary formats such
// as .docx survive intact; the explorer writes those bytes into the engine.
export default function FileSource({
  value,
  onChange,
  sampleIds,
}: FileSourceProps): React.ReactElement {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [viewing, setViewing] = useState(false);
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
  const binary = value ? looksBinary(value) : false;

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

      {value && (
        <div className={styles.fileMeta}>
          <button
            className={styles.fileMetaBtn}
            onClick={() => setViewing((v) => !v)}
            disabled={binary}
            title={binary ? "Binary file — download to inspect" : "Show the file contents"}
          >
            {viewing ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
            {binary ? "Binary file" : "View source"}
          </button>
          <span className={styles.fileMetaInfo}>
            {value.filename} · {formatBytes(byteLength(value))}
          </span>
          <button
            className={styles.fileMetaBtn}
            onClick={() => downloadFile(value)}
            title="Download this file"
          >
            <Download size={13} /> Download
          </button>
        </div>
      )}

      {value && viewing && !binary && <pre className={styles.fileView}>{value.content}</pre>}
    </div>
  );
}
