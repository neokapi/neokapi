import React from "react";
import Link from "@docusaurus/Link";
import type { ContentLabProps } from "@neokapi/kapi-lab";
import { ContentLab, ConversionExplorer, SegmentationLab } from "./index";
import { FlowBuilderRunner } from "./FlowBuilderRunner";
import { LabLesson } from "./LabLesson";
import { LabLaunch } from "./LabLaunch";

// Keep the course's experiments explicit: no implicit model-backed annotator.
const CHECK_LESSONS: ContentLabProps["lessons"] = [
  {
    id: "literal-checks",
    label: "Inspect hygiene findings",
    description:
      "Run hygiene checks on the selected file and inspect their attached findings.",
    sampleId: "checkout-checks",
    spec: {
      annotate: { qa: true, term: false, brand: false, segment: false },
      tab: "blocks",
    },
  },
];

export type FrameworkLessonId =
  | "representing-content"
  | "editing-with-fidelity"
  | "tools-and-annotations"
  | "segmentation"
  | "composing-flows"
  | "checks-and-coverage";

function Evidence({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <section aria-label="Interpret the evidence">
      <h2>Interpret the evidence</h2>
      {children}
    </section>
  );
}

function Transfer({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <section aria-label="Transfer exercise">
      <h2>Transfer exercise</h2>
      {children}
    </section>
  );
}

function RepresentingContent(): React.ReactElement {
  return (
    <>
      <h2>Predict before running</h2>
      <p>
        The checkout messages contain a customer name and different wording for
        an empty cart, one item, or several items. Predict which parts are
        editable text and which parts control how the application selects or
        completes a message.
      </p>
      <h2>Experiment</h2>
      <ol>
        <li>
          Launch the inspector and read checkout.mf. Find the name placeholder
          and compare its run with the original source. Locate the blocks for
          the plural branches.
        </li>
        <li>
          Select page.html. Find a paired inline code. Identify the text it
          encloses and the markup it represents.
        </li>
        <li>
          Select the JSON sample. Compare its literal <code>{"{name}"}</code>{" "}
          with an HTML inline code. The reader determines how format syntax
          enters the model.
        </li>
      </ol>
      <LabLaunch
        label="Open the content inspector"
        description="Read the supplied files with the browser engine. No AI model is needed."
      >
        <ContentLab
          autoStart
          lessonIds={["anatomy", "structure", "bilingual"]}
          defaultLessonId="anatomy"
          defaultSampleId="checkout-messageformat"
        />
      </LabLaunch>
      <Evidence>
        <p>
          The inspector shows the document assembled from the reader's output. A
          block contains source runs and may carry targets or overlays.
          Structural information describes how blocks belong to the document; it
          is distinct from editable text.
        </p>
        <p>
          Use the raw source and block view together. The MessageFormat reader
          extracts separate blocks for plural branches. The KBF example below
          also demonstrates a plural represented inside a Run. Similar rendered
          text can have different representations. A visible token is not
          evidence that the reader assigned it a placeholder kind.
        </p>
        <details>
          <summary>Examine more of the model</summary>
          <p>
            Select the bilingual lesson to inspect source and target on the same
            block. Open the
            <Link to="/kbf-lab"> KBF explorer</Link> to examine the serialized
            representation, including a plural Run. Compare that representation with
            the separate plural branch blocks extracted from MessageFormat.
          </p>
        </details>
      </Evidence>
      <Transfer>
        <p>
          Inspect a file in another supported format. Record one piece of
          editable text, one piece of structural information, and one inline
          construct. Explain how you identified each.
        </p>
      </Transfer>
      <p>
        Read: <Link to="/framework/content-model">Content model</Link> and
        <Link to="/framework/inline-formatting"> inline formatting</Link>.
      </p>
    </>
  );
}

