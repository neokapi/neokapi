import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { LAB_ELECTIVES, LAB_LESSONS, LAB_MODES, lessonPath } from "../components/Lab/curriculum";
import styles from "../components/Lab/curriculum.module.css";

export default function LabsOverviewPage(): React.ReactElement {
  return (
    <Layout
      title="Labs"
      description="A guided course in the neokapi content model, processing flows, and kapi projects, context and decisions."
    >
      <main className={styles.page}>
        <header className={styles.hero}>
          <p className={styles.eyebrow}>Experiments in content processing</p>
          <h1>Learn the framework. Work with kapi.</h1>
          <p className={styles.lead}>
            Start with one document and follow its content through the engine. Then examine how kapi
            applies context and records decisions across processing runs.
          </p>
          <p>
            Each lesson presents a question, a prediction, an experiment and evidence to inspect.
            Follow the sequence or choose a topic. The opening lessons need no installation or API
            key.
          </p>
          <Link className={styles.start} to={lessonPath(LAB_LESSONS[0])}>
            Start with the content model
          </Link>
        </header>
        {(["framework", "kapi"] as const).map((stage) => (
          <section
            className={styles.stage}
            id={stage}
            key={stage}
            aria-labelledby={`${stage}-heading`}
          >
            <h2 id={`${stage}-heading`}>
              {stage === "framework" ? "The neokapi framework" : "Working with kapi"}
            </h2>
            <p>
              {stage === "framework"
                ? "Learn what a processing run reads, changes, preserves and checks. Begin here before using the full flow editor."
                : "Build on the processing model to study recipes, applicable context and decisions. Recorded native cases cover capabilities beyond the browser runtime."}
            </p>
            <ol
              className={styles.lessons}
              start={
                stage === "framework"
                  ? 1
                  : LAB_LESSONS.findIndex((lesson) => lesson.stage === stage) + 1
              }
            >
              {LAB_LESSONS.filter((lesson) => lesson.stage === stage).map((lesson) => (
                <li key={lesson.id}>
                  <Link className={styles.lessonCard} to={lessonPath(lesson)}>
                    <span className={styles.number} aria-hidden="true">
                      {String(LAB_LESSONS.indexOf(lesson) + 1).padStart(2, "0")}
                    </span>
                    <div>
                      <h3>{lesson.title}</h3>
                      <p>{lesson.question}</p>
                      <span className={styles.mode}>{LAB_MODES[lesson.mode].label}</span>
                    </div>
                  </Link>
                </li>
              ))}
            </ol>
          </section>
        ))}
        <section className={styles.stage} aria-labelledby="execution-heading">
          <h2 id="execution-heading">Know what is running</h2>
          <dl className={styles.modes}>
            <div>
              <dt>Live browser experiments</dt>
              <dd>
                Open an experiment to load its runtime. Basic exercises run locally; model-based
                electives may need additional downloads and browser capabilities.
              </dd>
            </div>
            <div>
              <dt>Recorded native experiments</dt>
              <dd>
                Inspect saved inputs, context and results. Use the reproduction instructions to run
                or change a case with a local installation.
              </dd>
            </div>
            <div>
              <dt>AI and media</dt>
              <dd>
                Execution labels distinguish recorded model output, illustrative demo providers and
                browser model bridges. A demo response demonstrates processing, not model quality.
              </dd>
            </div>
          </dl>
        </section>
        <section className={styles.stage} id="electives" aria-labelledby="electives-heading">
          <h2 id="electives-heading">Electives and workspaces</h2>
          <p>
            Apply the core concepts to additional formats and media, or explore without the lesson
            sequence.
          </p>
          <ul className={styles.electives}>
            {LAB_ELECTIVES.map((lab) => (
              <li key={lab.to}>
                <Link to={lab.to}>{lab.title}</Link>. {lab.description}
              </li>
            ))}
            <li>
              <Link to="/lab">Flow workspace</Link>. Compose a flow, explore scripting and inspect
              execution traces.
            </li>
            <li>
              <Link to="/playground-cli">CLI playground</Link>. Experiment with commands and the
              browser filesystem.
            </li>
          </ul>
        </section>
      </main>
    </Layout>
  );
}
