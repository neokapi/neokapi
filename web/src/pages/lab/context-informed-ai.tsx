import React, { Suspense } from "react";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import LabLesson from "@site/src/components/Lab/LabLesson";
import LabLaunch from "@site/src/components/Lab/LabLaunch";
const Evidence = React.lazy(() => import("@site/src/components/Lab/KapiLessonAIEvidence"));
export default function ContextInformedAILesson(): React.ReactElement {
  const authoringHref = useBaseUrl("/authoring-lab");
  return (
    <LabLesson lessonId="context-informed-ai">
      <h2>Separate availability from use</h2>
      <p>
        A context graph can answer which guidance applies. An assistant must retrieve that answer
        and use it while writing. Predict how supplying a guide in the prompt differs from making it
        available in a workspace.
      </p>
      <ol>
        <li>
          Choose a model and an audience. Read the common task and applicable guide before comparing
          outputs.
        </li>
        <li>
          Identify a concrete difference in the documents. Trace it to a guide instruction or a
          source file the agent read.
        </li>
        <li>
          Switch context delivery to the workspace. Inspect the recorded kapi commands to determine
          whether that run retrieved any guidance.
        </li>
        <li>
          Repeat with another audience. State which observations this sample supports and what would
          need repeated, independently assessed runs.
        </li>
      </ol>
      <LabLaunch
        label="Open recorded AI experiment"
        description="Load recorded documents and context. No provider account, model download or paid request is needed."
      >
        <Suspense fallback={<p role="status">Loading recorded authoring runs…</p>}>
          <Evidence authoringHref={authoringHref} />
        </Suspense>
      </LabLaunch>
      <h2>Evaluate the evidence</h2>
      <p>
        Different wording is an observation, not a quality score. Check factual claims against the
        source, then assess whether the document meets its audience's needs. A passing literal check
        cannot establish either property.
      </p>
      <p>
        The browser demonstration provider produces illustrative output for some playground
        commands. That output establishes how a flow runs, not how well an AI model writes. The
        recorded documents above are actual model outputs with a separate execution history.
      </p>
      <h2>Transfer</h2>
      <p>
        For your own project, explicitly retrieve the applicable context before drafting and record
        what the assistant used. Follow{" "}
        <Link to="/kapi/get-started/use-with-claude">the agent setup guide</Link>. Native provider
        runs require your own configured credentials; inspect provider costs before running an
        experiment.
      </p>
    </LabLesson>
  );
}
