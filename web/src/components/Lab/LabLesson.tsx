import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { LAB_LESSONS, LAB_MODES, lessonPath } from "./curriculum";
import styles from "./curriculum.module.css";

export function LabLesson({
  lessonId,
  children,
}: {
  lessonId: string;
  children: React.ReactNode;
}): React.ReactElement {
  const index = LAB_LESSONS.findIndex((lesson) => lesson.id === lessonId);
  const lesson = LAB_LESSONS[index];
  if (!lesson) throw new Error(`Unknown lab lesson: ${lessonId}`);
  const previous = LAB_LESSONS[index - 1];
  const next = LAB_LESSONS[index + 1];
  const mode = LAB_MODES[lesson.mode];
  return (
    <Layout title={`${lesson.title} | Labs`} description={lesson.outcome}>
      <main className={styles.page}>
        <nav className={styles.breadcrumb} aria-label="Breadcrumb">
          <Link to="/labs">Labs</Link>
          <span aria-hidden="true">/</span>
          <Link to={`/labs#${lesson.stage}`}>
            {lesson.stage === "framework" ? "The neokapi framework" : "Working with kapi"}
          </Link>
        </nav>
        <header className={styles.lessonHeader}>
          <p className={styles.eyebrow}>
            Lesson {index + 1} of {LAB_LESSONS.length}
          </p>
          <h1>{lesson.title}</h1>
          <p className={styles.question}>{lesson.question}</p>
          <p className={styles.outcome}>
            <strong>Learning objective.</strong> {lesson.outcome}
          </p>
          <div className={styles.execution}>
            <span className={styles.mode}>{mode.label}</span>
            <span>{mode.description}</span>
          </div>
        </header>
        <div className={styles.lessonContent}>{children}</div>
        <nav className={styles.lessonNavigation} aria-label="Lesson navigation">
          {previous ? (
            <Link to={lessonPath(previous)}>
              <span>Previous lesson</span>
              <strong>{previous.title}</strong>
            </Link>
          ) : (
            <Link to="/labs">
              <span>Course map</span>
              <strong>All lessons</strong>
            </Link>
          )}
          {next ? (
            <Link to={lessonPath(next)}>
              <span>Next lesson</span>
              <strong>{next.title}</strong>
            </Link>
          ) : (
            <Link to="/labs#electives">
              <span>Continue exploring</span>
              <strong>Electives and workspaces</strong>
            </Link>
          )}
        </nav>
      </main>
    </Layout>
  );
}

export default LabLesson;
