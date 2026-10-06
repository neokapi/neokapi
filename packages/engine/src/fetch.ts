// Fetching and instantiating the engine binary, on the page or in the Worker.

/** Download progress for the engine boot, for hosts that render a bar. */
export interface BootProgress {
  /** Bytes received so far. */
  loaded: number;
  /** Total bytes (from Content-Length), or null when the server omits it. */
  total: number | null;
  /** True once the engine is up (terminal event). */
  done?: boolean;
}

/** Wrap a response body with a byte-counting stage that reports progress. */
function countingStream(
  resp: Response,
  report: (p: BootProgress) => void,
): ReadableStream<Uint8Array<ArrayBuffer>> {
  const total = Number(resp.headers.get("content-length")) || null;
  let loaded = 0;
  report({ loaded: 0, total });
  const counter = new TransformStream<Uint8Array<ArrayBuffer>, Uint8Array<ArrayBuffer>>({
    transform(chunk, controller) {
      loaded += chunk.byteLength;
      report({ loaded, total });
      controller.enqueue(chunk);
    },
  });
  return resp.body!.pipeThrough(counter);
}

// Fetch the wasm bytes. Prefer the precompressed `.wasm.gz` (the binary is
// ~90 MB raw, ~20 MB gzipped) and inflate it in the browser via
// DecompressionStream — this is portable and does not depend on the host
// setting Content-Encoding (GitHub Pages / Docusaurus static serving do not).
// Falls back to the raw `.wasm` if the compressed asset or the API is missing.
// Both paths report download progress.
export async function fetchWasmBytes(
  wasmUrl: string,
  report: (p: BootProgress) => void,
): Promise<ArrayBuffer | Response> {
  if (typeof DecompressionStream !== "undefined") {
    try {
      const gzResp = await fetch(`${wasmUrl}.gz`);
      if (gzResp.ok && gzResp.body) {
        const stream = countingStream(gzResp, report).pipeThrough(new DecompressionStream("gzip"));
        return await new Response(stream).arrayBuffer();
      }
    } catch {
      /* fall through to the raw asset */
    }
  }
  const resp = await fetch(wasmUrl);
  if (resp.ok && resp.body) {
    return await new Response(countingStream(resp, report)).arrayBuffer();
  }
  return resp;
}

export async function instantiate(
  source: ArrayBuffer | Response,
  importObject: WebAssembly.Imports,
): Promise<WebAssembly.Instance> {
  if (source instanceof Response) {
    try {
      const r = await WebAssembly.instantiateStreaming(source.clone(), importObject);
      return r.instance;
    } catch {
      const buf = await source.arrayBuffer();
      const r = await WebAssembly.instantiate(buf, importObject);
      return r.instance;
    }
  }
  const r = await WebAssembly.instantiate(source, importObject);
  return r.instance;
}

/** The URL of a file served in the same directory as `url`. */
export function sibling(url: string, name: string): string {
  return url.replace(/[^/]*$/, name);
}

// The Go class wasm_exec.js defines, and the fs/process host shims that Go's
// js/wasm runtime reads from globalThis (see syscall/fs_js.go). Deliberately
// NOT declared ambiently: global `fs`/`process` declarations would collide
// with @types/node in consumer programs.
export interface GoInstance {
  importObject: WebAssembly.Imports;
  env: Record<string, string>;
  run(instance: WebAssembly.Instance): Promise<void>;
}
export interface WasmExecHost {
  Go?: new () => GoInstance;
  fs?: unknown;
  process?: { env?: Record<string, string> };
}
