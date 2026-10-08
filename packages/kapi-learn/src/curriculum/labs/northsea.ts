// Series 1, "Start here": one language, in the Northsea sample.
//
// Northsea Maritime Systems keeps operator documentation, interface strings
// and a marketing page in one repository, in one language. The four labs
// follow the sample's own journey (samples/northsea/README.md): what a
// project is, what governs each file, the gate, and an edit through the
// engine. Every chapter is verified by scripts/learn-verify.

import type { Lab } from "../types.ts";

const DOCS = {
  projects: { label: "Projects", href: "/kapi/projects" },
  projectFile: { label: "The project file", href: "/reference/project-file" },
  contentModel: { label: "The content model", href: "/framework/content-model" },
  context: { label: "Context", href: "/kapi/context" },
  growing: { label: "Growing context", href: "/kapi/context-decisions" },
  verify: { label: "Check content like tests", href: "/kapi/recipes/verify-content" },
  loop: { label: "The kapi loop", href: "/kapi/convergence" },
  edit: { label: "Edit content", href: "/kapi/recipes/rewrite-content" },
  change: {
    label: "The change contract",
    href: "/contribute/architecture/engine/e-09-the-change-contract",
  },
  toolbox: { label: "The CLI text tools", href: "/toolbox/overview" },
};

export const northseaProject: Lab = {
  id: "northsea-project",
  series: "start-here",
  position: 1,
  title: "A project and its files",
  tagline: "What kapi reads, and how it addresses every piece of content.",
  summary:
    "A kapi project is a recipe beside the files it governs. This lab opens the Northsea repository, reads its recipe, lists the files kapi claims, and looks at how the engine turns a Markdown page and a JSON catalog into the same thing: blocks, each with an address.",
  sample: "northsea",
  concepts: ["project", "recipe", "collection", "point", "block", "format"],
  docs: [DOCS.projects, DOCS.projectFile, DOCS.contentModel],
  minutes: 4,
  chapters: [
    {
      id: "tree",
      title: "A company repository",
      narration:
        "Northsea Maritime Systems keeps its operator documentation, the strings of its Compass interface and one marketing page in one repository, in one language. The recipe at the root, kapi.yaml, says which of these files are content.",
      command: "ls",
      expect: ["kapi.yaml", "docs/"],
      look: { file: "kapi.yaml", view: "raw" },
    },
    {
      id: "recipe",
      title: "The recipe",
      narration:
        "Three collections name files by pattern and bind each to a point: northsea/docs, northsea/app and northsea/landing. The API reference and the changelog sit inside the documentation collection and take a point of their own, because a published record reads differently from a procedure.",
      command: "cat kapi.yaml",
      expect: ["collections:", "northsea/reference"],
      notice: ["channel:"],
      note: "A point is where content sits in the project: a product and a channel. What governs content is bound at the point, so two files at the same point are held to the same voice.",
    },
    {
      id: "ls",
      title: "The files kapi reads",
      narration:
        "kapi ls is the recipe applied to the tree: every file a collection claims, with the format it is read in. A file no pattern matches is not content.",
      command: "kapi ls",
      expect: ["7 file(s)", "markdown", "html"],
    },
    {
      id: "stats",
      title: "Measure a file",
      narration:
        "Measure before anything is changed or translated. A block is one piece of content: a paragraph, a heading, a list item. Segments are the sentences inside the blocks.",
      command: "kapi stats docs/berths.md",
      expect: ["Blocks:", "Words:"],
    },
    {
      id: "inspect",
      title: "Every block has an address",
      narration:
        "kapi inspect prints the blocks the reader found. Each carries a ref, the document and the block's place in it, and a rev, the revision of its text. Every later verb, from a check finding to an edit, names content this way.",
      command: "kapi inspect docs/berths.md | head -40",
      expect: ['"doc": "docs/berths.md"', '"rev":'],
      look: { file: "docs/berths.md", view: "blocks" },
    },
    {
      id: "keyed",
      title: "Keyed formats",
      narration:
        "In a catalog the key is the block's name: nav.plan, plan.title. The same model holds a Markdown paragraph and a JSON string, so one check, one edit and one translation step serve both.",
      command: "kapi inspect app/strings.en.json | head -24",
      expect: ['"block": "nav.plan"'],
      look: { file: "app/strings.en.json", view: "blocks" },
    },
    {
      id: "formats",
      title: "Every format, one model",
      narration:
        "The reader for each format parses into this model, and its writer puts the content back around the text that changed, byte for byte. The list is the engine's own: what it reads, what it writes, and how faithfully.",
      command: "kapi formats",
      expect: ["markdown", "openxml"],
      note: "The format reference pages carry the evidence behind each row.",
    },
  ],
  tryNext: [
    "kapi stats landing/index.html",
    "kapi inspect docs/changelog.md",
    "kapi inspect app/strings.en.json --json | head -30",
  ],
};

