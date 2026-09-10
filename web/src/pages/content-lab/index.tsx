import React from "react";
import Layout from "@theme/Layout";
import { ContentLab } from "./_ContentLab";
import evidence from "./_evidence.json";
import type { AudienceEvidence } from "./_types";

export default function ContentLabPage(): React.ReactElement {
  return (
    <Layout
      title="Content context lab"
      description="Compare recorded audience checks, inspect shared constraints and see what still needs factual review."
    >
      <ContentLab evidence={evidence as AudienceEvidence} />
    </Layout>
  );
}
