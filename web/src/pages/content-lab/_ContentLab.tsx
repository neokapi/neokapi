import React, { useState } from "react";
import type { AudienceEvidence, RecordedCase } from "./_types";
import styles from "./styles.module.css";

const audiences = [
  {
    id: "child",
    label: "Child",
    need: "Reading with a trusted adult. Familiar words and one step at a time.",
  },
  {
    id: "teen",
    label: "Teen",
    need: "Private, direct instructions, with clear choices about getting help.",
  },
  {
    id: "adult",
    label: "Adult",
    need: "Concise practical steps, eligibility and privacy boundaries.",
  },
  {
    id: "older-adult",
    label: "Older adult",
    need: "New to video appointments. Explain the technology and the route to a person.",
  },
];
const scenarios = [
  {
    id: "clean",
    label: "Original",
    detail: "The authored audience paragraph.",
  },
  {
    id: "assurance",
    label: "Violation",
    detail: "A prohibited assurance is inserted into the same file.",
  },
  {
    id: "repair",
    label: "Repair",
    detail: "An authored correction is checked again with the same profile.",
  },
  {
    id: "semantic-contradiction",
    label: "False facts",
    detail: "False service claims expose the boundary of literal pattern checks.",
  },
];
const views = ["Recorded checks", "Decision and scope", "Product relationships"] as const;

function JsonDetail({ title, value }: { title: string; value: unknown }) {
  return (
    <details className={styles.detail}>
      <summary>{title}</summary>
      <pre>{JSON.stringify(value, null, 2)}</pre>
    </details>
  );
}

function RecordedChecks({ evidence }: { evidence: AudienceEvidence }) {
  const [audience, setAudience] = useState("child");
  const [scenario, setScenario] = useState("assurance");
  const reader = audiences.find((item) => item.id === audience)!;
  const selected = scenarios.find((item) => item.id === scenario)!;
  const row = evidence.results.find((item) => item.id === `${audience}/${scenario}`);
  return (
    <section aria-labelledby="recorded-heading">
      <div className={styles.sectionHeading}>
        <div>
          <p className={styles.kicker}>One profile · four reading situations</p>
          <h2 id="recorded-heading">Change the audience. Keep the constraint.</h2>
        </div>
        <span className={styles.badge}>Recorded playback</span>
      </div>
      <div className={styles.controls}>
        <label>
          Audience
          <select
            aria-label="Audience"
            value={audience}
            onChange={(event) => setAudience(event.target.value)}
          >
            {audiences.map((item) => (
              <option key={item.id} value={item.id}>
                {item.label}
              </option>
            ))}
          </select>
        </label>
        <div>
          <span className={styles.controlLabel}>Paragraph</span>
          <div className={styles.switcher} aria-label="Recorded paragraph cases">
            {scenarios.map((item) => (
              <button
                key={item.id}
                type="button"
                aria-pressed={scenario === item.id}
                onClick={() => setScenario(item.id)}
              >
                {item.label}
              </button>
            ))}
          </div>
        </div>
      </div>
      <p className={styles.reader}>
        <strong>{reader.label}:</strong> {reader.need}
      </p>
      <div aria-live="polite" aria-atomic="true" className={styles.srOnly}>
        {reader.label}, {selected.label}.{" "}
        {row
          ? `${row.actualConstraintFindings} constraint findings. Exit ${row.run.exitCode}.`
          : "No recorded evidence for this case."}
      </div>
      {row ? (
        <CaseResult row={row} detail={selected.detail} scenario={scenario} />
      ) : (
        <div className={styles.notice} role="status">
          No recorded result is available for this case. Import a fresh sample artifact to inspect
          measured checks.
        </div>
      )}
      <p className={styles.caption}>
        Switching cases displays recorded input and output. It does not run the engine or a model in
        this browser. The repair is an authored fixture, not an automatic factual correction.
      </p>
    </section>
  );
}