export const northseaContext: Lab = {
  id: "northsea-context",
  series: "start-here",
  position: 2,
  title: "Context: what applies here",
  tagline: "Two questions a writer or an agent asks before touching a file.",
  summary:
    "A project's context is what its writing keeps to: the voice, the words to use and to avoid, and the decisions behind them. This lab reads the Northsea context into the project and asks the two questions kapi answers from it: where am I and what governs here, and what do we call this.",
  sample: "northsea",
  concepts: ["context", "voice profile", "terms", "point", "occurrence graph", "kapi up"],
  docs: [DOCS.context, DOCS.growing],
  minutes: 6,
  chapters: [
    {
      id: "proposal",
      title: "Context ships as files",
      narration:
        "The project's context arrives as two files: the house voice and the vocabulary, drafted from the content and corrected by the writer who owns it. Until somebody reads them in, they are a proposal, and the gates answer from an empty store.",
      command: "ls context",
      expect: ["voice.yaml", "terms.json"],
      look: { file: "context/voice.yaml", view: "raw" },
    },
    {
      id: "voice",
      title: "The house voice",
      narration:
        "A voice profile holds tone, style and constraints. A channel below it bends the register per surface: instructional in the documentation, warmer on the landing page. The vocabulary stays in force everywhere.",
      command: "head -32 context/voice.yaml",
      expect: ["tone:", "constraints:"],
    },
    {
      id: "import",
      title: "Read the context in",
      narration:
        "One command reads the files into the project's store, where every later answer comes from. The recipe binds the voice by the name the import stores it under, so an edit to the files takes effect when you import again.",
      command: "kapi store import ./context",
      expect: ["7 concepts", "1 voice profile"],
    },
    {
      id: "up",
      title: "Reconcile the sources",
      narration:
        "kapi up reconciles the context and the sources. With no target language it settles the source: every block goes into the project store and the occurrence graph is built, so a word can be found where it is used.",
      command: "kapi up",
      expect: ["Extracted 198 block(s)"],
    },
    {
      id: "where",
      title: "Where am I, and what governs here",
      narration:
        "The first question an assistant asks before it writes. The answer is a brief: the voice to write in, the words to use and to avoid at this point, and what has been suggested but not decided. A person reads it, and a model receives the same text.",
      command: "kapi context docs/berths.md",
      expect: ["Voice: Northsea", 'berth, not "mooring"'],
      look: { file: "docs/berths.md", view: "preview" },
    },
    {
      id: "explain",
      title: "Another point, another register",
      narration:
        "Three directories away, the landing page answers with another point and a warmer register. The same voice, the same terms. The explanation names how the answer was reached: the point, the profile that bound the voice, the revision that was read.",
      command: "kapi context landing/index.html --explain",
      expect: ["northsea/landing", "How this was answered"],
      notice: ["northsea/landing"],
    },
    {
      id: "search",
      title: "What do we call this",
      narration:
        "The second question. Northsea renamed a place alongside from mooring to berth, and the answer carries both halves: the word to say, the retired word that only reports, and the one field on the wire that keeps the old spelling because it is part of a published contract.",
      command: "kapi context search mooring",
      expect: ["deprecated", "mooring_id"],
      notice: ["deprecated", "admitted"],
    },
    {
      id: "search-berth",
      title: "Where a word lands",
      narration:
        "Ask for the preferred word and the graph says where it is used: a count, and every block that carries it, as of the last extraction.",
      command: "kapi context search berth | head -8",
      expect: ["used", "block(s)"],
    },
  ],
  tryNext: [
    "kapi context app/strings.en.json",
    "kapi context search vessel",
    "kapi voice show --profile northsea",
  ],
};