function EditingWithFidelity(): React.ReactElement {
  return (
    <>
      <h2>Predict before running</h2>
      <p>
        A transformation changes the text in checkout.mf. Predict what should
        happen to its name placeholder and plural selectors. Would the same
        preservation criteria make sense when converting the file to a different
        format?
      </p>
      <h2>Experiment</h2>
      <ol>
        <li>
          Launch the round-trip inspector. It applies pseudo-translation to the
          checkout sample and displays the written output against the original.
        </li>
        <li>
          Compare changed text with the surrounding syntax. Inspect the output
          as blocks as well as raw text.
        </li>
        <li>
          Repeat with the HTML sample. Separate changes to visible content from
          changes to markup.
        </li>
      </ol>
      <LabLaunch
        label="Run the round-trip experiment"
        description="Pseudo-translation is deterministic and needs no AI provider."
      >
        <ContentLab
          autoStart
          lessonIds={["roundtrip"]}
          defaultLessonId="roundtrip"
          defaultSampleId="checkout-messageformat"
        />
      </LabLaunch>
      <Evidence>
        <p>
          A diff provides evidence for this input, transformation, and writer.
          Preservation of structure, preservation of untouched bytes, and visual
          similarity are different properties. Inspect each property before
          making a fidelity claim.
        </p>
        <p>
          The browser preview is an interpretation of the content model. For an
          office document, visual fidelity also requires opening the written
          file in its normal application.
        </p>
      </Evidence>
      <details>
        <summary>Compare with conversion to another format</summary>
        <p>
          Launch the converter, keep the Markdown input fixed, and select HTML
          then another output format. Compare the rendered result and raw
          source. List the structures each destination can express. Conversion
          uses a destination writer and may change or omit unsupported features.
        </p>
        <LabLaunch label="Open the conversion experiment">
          <ConversionExplorer
            defaultSampleId="article-md"
            defaultTarget="html"
          />
        </LabLaunch>
      </details>
      <Transfer>
        <p>
          Choose one inline construct, such as a link. Define a preservation
          criterion, run the experiment on a second file, and report whether the
          evidence meets that criterion. State what your comparison leaves
          untested.
        </p>
      </Transfer>
      <p>
        Read: <Link to="/framework/formats">Formats</Link> and
        <Link to="/framework/format-maturity/axes"> format maturity axes</Link>.
      </p>
    </>
  );
}

function ToolsAndAnnotations(): React.ReactElement {
  return (
    <>
      <h2>Predict before running</h2>
      <p>
        The flow applies pseudo-translation followed by checks. Predict which
        step creates a target and which step reports findings. Should a finding
        itself change the text?
      </p>
      <h2>Experiment</h2>
      <ol>
        <li>
          Run the supplied flow once. Select the source and inspect a block
          before processing.
        </li>
        <li>
          Select the pseudo-translation step. Compare its incoming and outgoing
          block, including source and target.
        </li>
        <li>
          Select the check step. Inspect its findings separately from the
          content changes. Then inspect the written output at the sink.
        </li>
      </ol>
      <LabLaunch
        label="Open the tool experiment"
        description="This flow uses pseudo-translation and checks, with no model download."
      >
        <FlowBuilderRunner
          defaultScenarioId="pseudo"
          defaultSampleId="checkout-messageformat"
          scenarioIds={["pseudo"]}
        />
      </LabLaunch>
      <Evidence>
        <p>
          Tools have different effects: they can create targets, transform
          content, or attach findings and other stand-off state. Compare the
          selected step's input and output to determine its actual effect. An
          empty finding list does not mean every possible check ran.
        </p>
        <p>
          The flow transports Parts, including information beyond text blocks.
          Inspect the source and sink when assessing preservation; block text
          alone cannot establish that the whole document survived processing.
        </p>
      </Evidence>
      <Transfer>
        <p>
          Add a segmentation step before pseudo-translation and rerun. Identify
          the new overlay and explain how its role differs from a target and a
          check finding.
        </p>
      </Transfer>
      <p>
        Read: <Link to="/framework/tools">Tools</Link> and
        <Link to="/framework/checks"> checks</Link>.
      </p>
    </>
  );
}

