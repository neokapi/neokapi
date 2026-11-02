/**
 * What the project-screens walkthrough's context has to hold before a take,
 * read from the scratch checkout's `kapi context log --json`.
 *
 * The seed records its suggestions from a checkout (`kapi context note`), keeps
 * one as a person (`kapi context review --keep`), and shares them with
 * `kapi context sync`. A take then keeps a suggestion on the server, once per
 * theme. The next seed sets those decisions aside with `kapi context reset`, so
 * every take starts from the same queue. The predicates live here so they can
 * be tested without a stack.
 */

/** One operation of `kapi context log --json`, to the fields the seed reads. */
export interface LoggedOp {
  id: string;
  short?: string;
  kind: string;
  actor?: { kind?: string; name?: string; session?: string };
  subject?: {
    kind?: string;
    text?: string;
    term?: { term?: string; replacement?: string };
  };
  target?: string;
  at: string;
  /** "reset" marks an operation a reset has set aside. */
  status?: string;
}

/**
 * One suggestion the seed records: a name or word (`term` with `insteadOf`),
 * which takes `manage_terms` to decide, or a note in prose (`text`), which
 * takes `manage_voice`.
 */
export interface SeedNote {
  term?: string;
  insteadOf?: string;
  text?: string;
  seenIn: string;
  quote: string;
}

/** Who the seed records as: the agent session its notes carry and the person who keeps. */
export interface SeedIdentity {
  session: string;
  person: string;
}

/** Whether op is the observation that records note. */
export function recordsNote(op: LoggedOp, note: SeedNote): boolean {
  if (op.kind !== "observe") return false;
  if (note.term !== undefined) {
    return op.subject?.kind === "term" && op.subject.term?.replacement === note.term;
  }
  return op.subject?.kind === "note" && op.subject.text === note.text;
}

/** The observations the seed's own agent session recorded. */
function seedObservations(ops: LoggedOp[], who: SeedIdentity): LoggedOp[] {
  return ops.filter((op) => op.kind === "observe" && op.actor?.session === who.session);
}

/** The observation the seed recorded for note, if it has. */
export function observationOf(ops: LoggedOp[], who: SeedIdentity, note: SeedNote): LoggedOp | undefined {
  return seedObservations(ops, who).find((op) => recordsNote(op, note));
}

/** Whether the seed's person has kept the observation id, and no reset set that aside. */
export function keptBySeed(ops: LoggedOp[], who: SeedIdentity, id: string): boolean {
  return ops.some(
    (op) => op.kind === "keep" && op.target === id && op.actor?.name === who.person && op.status !== "reset",
  );
}

/**
 * The operation a reset goes back to before: the first decision recorded after
 * the seed's own operations that no reset has set aside yet, or undefined when
 * there is none. Those are the decisions a take made on the server. The seed's
 * own operations are its agent session's observations and every keep its
 * person made of the observation that records keptNote, so a keep the seed
 * made again after something set the first aside is never reset. The seed's
 * resets are not counted, so a reset made by an earlier seed stays in force.
 */
export function resetPoint(ops: LoggedOp[], who: SeedIdentity, keptNote: SeedNote): LoggedOp | undefined {
  const observed = seedObservations(ops, who);
  if (observed.length === 0) return undefined;
  const keptId = observationOf(ops, who, keptNote)?.id;
  const seedKeep = (op: LoggedOp) =>
    op.kind === "keep" && op.actor?.name === who.person && keptId !== undefined && op.target === keptId;
  const boundary = Math.max(...observed.map(instant));
  return byTime(
    ops.filter(
      (op) => instant(op) > boundary && op.kind !== "reset" && op.status !== "reset" && !seedKeep(op),
    ),
  )[0];
}

/**
 * The instant an operation was recorded. The log writes RFC 3339 with the
 * fraction's trailing zeros dropped, so two stamps compare as instants, never
 * as text.
 */
function instant(op: LoggedOp): number {
  return Date.parse(op.at);
}

/** ops ordered by the instant each was recorded, oldest first. */
function byTime(ops: LoggedOp[]): LoggedOp[] {
  return [...ops].sort((a, b) => instant(a) - instant(b));
}

/** A question of the server's Decide view, to the fields the seed reads. */
export interface DecideQuestion {
  id: string;
  kind: string;
  label?: string;
  assignee_id?: string;
}

/**
 * Why the queue cannot carry a take, or "" when it can. A take keeps one
 * suggestion per theme, so the queue holds at least two, and it holds both a
 * name or word and a note, so the questions need both manage_terms and
 * manage_voice.
 */
export function queueShortfall(questions: DecideQuestion[], kept: number): string {
  const terms = questions.filter((q) => q.kind === "name" || q.kind === "word").length;
  const notes = questions.filter((q) => q.kind === "note").length;
  const missing: string[] = [];
  if (questions.length < 2) missing.push(`${questions.length} suggestion(s) waiting, the walk keeps one per theme`);
  if (terms === 0) missing.push("no name or word waiting");
  if (notes === 0) missing.push("no note waiting");
  if (kept === 0) missing.push("no kept rule, so Kept by starts empty");
  return missing.join("; ");
}
