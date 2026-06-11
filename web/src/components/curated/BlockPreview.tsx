import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import type { InlineSample } from "./seed";
import "./curated.css";

// BlockPreview — the "kapi reader" curated view.
//
// Given a bundled fixture name (or an inline {name, content}), it boots the
// shared kapi runtime, calls the kit's KapiRuntime.preview(path) API, and
// renders the resulting content model — the translatable blocks, their ids, and
// their source text — in a clean table. This is the single best framework demo:
// "here's how kapi *sees* your file," with no terminal in sight.
//
// preview() returns { ok, format, blocks:[{id,text}], total, bytes }. The view
// surfaces the detected format + counts and renders inline-span markers (the
// Unicode Private Use Area code points kapi uses for inline markup) as small
// visible chips so the reader can see *where* inline formatting sat without the
// invisible code points themselves.

export interface BlockPreviewProps {
  /**
   * The sample to read: a bundled fixture name (e.g. "messages.json", see
   * fixtureNames from @neokapi/kapi-playground) or an inline {name, content}.
   */
  sample: string | InlineSample;
  /**
   * Optional title shown in the card header. Defaults to the file name.
   */
  title?: string;
  /**
   * Optional caption shown below the title (e.g. "How kapi parses an Android
   * strings.xml file").
   */
  caption?: string;
}