function CaseResult({
  row,
  detail,
  scenario,
}: {
  row: RecordedCase;
  detail: string;
  scenario: string;
}) {
  const resolutions = row.resolvedContext.constraints ?? [];
  const pattern = resolutions.find((entry) => entry.constraint.kind === "prohibited_pattern");
  const guidance = resolutions.find((entry) => entry.constraint.kind === "guidance");
  const semantic = row.report.execution?.analyzers.find((entry) => entry.id === "voice.guidance");
  return (
    <>
      <div className={styles.comparison}>
        <article className={styles.panel}>
          <p className={styles.kicker}>Recorded input</p>
          <h3>{row.input.title}</h3>
          <p className={styles.caption}>{detail}</p>
          <blockquote className={styles.paragraph}>{row.input.fact_body}</blockquote>
          <p className={styles.file}>{row.file}</p>
        </article>
        <article className={styles.panel}>
          <p className={styles.kicker}>Recorded result</p>
          <h3 className={row.report.pass ? styles.resultPass : styles.resultFail}>
            {row.report.pass ? "Configured gate passed" : "Configured gate failed"}
          </h3>
          <div className={styles.metrics}>
            <div>
              <strong>{row.actualConstraintFindings}</strong>
              <span>constraint findings</span>
            </div>
            <div>
              <strong>{row.report.summary.findings}</strong>
              <span>all findings</span>
            </div>
            <div>
              <strong>{row.run.exitCode}</strong>
              <span>process exit</span>
            </div>
          </div>
          <p>
            {row.actualConstraintFindings > 0
              ? "The exact wording rule detected the assurance, even with this audience’s presentation overrides."
              : "The configured deterministic checks found no shared-constraint violation."}
          </p>
          <div className={styles.boundary}>
            <strong>
              Factual guidance:{" "}
              {semantic?.status === "unsupported" ? "unsupported" : "coverage unreported"}
            </strong>
            <p>
              {scenario === "semantic-contradiction"
                ? "This paragraph contradicts the service facts. The literal rule cannot establish that, and the passing gate does not make the paragraph true."
                : "Appointments, consent and device access still require human or semantic review. A passing wording check does not verify these facts."}
            </p>
          </div>
        </article>
      </div>
      <div className={styles.constraintGrid}>
        <article className={styles.constraint}>
          <p className={styles.kicker}>
            Deterministic constraint · {pattern?.status ?? "unreported"}
          </p>
          <h3>{pattern?.constraint.statement ?? "No pattern record"}</h3>
          <code>{pattern?.constraint.regex}</code>
          <p className={styles.caption}>
            Literal RE2 pattern matching. No synonym search or factual inference.
          </p>
        </article>
        <article className={styles.constraint}>
          <p className={styles.kicker}>
            Shared factual guidance · {guidance?.status ?? "unreported"}
          </p>
          <p>{guidance?.constraint.statement ?? "No guidance record"}</p>
          <p className={styles.caption}>
            Visible in the writer’s context. Semantic verification remains outside this
            deterministic run.
          </p>
        </article>
      </div>
      <div className={styles.details}>
        <JsonDetail
          title="Source and resolved context"
          value={{
            input: row.input,
            inputSha256: row.inputSha256,
            context: row.resolvedContext,
          }}
        />
        <JsonDetail
          title="Raw check report and command"
          value={{
            command: row.run.command,
            exitCode: row.run.exitCode,
            report: row.report,
            stdout: row.run.stdout,
            stderr: row.run.stderr,
          }}
        />
      </div>
    </>
  );
}

