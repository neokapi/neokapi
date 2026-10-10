// The context engine: the one lab that draws what the project knows while the
// commands change it. It runs in Compass, the multilingual sample, because
// that is where every part of the graph has something to show: a voice and
// terms in force at one point, a content memory that grows when the loop
// runs, editions that move through the gates, and a record of decisions a
// person made.

import type { Lab } from "../types.ts";

const DOCS = {
  context: { label: "Context", href: "/kapi/context" },
  growing: { label: "Growing context", href: "/kapi/context-decisions" },
  sharing: { label: "Sharing context", href: "/kapi/context-portability" },
  store: {
    label: "The context store and graph",
    href: "/contribute/architecture/context/c-03-context-store-and-graph",
  },
};

const DECIDE_NL = `kapi status --review --json --jq '.pending[]|select(.locale=="nl" and (.target|startswith("⟦")|not))|{op:"decide",at:{doc:.relative,block:.key,edition:.locale},if_match:"*",outcome:"establish"}' > dutch.json`;

export const contextEngine: Lab = {
  id: "context-engine",
  series: "context",
  position: 1,
  title: "The context engine",
  tagline: "What the project knows, drawn as a graph that grows with the work.",
  summary:
    "kapi keeps what a project knows about its own content in one place: the voice it writes in, the words it uses and avoids, the content memory, and the record of every decision. Checks and translations read that graph, every decision feeds it, and an assistant gets the same answer a person does. This lab draws the graph above the terminal and takes the Compass project through one cycle: read the context in, converge, review, converge again. Each command adds something to the picture.",
  sample: "compass",
  lens: "context-graph",
  concepts: [
    "context graph",
    "point",
    "voice profile",
    "terms",
    "content memory",
    "decision record",
  ],
  docs: [DOCS.context, DOCS.growing, DOCS.sharing, DOCS.store],
  minutes: 9,
  chapters: [
    {
      id: "content",
      title: "A project before it knows anything",
      narration:
        "Compass has one source file, 38 blocks of interface strings, and three editions of it: German and Norwegian nearly complete, Dutch half done. The graph shows the content and the point it sits at, and nothing else: no voice, no terms, no memory, no record. Nothing says who wrote those translations or whether anyone approved them, so every gate would answer from an empty store.",
      command: "kapi ls --stats",
      expect: ["en-GB.json"],
      focus: "content",
    },
    {
      id: "import",
      title: "Read the context in",
      narration:
        "The context arrives as files in the repository: a voice profile, the terms, two content-memory bundles and a record of decisions a reviewer made earlier. Reading them in fills the right-hand side of the graph in one step, and the editions already show the share a person established.",
      command: "kapi store import ./context",
      expect: ["54 recorded decisions", "content-memory entries"],
      notice: ["voice profile", "recorded decisions"],
      focus: "context",
    },
    {
      id: "point",
      title: "What applies at this point",
      narration:
        "Content sits at a point: the Northsea profile on the app channel. Ask what applies there and the answer is the voice, the terms and the rules that hang off that point in the graph. An assistant working in the repository asks the same question and reads the same answer.",
      command: "kapi context site/locales/en-GB.json | head -20",
      expect: ["northsea"],
      focus: "point",
    },
    {
      id: "up",
      title: "Converge, and the memory grows",
      narration:
        "The loop reads the memory before it writes anything. Blocks the project already settled are recycled, the rest are drafted, and what was written joins the memory for the next run: watch the entry count rise and the edges to each edition thicken. Dutch stays parked: its drafts are AI-translated and nobody has reviewed them.",
      command: "kapi up",
      expect: ["parked (needs human)"],
      notice: ["parked (needs human)"],
      focus: "memory",
    },
    {
      id: "uses",
      title: "The graph learns where words live",
      narration:
        "Once the engine has read the content, a term is no longer just a rule: it is placed. Each concept now points at the blocks that use it, and a check or a rewrite can go straight to them.",
      command: "kapi terms occurrences berth",
      expect: ["berth"],
      focus: "terms",
    },
    {
      id: "gate",
      title: "The gate reads the same graph",
      narration:
        "The ship gate asks each edition the question the picker will ask: translated, and established by a person? Dutch fails on the second count, which is a reviewer's job, and the exit code says so without anyone reading a report.",
      command: "kapi check --ship --gate voice",
      exit: 3,
      expect: ["nl/compass-app"],
      focus: "editions",
    },
    {
      id: "select",
      title: "Pick what to approve",
      narration:
        "The review queue is data. One filter keeps the Dutch blocks that carry a real translation, and turns each into a decide operation addressed at its block.",
      command: DECIDE_NL,
      look: { file: "dutch.json", view: "raw" },
      focus: "editions",
    },
    {
      id: "decide",
      title: "A review becomes a record",
      narration:
        "Applying the change set establishes twenty blocks. Nothing is written into a target file; what changes is what the project knows: twenty Dutch blocks now carry a decision a person made, the edition's established share rises to match, and the decisions join the record at the bottom of the graph beside the imports.",
      command: "kapi apply dutch.json",
      expect: ["20 applied"],
      focus: "record",
    },
    {
      id: "log",
      title: "The record is the history",
      narration:
        "The log prints what was recorded about the context: who imported or noted what, when, and from which evidence. The block decisions sit beside it in the same record, and both travel with the project, so a colleague's clone and a server share one history.",
      command: "kapi context log | head -8",
      expect: ["import", "[established]"],
      focus: "record",
    },
    {
      id: "again",
      title: "Converge again",
      narration:
        "The second run reads the twenty decisions, draws on the memory for the rest, and every gated edition is shippable. The graph is the same graph, with more in it: that is the whole mechanism, and it is what an agent, a reviewer and a build all read.",
      command: "kapi up",
      expect: ["every gated scope is shippable"],
      focus: "editions",
    },
  ],
  tryNext: [
    "kapi context search berth",
    "kapi memory stats",
    "kapi status --ship",
    "kapi context log --json | head -40",
  ],
};

export const CONTEXT_LABS: Lab[] = [contextEngine];