function Segmentation(): React.ReactElement {
  return (
    <>
      <h2>Predict before running</h2>
      <p>
        In the default sample, mark the sentence endings on paper. Explain why
        the full stops in “Dr.”, “$3.50”, and “U.S.” have different roles from a
        sentence boundary.
      </p>
      <h2>Experiment</h2>
      <ol>
        <li>
          Use the “Abbreviations &amp; decimals” sample and English locale. Run
          the default rule and Unicode engines first.
        </li>
        <li>
          Compare each boundary with your prediction and the reference below.
          Record the position and reason for every disagreement.
        </li>
        <li>
          Change only one abbreviation to a full word and rerun. Observe whether
          the boundaries change.
        </li>
      </ol>
      <LabLaunch
        label="Open the segmentation experiment"
        description="The default comparison needs no AI model. Learned engines are optional and download their assets when selected and run."
      >
        <SegmentationLab />
      </LabLaunch>
      <Evidence>
        <details>
          <summary>Reference boundaries for the default sample</summary>
          <p>
            This authored reference follows conventional sentence punctuation.
            It is a teaching reference for this passage, not a measured accuracy
            score.
          </p>
          <ol>
            <li>Dr. Smith paid $3.50 for the U.S. edition on Jan. 5, 2024.</li>
            <li>Mr. Lee asked, “Is it ready?”</li>
            <li>It was.</li>
            <li>The next batch ships at 9 a.m.</li>
          </ol>
          <p>
            The question belongs to the sentence introducing it. The
            abbreviation at the end of the final sentence also marks the end of
            the passage. Dialogue conventions can produce legitimate differences
            in more complex passages; document your chosen convention.
          </p>
        </details>
        <p>
          Agreement measures consistency between engines. Accuracy requires a
          justified reference. A segmentation overlay marks spans on the
          existing runs; it does not require rewriting the source into separate
          blocks.
        </p>
        <p>
          Some optional learned engines produce spans through browser adapters.
          Their comparison does not establish parity with a native plugin or its
          performance.
        </p>
      </Evidence>
      <Transfer>
        <p>
          Switch to file input and inspect a document with inline formatting.
          Explain how a boundary affects the unit passed to later tools, and
          identify the inline content that must remain associated with that
          unit.
        </p>
      </Transfer>
      <p>
        Read: <Link to="/framework/segmentation">Segmentation</Link>.
      </p>
    </>
  );
}

function ComposingFlows(): React.ReactElement {
  return (
    <>
      <h2>Predict before running</h2>
      <p>
        A check can inspect only the content available when it runs. Predict
        what changes if you move the check before pseudo-translation.
        Distinguish the source and sink from processing steps.
      </p>
      <h2>Experiment</h2>
      <ol>
        <li>Run the initial flow and inspect the check's input and output.</li>
        <li>
          Move the check before pseudo-translation, keeping the input file and
          tool configuration fixed. Run again and compare findings and available
          targets.
        </li>
        <li>
          Open the recipe view. Locate the steps and bindings corresponding to
          the canvas. Add a segmentation step and observe the recipe change.
        </li>
      </ol>
      <LabLaunch
        label="Open the flow experiment"
        description="The initial flow needs no AI. Recorded native traces are a separate replay option."
      >
        <FlowBuilderRunner
          defaultScenarioId="pseudo"
          defaultSampleId="checkout-messageformat"
          scenarioIds={["pseudo"]}
          withRecordedTraces
        />
      </LabLaunch>
      <Evidence>
        <p>
          Use intermediate content to explain the result. A successful run
          establishes that the configured flow executed; its usefulness depends
          on tool order, configuration, and the checks actually performed.
        </p>
        <details>
          <summary>Failures and native execution</summary>
          <p>
            Add a script step and enter an incomplete JavaScript expression in
            its configuration. Run and inspect the error. Close and reopen the
            experiment to restore the initial flow controls before continuing.
          </p>
          <p>
            The recorded trace selector replays native executions. Compare their
            worker activity with the live browser trace. A recording supplies
            evidence about its recorded run; changing the canvas does not rerun
            that recording.
          </p>
        </details>
      </Evidence>
      <Transfer>
        <p>
          Build a flow that segments, pseudo-translates, and checks the sample.
          Explain each step's input and output, then use a second file to test
          whether your explanation still holds.
        </p>
      </Transfer>
      <p>
        Read: <Link to="/framework/flows">Flows</Link> and
        <Link to="/framework/pipeline"> processing pipelines</Link>.
      </p>
    </>
  );
}

