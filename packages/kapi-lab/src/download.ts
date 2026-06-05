// Browser download helpers, shared by the file explorer and output viewer.
// Both copy the bytes into a fresh buffer so the Blob never aliases the wasm
// runtime's memory (which can be reused or detached after the call returns).

const enc = new TextEncoder();

/** Trigger a download of raw bytes under the given filename. */
export function downloadBytes(filename: string, data: Uint8Array): void {
  const copy = new Uint8Array(data.length);
  copy.set(data);
  const url = URL.createObjectURL(new Blob([copy as BlobPart]));
  const a = document.createElement("a");
  a.href = url;
  a.download = basename(filename);
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/** Trigger a download of UTF-8 text under the given filename. */
export function downloadText(filename: string, text: string): void {
  downloadBytes(filename, enc.encode(text));
}

function basename(p: string): string {
  return p.replace(/\/+$/, "").split("/").pop() || p;
}

/** Human-readable byte size, e.g. "812 B", "13.4 KB", "2.1 MB". */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
