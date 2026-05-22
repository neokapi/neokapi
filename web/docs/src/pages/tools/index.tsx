import { useMemo } from "react";
import Layout from "@theme/Layout";
import { tools } from "@neokapi/reference-data";
import { firstLine } from "@site/src/components/reference/Markdown";
import ReferenceList from "@site/src/components/reference/ReferenceList";

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
          Processing tools transform content as it streams through a flow — translating,
          validating, analyzing, and converting blocks. Tools are grouped by category below. Expand
          one to read its documentation and configure its parameters live; the YAML output drops
          into a flow step.
        </p>

        <ReferenceList entries={entries} kind="tool" />
      </main>
    </Layout>
  );
}