function ChecksAndCoverage(): React.ReactElement {
  return (
    <>
      <h2>Predict before running</h2>
      <p>
        A paragraph is grammatically tidy but contradicts a product policy.
        Would a whitespace or length check detect the contradiction? Name the
        information a checker would need.
      </p>
      <h2>Experiment</h2>
      <ol>
        <li>
          Launch the inspector and read the hygiene findings on
          checkout-checks.mf. Locate each finding in the source and identify its
          rule.
        </li>
        <li>
          Select checkout.mf as the clean comparison using the same checks.
          Record what ran and what it reported, including cases with no
          findings.
        </li>
        <li>
          Open the recorded case study below. Compare a clean case with a
          semantic contradiction and inspect the report's analyzer execution
          evidence.
        </li>
      </ol>
      <LabLaunch
        label="Open the check inspector"
        description="Runs the browser inspector’s deterministic hygiene checks and displays their findings."
      >
        <ContentLab autoStart lessons={CHECK_LESSONS} />
      </LabLaunch>
      <Evidence>
        <p>
          A verdict describes the configured checks. Coverage describes which
          checks ran, skipped, or were unavailable. Missing semantic coverage
          cannot be inferred from a clean set of literal findings. Keep these
          conclusions separate from format fidelity.
        </p>
        <p>
          The <Link to="/content-lab">recorded content context case study</Link>{" "}
          provides inputs, resolved context, reports, and execution provenance.
          Compare its clean, assurance, repair, and semantic-contradiction
          cases. The repair is authored and separately checked.
        </p>
        <details>
          <summary>Evidence to retain</summary>
          <p>
            Record the input, rule configuration, individual findings, and
            execution coverage. In the case study, expand the raw report and
            inspect <code>execution.analyzers</code>, including{" "}
            <code>voice.guidance</code>. Use recorded provenance to identify the
            engine and inputs behind the result. The browser inspector's
            overlays alone are insufficient evidence for a claim that every
            analyzer ran.
          </p>
        </details>
      </Evidence>
      <Transfer>
        <p>
          Write a short statement that is well formed but factually inconsistent
          with a supplied policy. Specify a check that could catch it, the
          context it requires, and how you would report an unavailable analyzer.
        </p>
      </Transfer>
      <p>
        Read: <Link to="/framework/checks">Checks</Link> and
        <Link to="/framework/checks/rule-checks"> rule-based checks</Link>.
      </p>
    </>
  );
}

const LESSON_CONTENT: Record<FrameworkLessonId, () => React.ReactElement> = {
  "representing-content": RepresentingContent,
  "editing-with-fidelity": EditingWithFidelity,
  "tools-and-annotations": ToolsAndAnnotations,
  segmentation: Segmentation,
  "composing-flows": ComposingFlows,
  "checks-and-coverage": ChecksAndCoverage,
};

export default function FrameworkLesson({
  lessonId,
}: {
  lessonId: FrameworkLessonId;
}): React.ReactElement {
  const Content = LESSON_CONTENT[lessonId];
  return (
    <LabLesson lessonId={lessonId}>
      <Content />
    </LabLesson>
  );
}
