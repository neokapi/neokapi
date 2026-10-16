// How a suggested rule reads: as a sentence, for the detail pane's title and a
// disagreement's sides, or on one line, for a row of the list. The wording is
// Kapi Desktop's, so a rule reads the same on either surface.
import type { CandidateRule } from "./types";
import type { CandidateRow } from "./candidateRows";

/** "Write use, not utilize", or "Avoid synergy" for a phrasing with no form to write. */
export function CandidateSentence({ candidate }: { candidate: CandidateRule }) {
  const avoid = <Struck>{candidate.term}</Struck>;
  return candidate.replacement ? (
    <>
      Write <Form>{candidate.replacement}</Form>, not {avoid}
    </>
  ) : (
    <>Avoid {avoid}</>
  );
}

/** The rule on one line: the form to write, then the phrasing it replaces, struck through. */
export function CandidateLine({ candidate }: { candidate: CandidateRule }) {
  const struck = <Struck>{candidate.term}</Struck>;
  if (!candidate.replacement) return <span>Avoid {struck}</span>;
  return (
    <span>
      <Form>{candidate.replacement}</Form>, not {struck}
    </span>
  );
}

/** A disagreement on one line: "use or employ, not utilize". */
export function ConflictLine({ row }: { row: CandidateRow }) {
  const [first, second] = row.sides ?? [];
  if (!first || !second) return <CandidateLine candidate={row.candidate} />;
  return (
    <span>
      <SideForm side={first} /> or <SideForm side={second} />, not <Struck>{first.term}</Struck>
    </span>
  );
}

/** A disagreement as a question, for the detail pane's title: "use or employ for utilize?" */
export function ConflictQuestion({ row }: { row: CandidateRow }) {
  const [first, second] = row.sides ?? [];
  if (!first || !second) return <CandidateSentence candidate={row.candidate} />;
  return (
    <span>
      <SideForm side={first} /> or <SideForm side={second} /> for <Struck>{first.term}</Struck>?
    </span>
  );
}

/** The form one side writes. A side with none removes the phrasing. */
function SideForm({ side }: { side: CandidateRule }) {
  return side.replacement ? <Form>{side.replacement}</Form> : <>nothing</>;
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
