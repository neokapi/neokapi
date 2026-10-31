// How a digest item's rule reads: as a sentence, for the detail pane's title
// and a disagreement's sides, or on one line, for a row of the list.

import { t } from "@neokapi/i18n-react/runtime";
import type { DigestConflict, DigestItem, DigestTermRule } from "./types";

type RuleItem = Pick<DigestItem, "subject" | "sentence">;

/**
 * The rule as a sentence, with the forms set apart from the words around them.
 * The workspace's rules on the home read through it too.
 */
export function RuleSentence({ item }: { item: RuleItem }) {
  const s = item.subject;
  if (s.kind === "term" && s.term) {
    const avoid = <Forms rule={s.term} />;
    return s.term.replacement ? (
      <>
        Write <Form>{s.term.replacement}</Form>, not {avoid}
      </>
    ) : (
      <>Avoid {avoid}</>
    );
  }
  if (s.kind === "memory" && s.memory) {
    return (
      <>
        Translate <Form>{s.memory.source}</Form> into {s.memory.target_locale} as{" "}
        <Form>{s.memory.target}</Form>
      </>
    );
  }
  return <>{s.text || item.sentence}</>;
}

/**
 * The rule on one line: the form to write, then the first form it replaces,
 * struck through and muted. The rest of the forms are in the detail pane.
 */
export function RuleLine({ item }: { item: RuleItem }) {
  const s = item.subject;
  if (s.kind === "term" && s.term) {
    const avoided = s.term.term || s.term.forms?.[0] || "";
    const struck = <Struck>{avoided}</Struck>;
    if (!s.term.replacement) return <span>Avoid {struck}</span>;
    const write = <Form>{s.term.replacement}</Form>;
    return avoided ? (
      <span>
        {write}, not {struck}
      </span>
    ) : (
      write
    );
  }
  if (s.kind === "memory" && s.memory) {
    const source = <Form>{s.memory.source}</Form>;
    const target = <Form>{s.memory.target}</Form>;
    return (
      <span>
        {source} as {target}
      </span>
    );
  }
  return <span>{s.text || item.sentence}</span>;
}

/** The form each side of a disagreement writes, the way a row and a title say it. */
function sideForm(side: DigestItem): string {
  const term = side.subject.term;
  return term?.replacement || term?.term || side.subject.text || side.sentence;
}

/** A disagreement on one line: "sign in or log in". */
export function ConflictLine({ conflict }: { conflict: DigestConflict }) {
  const [first, second] = conflict.sides;
  if (!first) return null;
  if (!second) return <RuleLine item={first} />;
  const a = <Form>{sideForm(first)}</Form>;
  const b = <Form>{sideForm(second)}</Form>;
  return (
    <span>
      {a} or {b}
    </span>
  );
}

/** A disagreement as a question, for the detail pane's title: "sign in or log in?" */
export function ConflictQuestion({ conflict }: { conflict: DigestConflict }) {
  const [first, second] = conflict.sides;
  if (!first) return null;
  if (!second) return <RuleSentence item={first} />;
  const a = <Form>{sideForm(first)}</Form>;
  const b = <Form>{sideForm(second)}</Form>;
  return (
    <span>
      {a} or {b}?
    </span>
  );
}

/** A suggestion theme's name, written here so it is translated with the rest. */
export function themeLabel(theme: string): string {
  switch (theme) {
    case "names":
      return t("Names and spellings");
    case "words":
      return t("Words to avoid");
    case "writing":
      return t("How the project writes");
    case "memory":
      return t("Wording in other languages");
  }
  return theme;
}

function Form({ children }: { children: React.ReactNode }) {
  return (
    <span className="font-medium" translate="no">
      {children}
    </span>
  );
}

function Struck({ children }: { children: React.ReactNode }) {
  return (
    <s className="text-muted-foreground decoration-muted-foreground/60" translate="no">
      {children}
    </s>
  );
}

/** The forms a rule avoids, struck through, as "a, b / c". */
function Forms({ rule }: { rule: DigestTermRule }) {
  const forms = [rule.term ?? "", ...(rule.forms ?? [])].filter(Boolean);
  return (
    <span translate="no" className="text-muted-foreground">
      {forms.map((f, i) => (
        <span key={f}>
          {i > 0 && (i === forms.length - 1 ? " / " : ", ")}
          <span className="line-through decoration-muted-foreground/50">{f}</span>
        </span>
      ))}
    </span>
  );
}