export const northseaChecks: Lab = {
  id: "northsea-checks",
  series: "start-here",
  position: 3,
  title: "Check content like tests",
  tagline: "A gate that runs offline, and a decision that changes what it finds.",
  summary:
    "kapi check runs the project's configured checks the way a test suite runs: it exits 3 on a finding that fails and reports the rest. This lab runs the gate on Northsea, corrects the source where the wording was wrong, records one decision the graph had never been told, and watches the next run enforce it.",
  sample: "northsea",
  setup: ["kapi store import ./context", "kapi up"],
  concepts: [
    "check",
    "finding",
    "fails and reports",
    "exit codes",
    "decision",
    "kapi apply",
    "converge",
  ],
  docs: [DOCS.verify, DOCS.growing, DOCS.loop],
  minutes: 7,
  chapters: [
    {
      id: "gate",
      title: "Run the gate",
      narration:
        "Run the configured checks the way you run tests. One finding fails: a forbidden word on the landing page. Four report: a retired word the documentation still carries, which the terms mark deprecated rather than forbidden. Exit code 3 is the gate: at least one finding fails.",
      command: "kapi check",
      exit: 3,
      expect: ["FAILS", "REPORTS", "seamless", "mooring"],
      notice: ["FAILS", "REPORTS"],
      look: { file: "landing/index.html", view: "preview" },
    },
    {
      id: "json",
      title: "The shape CI reads",
      narration:
        "The same run in machine shape. Each finding carries its rule, whether it fails, the file and block it sits at, and the replacement, which is what a pull-request check and an assistant both read.",
      command:
        "kapi check --json --jq '.findings[] | {rule, fails, file: .location.file, suggestion}'",
      exit: 3,
      expect: ['"fails": true', '"suggestion"'],
    },
    {
      id: "fix-landing",
      title: "Correct the source",
      narration:
        "Three findings are wording the source got wrong, so correct the source. ksed edits the text inside the format: the HTML around the sentence is untouched.",
      command: "ksed -i 's/our seamless integration/our unified integration/' landing/index.html",
    },
    {
      id: "fix-docs",
      title: "The documentation",
      narration: "The same edit in a Markdown list item.",
      command: "ksed -i 's/Release a mooring earlier/Release a berth earlier/' docs/berths.md",
    },
    {
      id: "fix-app",
      title: "The interface strings",
      narration: "And in the catalog, where the placeholder text of a search field said ship.",
      command: "ksed -i 's/by ship name/by vessel name/' app/strings.en.json",
    },
    {
      id: "decision",
      title: "A decision rather than an edit",
      narration:
        "The reviewer also notices a word the graph has never been told about: the testimonial says dock, and Northsea says berth. That is a decision, so it is written as one operation and applied. Forbidden rather than deprecated: dock is a word Northsea does not use, so it should stop a build.",
      files: [
        {
          path: "decisions.jsonl",
          content:
            '{"op":"term","action":"upsert","term":"dock","locale":"en-GB","status":"forbidden","replacement":"berth"}\n',
        },
      ],
      command: "kapi apply decisions.jsonl",
      expect: ["1 applied"],
      look: { file: "decisions.jsonl", view: "raw" },
    },
    {
      id: "recheck",
      title: "The next run enforces it",
      narration:
        "Run the check again and it finds what the previous run passed. One operation reached the record and the gate.",
      command: "kapi check",
      exit: 3,
      expect: ['"dock"'],
      notice: ["dock"],
    },
    {
      id: "fix-quote",
      title: "Fix the last one",
      narration: "The testimonial keeps its meaning and takes the house word.",
      command:
        "ksed -i 's/could not dock when the pilot called/could not reach its berth when the pilot called/' landing/index.html",
    },
    {
      id: "converge",
      title: "Converge",
      narration:
        "Three source files changed, so kapi up extracts them again and the occurrence graph follows.",
      command: "kapi up",
      expect: ["block(s)"],
    },
    {
      id: "status",
      title: "Where the project stands",
      narration:
        "One line for the source axis. A monolingual project has no per-language standing to report.",
      command: "kapi status",
      expect: ["source"],
    },
    {
      id: "pass",
      title: "The gate passes",
      narration:
        "Two findings remain and report: the changelog keeps the retired name, because a past entry says what was announced in the words it was announced in. A retirement never fails a build.",
      command: "kapi check",
      exit: 0,
      expect: ["PASS", "REPORTS"],
      notice: ["PASS"],
    },
    {
      id: "ship",
      title: "The pre-release bar",
      narration:
        "kapi check --ship runs the same gates plus the coverage gates the recipe declares. This is the bar a release tag waits for; an ordinary build never fails on it.",
      command: "kapi check --ship",
      exit: 0,
      expect: ["PASS"],
    },
  ],
  tryNext: [
    "kapi check --json > findings.json",
    "kapi context search dock",
    "kapi check --forbid 'best-in-class' landing/index.html",
  ],
};

