import React, { Suspense } from "react";
import Link from "@docusaurus/Link";
import LabLesson from "@site/src/components/Lab/LabLesson";
import LabLaunch from "@site/src/components/Lab/LabLaunch";
const Evidence = React.lazy(() => import("@site/src/components/Lab/KapiLessonContextEvidence"));
export default function ContextAndReuseLesson(): React.ReactElement {
  return (
    <LabLesson lessonId="context-and-reuse">
      <h2>Predict what applies</h2>
      <p>
        Harbor Help explains video appointments to different audiences. Predict which parts of the
        guidance should change for a child and an adult, and which service facts should remain
        shared.
      </p>
      <ol>
        <li>Select an audience to compare with the child case.</li>
        <li>
          Inspect the coordinates and voice guides. Find an audience-specific instruction and a
          shared constraint.
        </li>
        <li>
          Open each full context answer. Locate the terms and the provenance of the shared rule.
        </li>
      </ol>
      <LabLaunch
        label="Open context evidence"
        description="Load recorded context answers. This experiment needs no engine download or AI account."
      >
        <Suspense fallback={<p role="status">Loading context evidence…</p>}>
          <Evidence />
        </Suspense>
      </LabLaunch>
      <h2>Connect context to reuse</h2>
      <p>
        The previous lesson reused supplied content-memory entries. This fixture shows how kapi
        resolves guidance and terms at a content location. Content-memory retrieval and a live graph
        traversal fall outside it.
      </p>
      <p>
        A native context graph connects content with applicable guidance and recorded decisions.
        Resolving those relationships needs no model. Generation and semantic checks can use the
        resulting context in a later step.
      </p>
      <h2>Transfer</h2>
      <p>
        Reproduce the fixture locally using the instructions inside the evidence view. Change one
        audience instruction in its context files, rerun the fixture and compare the recorded
        guides. Use <Link to="/kapi/context">Context</Link> to trace the native resolution path.
      </p>
    </LabLesson>
  );
}
