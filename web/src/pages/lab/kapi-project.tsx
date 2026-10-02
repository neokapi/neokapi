import React from "react";
import Link from "@docusaurus/Link";
import LabLesson from "@site/src/components/Lab/LabLesson";
import LabLaunch from "@site/src/components/Lab/LabLaunch";
import { ProjectExplorer } from "@site/src/components/Lab/ProjectExplorer";

export default function KapiProjectLesson(): React.ReactElement {
  return (
    <LabLesson lessonId="kapi-project">
      <h2>Predict the lifecycle</h2>
      <p>
        A recipe identifies source files, target languages and a flow. Before executing, identify
        which step reads source content, which changes the working state, and which writes output
        files.
      </p>
      <ol>
        <li>Start with the JSON sample and the content-memory flow. Read its recipe and source.</li>
        <li>Run extract, then inspect project state before continuing.</li>
        <li>
          Run the flow and read its command output. Finally merge and compare the written target
          with the source.
        </li>
        <li>
          Switch to translate-exact and repeat. The supplied entries match exactly, so both flows
          should agree. Add an exclamation mark to “Welcome to Acme” and compare the flows at their
          declared 75% and 100% thresholds. Inspect whether reused wording needs review.
        </li>
      </ol>
      <p>
        <strong>Live browser execution.</strong> The content-memory sample supplies stored wording,
        and reusing it needs no model. Browser project storage supports this extraction and merge
        exercise; a native workspace is needed for the complete context graph and operation history.
      </p>
      <LabLaunch
        label="Open project experiment"
        description="Load the project explorer, then start the engine to run each step."
      >
        <ProjectExplorer defaultSampleId="json" />
      </LabLaunch>
      <h2>Explain the result</h2>
      <p>
        Which step first wrote the target-language file? Which inputs would you commit to version
        control? Describe the difference between a recipe and the content held in a project store.
      </p>
      <h2>Transfer</h2>
      <p>
        Repeat with a document sample. Identify what changes in the source and output inspection
        while the lifecycle stays the same. Continue locally with{" "}
        <Link to="/kapi/get-started/first-project">your first project</Link> and the{" "}
        <Link to="/kapi/project-store">project store</Link>.
      </p>
    </LabLesson>
  );
}