export const northseaEdit: Lab = {
  id: "northsea-edit",
  series: "start-here",
  position: 4,
  title: "Edit through the engine",
  tagline: "Read a revision, change one block, and keep every other byte.",
  summary:
    "kapi apply is the one write verb: a change set names a block by its ref and the revision it read, and the format's writer lands the new text with everything around it unchanged. This lab edits the landing page's heading, compares the page before and after, and sees what happens when the same change set is sent twice.",
  sample: "northsea",
  setup: ["kapi store import ./context", "kapi up"],
  concepts: ["change contract", "revision", "if_match", "write-back fidelity", "kdiff"],
  docs: [DOCS.edit, DOCS.change, DOCS.toolbox],
  minutes: 5,
  chapters: [
    {
      id: "read",
      title: "Read before you write",
      narration:
        "Each block prints its ref and the rev of its text. An edit names both, so the engine can tell whether the text moved since you read it.",
      command: "kapi inspect landing/index.html | head -42",
      expect: ['"header/h1"', '"rev": "r:148b50731fa08ef4"'],
      look: { file: "landing/index.html", view: "preview" },
    },
    {
      id: "keep",
      title: "Keep a copy",
      narration: "Keep the page as it is, to compare against afterwards.",
      command: "cat landing/index.html > index.before.html",
    },
    {
      id: "apply",
      title: "One change set, one block",
      narration:
        "A change set is one operation per line. set_content gives the heading new text, at its ref, if the revision still matches. kapi apply lands it through the HTML writer.",
      files: [
        {
          path: "edit.jsonl",
          content:
            '{"op":"set_content","at":{"doc":"landing/index.html","block":"header/h1","edition":"en-GB"},"if_match":"r:148b50731fa08ef4","text":"The port day, on one screen"}\n',
        },
      ],
      command: "kapi apply edit.jsonl",
      expect: ["wrote landing/index.html", "1 applied"],
      look: { file: "landing/index.html", view: "preview" },
    },
    {
      id: "diff",
      title: "What changed, and what did not",
      narration:
        "kdiff compares the text inside the format, block by block: one block changed. The markup, the whitespace and the styles around it are byte for byte what they were. Exit 1 says the two differ.",
      command: "kdiff index.before.html landing/index.html",
      exit: 1,
      expect: ["on one screen"],
    },
    {
      id: "stale",
      title: "Sent twice",
      narration:
        "Send the same change set again and it is refused: the heading moved since the revision it names. The refusal carries the current revision and text, so a writer, or an agent, reads again and resends rather than overwriting what it has not seen.",
      command: "kapi apply edit.jsonl",
      exit: 3,
      expect: ["stale"],
      notice: ["stale"],
    },
    {
      id: "grep",
      title: "Search the text, not the tags",
      narration:
        "kgrep searches the text a reader sees, across the format, so a match in the title and one in the heading come back as two blocks with no markup in the way.",
      command: "kgrep 'port day' landing/index.html",
      expect: ["on one screen"],
    },
  ],
  tryNext: [
    "kapi apply --schema set_content",
    "kapi apply --dry-run edit.jsonl",
    "kapi inspect docs/berths.md --json | head -30",
  ],
};

export const NORTHSEA_LABS: Lab[] = [
  northseaProject,
  northseaContext,
  northseaChecks,
  northseaEdit,
];