function DecisionScope({ evidence }: { evidence: AudienceEvidence }) {
  const [scope, setScope] = useState("child");
  const row = evidence.results.find((item) => item.id === "child/assurance");
  const constraint = row?.resolvedContext.constraints?.find(
    (entry) => entry.constraint.kind === "prohibited_pattern",
  )?.constraint;
  const exception = constraint?.exceptions?.find(
    (entry) => entry.scope.channel === "policy-example",
  );
  const excepted = scope === "policy-example";
  return (
    <section aria-labelledby="scope-heading">
      <div className={styles.sectionHeading}>
        <div>
          <p className={styles.kicker}>Scope and provenance</p>
          <h2 id="scope-heading">An exception belongs to one rule.</h2>
        </div>
        <span className={styles.badge}>Illustration · expected impact</span>
      </div>
      <p>
        A profile author can record why a policy example may quote prohibited wording. Select a
        point to inspect the expected consequence. These controls illustrate scope matching; they do
        not submit an approval or recompute a report.
      </p>
      <label className={styles.scopeSelect}>
        Illustrated channel
        <select
          aria-label="Illustrated channel"
          value={scope}
          onChange={(event) => setScope(event.target.value)}
        >
          {audiences.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
          <option value="policy-example">Policy example</option>
        </select>
      </label>
      <div className={styles.scopeFlow}>
        <article className={styles.panel}>
          <p className={styles.kicker}>Shared rule</p>
          <h3>Do not promise a risk-free service.</h3>
          <p>
            Channel tone and style may be replaced. The top-level constraint remains part of the
            selected profile.
          </p>
          <code>{constraint?.id ?? "Constraint provenance not loaded"}</code>
        </article>
        <div className={styles.connector} aria-hidden="true">
          →
        </div>
        <article className={styles.panel} aria-live="polite">
          <p className={styles.kicker}>Expected at {scope}</p>
          <h3>
            {excepted ? "Only the quoted example is excepted" : "The critical wording rule applies"}
          </h3>
          <p>
            {excepted
              ? "The exception matches the policy-example channel. It does not exempt any of the four audience pages."
              : "The policy-example exception does not match this audience. The same prohibited phrase remains a critical violation."}
          </p>
          <p>Factual guidance still needs review in either case.</p>
        </article>
      </div>
      <div className={styles.constraint}>
        <h3>Asserted local approval</h3>
        {exception ? (
          <dl className={styles.provenance}>
            <dt>Reason</dt>
            <dd>{exception.reason}</dd>
            <dt>Approved by</dt>
            <dd>{exception.approved_by}</dd>
            <dt>Approval reference</dt>
            <dd>
              <code>{exception.approval_ref}</code>
            </dd>
            <dt>Exact scope</dt>
            <dd>
              <code>{JSON.stringify(exception.scope)}</code>
            </dd>
          </dl>
        ) : (
          <p>Import the recorded context to inspect the authored exception.</p>
        )}
        <p className={styles.caption}>
          These fields record an author’s assertion. They are not authenticated proof of approval.
          The exception relaxes its owning wording constraint only.
        </p>
      </div>
      <p className={styles.caption}>
        Expected impact is separate from recorded verification. Select Recorded checks to inspect
        measured results at each audience point. This sample does not record a policy-example run.
      </p>
    </section>
  );
}

const products = {
  neokapi: {
    title: "neokapi",
    subtitle: "Content engine",
    relationship:
      "The engine supplies the content model, context resolution and check contracts used by product surfaces.",
    detail:
      "The audience fixture exercises these engine capabilities through kapi. Format structure, shared constraints and reported findings use the same underlying model.",
  },
  kapi: {
    title: "kapi",
    subtitle: "Project tools for people and agents",
    relationship:
      "kapi puts the engine into project workflows through the CLI, desktop app and MCP.",
    detail:
      "The recorded commands in this lab resolve a file’s audience context and check its content. The JSON report is evidence from an actual CLI run.",
  },
};

export interface RelatedProduct {
  id: string;
  title: string;
  subtitle: string;
  relationship: string;
  detail: string;
}

function ProductRelationships({ relatedProduct }: { relatedProduct?: RelatedProduct }) {
  const [selected, setSelected] = useState("kapi");
  const visibleProducts = [
    { id: "kapi", ...products.kapi },
    ...(relatedProduct ? [relatedProduct] : []),
  ];
  const detail =
    selected === "neokapi"
      ? products.neokapi
      : (visibleProducts.find((product) => product.id === selected) ?? products.kapi);
  return (
    <section aria-labelledby="products-heading">
      <div className={styles.sectionHeading}>
        <div>
          <p className={styles.kicker}>Product relationships</p>
          <h2 id="products-heading">An engine for content workflows.</h2>
        </div>
        <span className={styles.badge}>Architecture illustration</span>
      </div>
      <p>Select a product to trace its relationship to the evidence in this lab.</p>
      <div className={styles.productLayout}>
        <div className={styles.graph} aria-label="Product relationship graph">
          <div className={styles.productRow}>
            {visibleProducts.map(({ id, title, subtitle }) => (
              <button
                type="button"
                key={id}
                aria-pressed={selected === id}
                onClick={() => setSelected(id)}
              >
                <strong>{title}</strong>
                <span>{subtitle}</span>
              </button>
            ))}
          </div>
          <div className={styles.graphEdge}>
            <svg viewBox="0 0 500 64" aria-hidden="true">
              <path
                d={
                  visibleProducts.length === 1
                    ? "M250 0 V61 M242 53 L250 61 L258 53"
                    : "M125 0 V25 H375 V0 M250 25 V61 M242 53 L250 61 L258 53"
                }
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
              />
            </svg>
            <span>Built on the engine</span>
          </div>
          <button
            type="button"
            className={styles.engine}
            aria-pressed={selected === "neokapi"}
            onClick={() => setSelected("neokapi")}
          >
            <strong>neokapi</strong>
            <span>Content model · context · checks</span>
          </button>
        </div>
        <article className={styles.panel} aria-live="polite">
          <p className={styles.kicker}>{detail.subtitle}</p>
          <h3>{detail.title}</h3>
          <p>{detail.relationship}</p>
          <p>{detail.detail}</p>
        </article>
      </div>
    </section>
  );
}

export function ContentLab({
  evidence,
  relatedProduct,
}: {
  evidence: AudienceEvidence;
  relatedProduct?: RelatedProduct;
}) {
  const [view, setView] = useState<(typeof views)[number]>("Recorded checks");
  return (
    <main className={styles.lab}>
      <header className={styles.hero}>
        <p className={styles.kicker}>Content context lab</p>
        <h1>
          Different readers.
          <br />
          The same service facts.
        </h1>
        <p>
          Explore how shared constraints survive audience-specific presentation. Inspect the
          recorded checks, their provenance and the limits of what they establish.
        </p>
      </header>
      <nav className={styles.viewNav} aria-label="Content lab views">
        {views.map((label) => (
          <button
            type="button"
            key={label}
            aria-pressed={view === label}
            onClick={() => setView(label)}
          >
            {label}
          </button>
        ))}
      </nav>
      {evidence.failure ? (
        <div className={styles.notice} role="status">
          The fixture stopped before completing: {evidence.failure.message}. Only completed cases
          are available below.
        </div>
      ) : null}
      {view === "Recorded checks" ? (
        <RecordedChecks evidence={evidence} />
      ) : view === "Decision and scope" ? (
        <DecisionScope evidence={evidence} />
      ) : (
        <ProductRelationships relatedProduct={relatedProduct} />
      )}
      <footer className={styles.evidence}>
        <h2>Evidence and reproduction</h2>
        <p>
          {evidence.recordedAt
            ? `Recorded ${evidence.recordedAt.slice(0, 10)}. `
            : "Evidence import pending. "}
          Offline fixture, with no paid provider or live semantic review.
        </p>
        {evidence.sourceCommit ? (
          <p>
            Source commit{" "}
            <a href={`https://github.com/neokapi/neokapi/commit/${evidence.sourceCommit}`}>
              {evidence.sourceCommit.slice(0, 12)}
            </a>
            {evidence.sourceDirty ? " with uncommitted changes; inspect the diff hash below." : "."}
          </p>
        ) : null}
        <details className={styles.detail}>
          <summary>Reproduce locally</summary>
          <pre>
            {
              "make build\nnode samples/audience-context/run.mjs --binary bin/kapi --output /tmp/audience-results.json"
            }
          </pre>
          <p>
            The runner uses temporary project, configuration and plugin directories. Re-run it on
            the checked-out source to obtain new measurements.
          </p>
        </details>
        <JsonDetail
          title="Artifact provenance and limitations"
          value={{
            schema: evidence.schema,
            failure: evidence.failure,
            recordedAt: evidence.recordedAt,
            sourceCommit: evidence.sourceCommit,
            sourceDirty: evidence.sourceDirty,
            sourceDiffSha256: evidence.sourceDiffSha256,
            binaryVersion: evidence.binaryVersion,
            binarySha256: evidence.binarySha256,
            profileSha256: evidence.profileSha256,
            recipeSha256: evidence.recipeSha256,
            scope: evidence.scope,
            limitations: evidence.limitations,
          }}
        />
      </footer>
    </main>
  );
}