// Lazy chunk: the boot helper, seed helpers, and the inline-span renderer all
// live here so a docs page that never mounts a BlockPreview ships zero wasm.
const LazyBlockPreview = React.lazy(async () => {
  const { useCuratedRuntime } = await import("./useCuratedRuntime");
  const { ensureSample, resolveInCwd } = await import("./seed");
  const { CodeView } = await import("@neokapi/ui-primitives/preview");
  type PreviewResult = import("./useCuratedRuntime").PreviewResult;
  type KapiRuntime = import("./useCuratedRuntime").KapiRuntime;

  // Read a file's bytes and decode as UTF-8; flag binary content (NUL byte or a
  // failed strict decode) so we show "download to view" rather than garbage.
  function readRaw(
    rt: KapiRuntime,
    path: string,
  ): { text: string; binary: boolean; error?: string } {
    let bytes: Uint8Array;
    try {
      bytes = rt.vol.readFile(path);
    } catch (e) {
      return { text: "", binary: false, error: e instanceof Error ? e.message : String(e) };
    }
    if (bytes.includes(0)) return { text: "", binary: true };
    try {
      return { text: new TextDecoder("utf-8", { fatal: true }).decode(bytes), binary: false };
    } catch {
      return { text: "", binary: true };
    }
  }

  // Render coded text: PUA markers (U+E000–U+F8FF) that kapi uses for inline
  // spans become little chips; everything else is plain text. We chunk runs of
  // non-marker text so React keys stay stable.
  function renderCoded(text: string): React.ReactNode {
    const out: React.ReactNode[] = [];
    let buf = "";
    let key = 0;
    const flush = () => {
      if (buf) {
        out.push(<span key={`t${key++}`}>{buf}</span>);
        buf = "";
      }
    };
    for (const ch of text) {
      const cp = ch.codePointAt(0) ?? 0;
      if (cp >= 0xe000 && cp <= 0xf8ff) {
        flush();
        out.push(
          <span
            key={`s${key++}`}
            className="kapi-cur-span"
            title={`inline span (U+${cp.toString(16).toUpperCase()})`}
          >
            {"</>"}
          </span>,
        );
      } else {
        buf += ch;
      }
    }
    flush();
    return out;
  }

  function BlockPreviewInner({ sample, title, caption }: BlockPreviewProps): React.ReactElement {
    const { runtime, error, cold } = useCuratedRuntime();
    const [data, setData] = React.useState<PreviewResult | null>(null);
    const [resolvedPath, setResolvedPath] = React.useState<string>("");
    const [seedError, setSeedError] = React.useState<string>("");
    const fileName = typeof sample === "string" ? sample : sample.name;
    const heading = title ?? fileName;

    React.useEffect(() => {
      if (!runtime) return;
      let cancelled = false;
      try {
        const path = ensureSample(runtime, sample);
        const abs = resolveInCwd(runtime, path);
        setResolvedPath(abs);
        runtime.preview(abs).then((r) => {
          if (!cancelled) setData(r);
        });
      } catch (e) {
        setSeedError(e instanceof Error ? e.message : String(e));
      }
      return () => {
        cancelled = true;
      };
      // sample is stable per usage; re-running on runtime is enough.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [runtime]);

    // When the engine can't parse the file (no reader for e.g. a .kapi YAML
    // recipe), don't dead-end — read the raw bytes and show highlighted text.
    const unparseable = data != null && !data.ok;
    const raw = unparseable && runtime ? readRaw(runtime, resolvedPath) : null;
    const rawExt = fileName.includes(".")
      ? "." + fileName.slice(fileName.lastIndexOf(".") + 1).toLowerCase()
      : fileName;

    return (
      <div className="kapi-cur">
        <div className="kapi-cur-card">
          <div className="kapi-cur-head">
            <span className="kapi-cur-title">{heading}</span>
            {data?.ok && data.format && <span className="kapi-cur-badge">{data.format}</span>}
          </div>
          <div className="kapi-cur-body">
            {caption && <p className="kapi-cur-meta">{caption}</p>}

            {(error || seedError) && (
              <p className="kapi-cur-error">Could not read the file: {error || seedError}</p>
            )}

            {!error && !seedError && !data && (
              <div className="kapi-cur-loading">
                <span className="kapi-cur-spinner" aria-hidden="true" />
                <span>
                  {cold
                    ? "Starting the kapi reader for the first time…"
                    : "Reading with the kapi parser…"}
                </span>
              </div>
            )}

            {unparseable && (
              <>
                <p className="kapi-cur-meta">
                  kapi has no reader for <code>{rawExt}</code> — showing raw text.
                </p>
                {raw && raw.error && (
                  <p className="kapi-cur-error">Could not read the file: {raw.error}</p>
                )}
                {raw && !raw.error && raw.binary && (
                  <p className="kapi-cur-meta">Binary file — download to view.</p>
                )}
                {raw && !raw.error && !raw.binary && (
                  <CodeView
                    className="kapi-cur-raw"
                    text={raw.text}
                    filename={fileName}
                    lineNumbers={false}
                  />
                )}
              </>
            )}

            {data?.ok && (
              <>
                <p className="kapi-cur-meta">
                  {data.total} block{data.total === 1 ? "" : "s"} · {data.bytes} bytes · parsed as{" "}
                  <code>{data.format}</code>
                </p>
                {data.blocks && data.blocks.length > 0 ? (
                  <table className="kapi-cur-table">
                    <thead>
                      <tr>
                        <th className="kapi-cur-num">#</th>
                        <th>id</th>
                        <th>source text</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.blocks.map((b, i) => (
                        <tr key={i}>
                          <td className="kapi-cur-num">{i + 1}</td>
                          <td>
                            <code className="kapi-cur-id">{b.id}</code>
                          </td>
                          <td>{renderCoded(b.text)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                ) : (
                  <p className="kapi-cur-meta">No translatable blocks found.</p>
                )}
                {data.total !== undefined && data.blocks && data.total > data.blocks.length && (
                  <p className="kapi-cur-meta">
                    … showing the first {data.blocks.length} of {data.total} blocks.
                  </p>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    );
  }

  return { default: BlockPreviewInner };
});

/**
 * BlockPreview — render a sample file as kapi's content model (blocks /
 * segments / inline spans). Lazy + client-only: a docs page ships zero wasm
 * until a BlockPreview first mounts in the browser.
 */
export default function BlockPreview(props: BlockPreviewProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<div className="kapi-cur" />}>
      {() => (
        <Suspense fallback={<div className="kapi-cur" />}>
          <LazyBlockPreview {...props} />
        </Suspense>
      )}
    </BrowserOnly>
  );
}
