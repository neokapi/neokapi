import { useMemo } from "react";
import Layout from "@theme/Layout";
import { tools } from "@neokapi/reference-data";
import { firstLine } from "@site/src/components/reference/Markdown";
import ReferenceCount from "@site/src/components/reference/ReferenceCount";
import ReferenceGrid from "@site/src/components/reference/ReferenceGrid";

export default function Tools() {
  // Category grouping happens in ReferenceList; here we only ensure a stable
  // display-name order and a description fallback.
  const entries = useMemo(() => {
    return [...tools.entries]
      .map((e) => ({
        ...e,
        description: e.description || firstLine(e.doc?.overview),
      }))
      .sort((a, b) => a.displayName.localeCompare(b.displayName));
  }, []);

  return (
    <Layout
      title="Tool Reference"
      description="Interactive reference for every neokapi processing tool — built-in and Okapi bridge — grouped by category with live, configurable parameters."
    >
      <main className="container margin-vert--lg">
        <h1>Tool Reference</h1>
        <p>
          Processing tools transform content as it streams through a flow — translating, validating,
          analyzing, and converting blocks. Tools are grouped by category below. Select one to read
          its documentation and configure its parameters live; the YAML output drops into a flow
          step. Each tool has a shareable link.
        </p>
        <div className="alert alert--secondary" role="note">
          <p style={{ marginBottom: "0.5rem" }}>
            <strong>Two sources, one grid.</strong> Each card is tagged by where the tool comes
            from; the <strong>Built-in</strong>/<strong>Okapi bridge</strong> filter narrows by that
            tag:
          </p>
          <ul style={{ marginBottom: "0.5rem" }}>
            <li>
              <strong>Built-in</strong> (<ReferenceCount kind="tool" source="built-in" />) — the
              native processing tools maintained in neokapi, such as <code>word-count</code>,{" "}
              <code>pseudo-translate</code>, and <code>qa</code>.
            </li>
            <li>
              <strong>Okapi bridge</strong> (<ReferenceCount kind="tool" source="okapi" />) — the
              pipeline steps exposed by the optional Okapi bridge plugin, for compatibility with the
              Java <a href="https://okapiframework.org/">Okapi Framework</a>.
            </li>
          </ul>
          <p style={{ marginBottom: 0 }}>
            A handful of tools exist in both sources (a built-in and an Okapi twin share an id, e.g.{" "}
            <code>word-count</code>). To keep their static pages distinct, the Okapi twin's page is
            suffixed with <code>-okapi</code> — <code>/reference/tools/word-count</code> for the
            built-in and <code>/reference/tools/word-count-okapi</code> for the bridge.
          </p>
        </div>

        <ReferenceGrid entries={entries} kind="tool" />
      </main>
    </Layout>
  );
}
