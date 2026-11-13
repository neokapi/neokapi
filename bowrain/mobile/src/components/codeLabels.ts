// The labels a code chip shows, one notation for every code in a run sequence:
// a paired code reads <b> and </b>, the closing half named after its opening
// half; a placeholder named by a bare word reads <br/>; a placeholder that
// already carries its own delimiters, such as {{name}}, reads as it is; a
// subblock reference reads [name]. A label is what a person reads for the
// code, never its native data.
import { projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";

/**
 * The name a code goes by: its display label, else its equiv, else its id.
 * A blank label is skipped.
 * The vocabulary writes display labels in brackets ("[B]", "[/B]"); the
 * brackets are dropped so the chip notation adds its own.
 */
export function codeName(code: { id: string; equiv?: string; disp?: string }): string {
  const name = [code.disp, code.equiv].find((n) => n && n.trim() !== "") ?? code.id;
  const bracketed = /^\[\/?(.+)\]$/.exec(name);
  return bracketed?.[1] ?? name;
}

const BARE = /^[\p{L}\p{N}_:.-]+$/u;

/** The name of each paired code in one run sequence, by id, from its opening half. */
const PAIR_NAMES: ModelRunSpec<readonly [string, string]> = {
  text: { dropped: "text is no code" },
  ph: { dropped: "a placeholder has no closing half to name" },
  pcOpen: (r) => [r.pcOpen.id, codeName(r.pcOpen)],
  pcClose: { dropped: "the closing half takes the name of its opening half" },
  sub: { dropped: "a subblock reference has no closing half to name" },
  // The forms of a plural or select are sequences of their own: a pair inside
  // a form opens and closes inside it, and is named when that form is drawn.
  plural: { dropped: "a form's pairs are named with the form" },
  select: { dropped: "a case's pairs are named with the case" },
  fallback: () => ["", ""],
};

/** The labels of the codes in one run sequence. */
export interface CodeLabels {
  ph(ph: { id: string; equiv?: string; disp?: string }): string;
  pcOpen(pc: { id: string; equiv?: string; disp?: string }): string;
  pcClose(pc: { id: string; equiv?: string }): string;
  sub(sub: { id: string; equiv?: string }): string;
}

export function codeLabels(runs: readonly Run[]): CodeLabels {
  const pairs = new Map(projectRuns(runs, PAIR_NAMES).filter(([id]) => id !== ""));
  return {
    ph: (ph) => {
      const name = codeName(ph);
      return BARE.test(name) ? `<${name}/>` : name;
    },
    pcOpen: (pc) => `<${pairs.get(pc.id) ?? codeName(pc)}>`,
    pcClose: (pc) => `</${pairs.get(pc.id) ?? codeName(pc)}>`,
    sub: (sub) => `[${codeName(sub)}]`,
  };
}
