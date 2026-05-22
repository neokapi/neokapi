import { useMemo } from "react";
import Layout from "@theme/Layout";
import { formats } from "@neokapi/reference-data";
import { firstLine } from "@site/src/components/reference/Markdown";
import ReferenceList from "@site/src/components/reference/ReferenceList";

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
          bridge plugin. Expand a format to read its documentation and configure its parameters
          live — the form below mirrors the editor used in Kapi Desktop, and the YAML output is
          ready to drop into a project recipe.
        </p>

        <ReferenceList entries={entries} kind="format" />
      </main>
    </Layout>
  );
}
