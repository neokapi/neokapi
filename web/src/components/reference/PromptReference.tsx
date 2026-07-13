import React from "react";
import { prompts } from "@neokapi/reference-data";
import type { PromptEntry, PromptSection } from "@neokapi/reference-data";

import "./promptReference.css";

/**
 * The generated prompt reference: every prompt kapi sends to a language model,
 * rendered from prompts.json — which gen-refs produces from the same builders
 * the binary uses, and a CI drift gate keeps honest.
 *
 * Nothing on this page is written by hand. Reword a prompt in the code and this
 * page changes with it, or the build fails.
 */

/** Who owns a section, in the user's terms. */
const OWNER: Record<PromptSection["kind"], string> = {
  task: "framework",
  constraint: "framework",
  instruction: "you",
  voice: "you",
  glossary: "you",
  content: "your document",
};

function SectionBlock({ section }: { section: PromptSection }) {
  return (
    <div className="prompt-section" data-kind={section.kind}>
      <div className="prompt-section__meta">
        <code>{section.kind}</code>
        <span>· {section.origin}</span>
        <span className="prompt-section__owner">{OWNER[section.kind]}</span>
      </div>
      <pre className="prompt-section__text">
        {section.heading ? `${section.heading}\n` : ""}
        {section.text}
      </pre>
    </div>
  );
}

function Prompt({ entry }: { entry: PromptEntry }) {
  return (
    <section className="prompt-entry" id={entry.id}>
      <h3>
        <code>{entry.id}</code>
      </h3>
      <p>
        {entry.summary} Sent by <code>{entry.tool}</code>.
      </p>

      {!entry.structured && (
        <div className="theme-admonition alert alert--secondary prompt-entry__inline">
          <p>
            This prompt is still built inline at its call site, so its text cannot be rendered here.
            Run the tool with <code>--explain-prompts</code> to see exactly what it sends — that
            path always shows the real thing.
          </p>
        </div>
      )}

      {entry.turns?.map((turn) => (
        <div key={turn.role} className="prompt-turn">
          <h4>{turn.role}</h4>
          {turn.sections.map((s, i) => (
            <SectionBlock key={`${s.kind}-${i}`} section={s} />
          ))}
        </div>
      ))}
    </section>
  );
}

export default function PromptReference() {
  return (
    <div className="prompt-reference">
      <p>
        Catalog version <code>{prompts.version}</code> · {prompts.prompts.length} prompts.
      </p>
      {prompts.prompts.map((entry) => (
        <Prompt key={entry.id} entry={entry} />
      ))}
    </div>
  );
}
