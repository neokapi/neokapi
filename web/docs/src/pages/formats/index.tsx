import { useMemo } from "react";
import Layout from "@theme/Layout";
import { Play } from "lucide-react";
import { formats } from "@neokapi/reference-data";
// SSR-clean event bus only (no xterm/wasm) — the shared modal (mounted in
// Root.tsx) code-splits the heavy runtime in when first opened.
import { openKapi } from "@neokapi/kapi-playground/store";
import { firstLine } from "@site/src/components/reference/Markdown";
import ReferenceGrid from "@site/src/components/reference/ReferenceGrid";

export default function Formats() {
  // Sort by display name and fall back to the first overview line when an
  // entry has no standalone description, so cards always show context.
  const entries = useMemo(() => {
    return [...formats.entries]
      .map((e) => ({
        ...e,
        description: e.description || firstLine(e.doc?.overview),
      }))
      .sort((a, b) => a.displayName.localeCompare(b.displayName));
  }, []);

  return (
    <Layout
      title="Format Reference"
      description="Interactive reference for every neokapi data format — built-in and Okapi bridge — with live, configurable parameters."
    >
      <main className="container margin-vert--lg">
        <h1>Format Reference</h1>
        <p>
          Every data format neokapi can read and write, from the built-in engine and the Okapi
          bridge plugin. Select a format to read its documentation and configure its parameters live
          — the form mirrors the editor used in Kapi Desktop, and the YAML output is ready to drop
          into a project recipe. Each format has a shareable link.
        </p>
        <p>
          <button
            type="button"
            className="button button--primary"
            onClick={() => openKapi({ cmd: "kapi formats list", autoRun: true })}
          >
            <Play size={16} aria-hidden="true" fill="currentColor" style={{ marginRight: 6 }} />
            Try it live
          </button>{" "}
          &mdash; list the registered formats from the real <code>kapi</code> binary, in an
          in-browser terminal.
        </p>

        <ReferenceGrid entries={entries} kind="format" />
      </main>
    </Layout>
  );
}
