import React, { Suspense } from "react";
import Link from "@docusaurus/Link";
import LabLesson from "@site/src/components/Lab/LabLesson";
import LabLaunch from "@site/src/components/Lab/LabLaunch";
const Lifecycle = React.lazy(() => import("@site/src/components/Lab/KapiLessonLifecycle"));
export default function DecisionsAndChangeLesson(): React.ReactElement {
  return (
    <LabLesson lessonId="decisions-and-change">
      <h2>Predict the effect of a decision</h2>
      <p>
        A writer notices that a product name appears as both Quickcast and Quick cast. Predict what
        a context answer should return before the observation, while it is a suggestion, and after a
        person keeps it.
      </p>
      <ol>
        <li>
          Inspect the suggested and established steps. Find the operation id connecting the
          observation to the decision. Compare reporting and failing findings in the recorded check.
        </li>
        <li>
          Compare the source edit with the native run that follows it. Inspect freshness and
          recorded term occurrences. Does the governing rule change? What evidence supports your
          answer?
        </li>
        <li>
          Inspect the reverted step. Identify which terms remain, which restrictions disappear, and
          what the operation log retains.
        </li>
      </ol>
      <LabLaunch
        label="Open decision recording"
        description="Load an isolated native run with its source hashes, context answers and operation history. No AI is needed."
      >
        <Suspense fallback={<p role="status">Loading recorded decisions…</p>}>
          <Lifecycle />
        </Suspense>
      </LabLaunch>
      <h2>Distinguish the records</h2>
      <p>
        This experiment establishes and retracts a wording rule. A review decision concerns
        particular content: an approval is bound to the hash of the wording reviewed. Changed
        wording requires a new decision. A source change can also make an existing translation
        stale.
      </p>
      <p>
        The source-only native run refreshes the project context graph without AI. The search
        returns actual term occurrences with document and block identities. This recording does not
        execute a translation approval. Follow{" "}
        <Link to="/kapi/recipes/review-and-approve">Review and approve</Link> for an actual review
        queue, and <Link to="/kapi/convergence">the kapi loop</Link> for how pending and stale
        content is handled. Target-language drift is pending work; release coverage is an explicit
        gate.
      </p>
      <h2>Transfer</h2>
      <p>
        Reproduce the fixture. In a separate local test, record another observation and drop it
        instead of keeping it. Compare the operation log and the resulting context. Use the{" "}
        <Link to="/kapi/context-decisions">context operation guide</Link> to choose the command and
        inspect its recorded effect.
      </p>
    </LabLesson>
  );
}
