// Series 2, "Add languages": the multilingual loop, in the Compass sample.
//
// Compass is the Northsea product whose interface strings the first series
// governs, with three target languages and a deployable page whose language
// picker reads what the loop ships. The labs follow samples/compass/README.md:
// one more axis, recycle before AI, review moves the edge, and the hand-off.

import type { Lab } from "../types.ts";

const DOCS = {
  loop: { label: "The kapi loop", href: "/kapi/convergence" },
  ci: { label: "The kapi loop in CI", href: "/kapi/convergence-in-ci" },
  addLanguages: { label: "Add languages", href: "/kapi/get-started/add-languages" },
  review: { label: "Review and approve", href: "/kapi/recipes/review-and-approve" },
  gates: { label: "Ship gates and CI", href: "/kapi/recipes/ship-gates-and-ci" },
  memory: {
    label: "Reuse what you've translated",
    href: "/kapi/recipes/pre-translate-from-memory",
  },
  provider: { label: "Choose a model", href: "/kapi/recipes/choose-a-translation-provider" },
  bilingual: { label: "Hand off to translators", href: "/kapi/bilingual-workflow" },
  sharing: { label: "Sharing context", href: "/kapi/context-portability" },
};

const DECIDE_NL = `kapi status --review --json --jq '.pending[]|select(.locale=="nl" and (.target|startswith("⟦")|not))|{op:"decide",at:{doc:.relative,block:.key,edition:.locale},if_match:"*",outcome:"establish"}' > dutch.json`;

const DECIDE_NB = `kapi status --review --json --jq '.pending[]|select(.locale=="nb")|{op:"decide",at:{doc:.relative,block:.key,edition:.locale},if_match:"*",outcome:"establish"}' > norwegian.json`;

export const compassAxis: Lab = {
  id: "compass-axis",
  series: "add-languages",
  position: 1,
  title: "One more axis",
  tagline: "The same content, the same point, and three languages added.",
  summary:
    "Compass ships in English at four North Sea terminals, and its operators read Norwegian, German and Dutch. The recipe adds one line to what the first series governed. This lab reads it, reads the context in, and looks at where every language stands before the loop has run.",
  sample: "compass",
  concepts: ["target languages", "lifecycle ladder", "ship gate", "ship states", "review record"],
  docs: [DOCS.addLanguages, DOCS.loop, DOCS.gates],
  minutes: 5,
  chapters: [
    {
      id: "recipe",
      title: "One line makes it multilingual",
      narration:
        "Compass is the product whose interface strings the monolingual sample governs. The recipe adds target_languages and two gates. The point, the voice and the vocabulary are the ones already in force; the catalogs sit where the site loads them from, so the loop writes straight into the deployed tree.",
      command: "cat kapi.yaml",
      expect: ["target_languages: [nb, de, nl]", "ship_gate:"],
      notice: ["target_languages", "ship_gate", "established_gate"],
      look: { file: "kapi.yaml", view: "raw" },
    },
    {
      id: "import",
      title: "Read the context in",
      narration:
        "The context arrives as files again, with two more kinds beside the voice and the terms: approved wording per language, which the loop recycles, and the review record, who approved what and of which text.",
      command: "kapi store import ./context",
      expect: ["54 recorded decisions", "content-memory entries"],
    },
    {
      id: "status",
      title: "Both axes on one screen",
      narration:
        "The source gained a Tide window panel last week, four strings, and no language carries it. That is ordinary work rather than a failure, and it is enough to hold every language behind the ship gate.",
      command: "kapi status",
      expect: ["nb/compass-app", "blocked"],
      notice: ["blocked"],
    },
    {
      id: "point",
      title: "The point did not move",
      narration:
        "Ask the source catalog where it sits and the answer is the one the monolingual sample gives for the same strings: the northsea profile, the app channel, the voice in force. The voice governs the translations too, which is what keeps vessel from becoming ship on the way into another language.",
      command: "kapi context site/locales/en-GB.json | head -6",
      expect: ["northsea"],
    },
    {
      id: "ladder",
      title: "The lifecycle ladder",
      narration:
        "A translation climbs a ladder: draft, translated, approved. Norwegian is translated and mostly approved, German mid-review, Dutch half done and unreviewed. Nothing here is stored as a flag; every number is derived from the files and the record.",
      command: "kapi status --locale nb",
      expect: ["nb/compass-app"],
      notice: ["approved"],
    },
    {
      id: "ship",
      title: "What the site may offer",
      narration:
        "The ship manifest answers one question per language: may a reader be offered it. Nothing is offered yet. A catalog being present is not a decision; the gate is.",
      command: "kapi status --ship",
      expect: ['"shippable":false'],
      look: { file: "site/ship.json", view: "raw" },
    },
    {
      id: "picker",
      title: "The picker reads one file",
      narration:
        "The deployed page builds its language picker from ship.json and nothing else: it offers what is shippable and marks what is translated but not yet established.",
      command: "grep -n 'ship.json|established|AI' site/language-picker.js | head -8",
      expect: ["ship.json"],
      look: { file: "site/language-picker.js", view: "raw" },
    },
  ],
  tryNext: [
    "kapi context site/locales/en-GB.json --explain",
    "kapi status --review | head -12",
    "cat site/locales/nl.json",
  ],
};

export const compassConverge: Lab = {
  id: "compass-converge",
  series: "add-languages",
  position: 2,
  title: "Recycle first, draft the rest",
  tagline: "One verb catches every language up, and asks a provider only for what is left.",
  summary:
    "kapi up runs the loop: it reuses the wording the project has established, drafts the remainder with the configured provider, checks what it produced, and parks what a machine cannot decide. This lab runs it on Compass with the built-in demo provider and reads what shipped and what parked.",
  sample: "compass",
  setup: ["kapi store import ./context"],
  concepts: ["kapi up", "content memory", "recycle", "demo provider", "parked locale", "ship.json"],
  docs: [DOCS.loop, DOCS.memory, DOCS.provider],
  minutes: 6,
  chapters: [
    {
      id: "up",
      title: "Converge",
      narration:
        "One verb. Approved wording is recycled first; only what is left reaches a provider, here the built-in demo stub, keyless and offline, which marks everything it writes. Thirty-seven of Norwegian's thirty-eight units come back out of the project's own memory. Norwegian and German clear the gate. Dutch parks: one of its units fails the project's bound checks.",
      command: "kapi up",
      expect: ["content memory 37 · AI 1", "parked (needs human)"],
      notice: ["content memory 37", "parked"],
    },
    {
      id: "status",
      title: "Two offered, one held",
      narration:
        "Two languages are offered on translated work. Dutch is held until somebody looks at the unit that failed. Its drafts are safe in the store; what the gate withholds is delivery.",
      command: "kapi status",
      expect: ["2 of 3 scopes ready to ship"],
    },
    {
      id: "emit",
      title: "Write the manifest where the site reads it",
      narration: "The same answer, written to the file the page fetches.",
      command: "kapi status --ship --emit site/ship.json",
      expect: ["wrote site/ship.json"],
    },
    {
      id: "manifest",
      title: "The contract with the delivery edge",
      narration:
        "Per language, two answers: is it safe to offer at all, and is every string in it established or only translated. Dutch also notes that no terms govern it.",
      command: "cat site/ship.json",
      expect: ['"nb": {"shippable":true,"state":"translated"}', '"nl": {"shippable":false'],
      look: { file: "site/ship.json", view: "raw" },
    },
    {
      id: "drafts",
      title: "The stub marks its output",
      narration:
        "The materialized Norwegian catalog carries the recycled wording and, for the four new strings, the stub's marked drafts. Nobody mistakes them for a translation.",
      command: "grep -n '⟦' site/locales/nb.json",
      expect: ["⟦nb⟧"],
      look: { file: "site/locales/nb.json", view: "raw" },
    },
    {
      id: "memory",
      title: "What the recycle step read",
      narration:
        "The project's content memory: approved pairs per language, including what the committed catalogs taught it on this first pass.",
      command: "kapi memory search berth | head -8",
      expect: ["SOURCE", "en-GB →"],
    },
    {
      id: "bar",
      title: "The pre-release bar",
      narration:
        "kapi check --ship names what holds Dutch back: a failing check and no approval yet. Exit 3 in CI, until a review clears it. An ordinary build never runs this.",
      command: "kapi check --ship --gate voice",
      exit: 3,
      expect: ["nl/compass-app", "approved coverage 0%"],
    },
  ],
  tryNext: [
    "kapi status --review --lang nl | head -10",
    "kapi memory search Tidewatch",
    "cat site/locales/nl.json",
  ],
};

export const compassReview: Lab = {
  id: "compass-review",
  series: "add-languages",
  position: 3,
  title: "Review moves the edge",
  tagline: "A review in the morning is why a language is offered in the afternoon.",
  summary:
    "Review is a change set: a decide operation per unit, bound to the wording a person read. This lab approves the Dutch a person wrote, watches the loop unpark the language, establishes the rest of Norwegian, and reads the manifest the picker follows at each step.",
  sample: "compass",
  setup: ["kapi store import ./context", "kapi up"],
  concepts: ["review queue", "decide", "established", "established gate", "context log"],
  docs: [DOCS.review, DOCS.gates, DOCS.ci],
  minutes: 6,
  chapters: [
    {
      id: "queue",
      title: "The review queue",
      narration:
        "The queue names each unit's document, block and language, which is what a decision addresses. Dutch holds twenty units a person wrote and eighteen the stub drafted.",
      command: "kapi status --review --lang nl | head -14",
      expect: ["awaiting review", "nl.json:nav.berths"],
    },
    {
      id: "select",
      title: "Approve what a person wrote",
      narration:
        "A reviewer who read each unit approves it with a decide operation. This filter takes the Dutch a person wrote and leaves what the stub drafted, one operation per unit, written to dutch.json.",
      command: DECIDE_NL,
    },
    {
      id: "count",
      title: "Twenty decisions",
      narration: "Twenty approvals, waiting to be applied.",
      command: "grep -c '\"decide\"' dutch.json",
      expect: ["20"],
      look: { file: "dutch.json", view: "raw" },
    },
    {
      id: "apply",
      title: "On the record",
      narration:
        "Each approval lands in the record as it is made, attributable, and bound to the wording that stood when it landed.",
      command: "kapi apply dutch.json",
      expect: ["20 applied"],
    },
    {
      id: "log",
      title: "The context log",
      narration: "What was recorded, by whom, and when: the record a team shares.",
      command: "kapi context log | head -8",
      expect: ["establish"],
    },
    {
      id: "up",
      title: "Run the loop again",
      narration:
        "A decision unparks the language. The drafts the loop already made are recycled, so the provider is asked for nothing a second time.",
      command: "kapi up",
      expect: ["every gated scope is shippable"],
    },
    {
      id: "ship",
      title: "Dutch is offered",
      narration:
        "Dutch is offered now, in the translated state, because not every string in it is established.",
      command: "kapi status --ship",
      expect: ['"nl": {"shippable":true,"state":"translated"'],
      notice: ["translated"],
    },
    {
      id: "norwegian",
      title: "Establish the rest of Norwegian",
      narration: "Four units remain in Norwegian: the Tide window strings. Approve them.",
      command: DECIDE_NB,
    },
    {
      id: "apply-nb",
      title: "Four more decisions",
      narration: "They land the same way.",
      command: "kapi apply norwegian.json",
      expect: ["4 applied"],
    },
    {
      id: "established",
      title: "No marker on Norwegian",
      narration:
        "The same file moves Norwegian to established, so the picker offers it without the marker. Three languages, two states, none of them configured by hand.",
      command: "kapi status --ship",
      expect: ['"nb": {"shippable":true,"state":"established"}'],
      notice: ["established"],
    },
    {
      id: "bar",
      title: "The bar passes",
      narration: "The pre-release bar that failed before the review passes after it.",
      command: "kapi check --ship --gate voice",
      exit: 0,
      expect: ["PASS"],
    },
  ],
  tryNext: [
    "kapi status --review --lang de | head -10",
    "kapi context log | head -20",
    "kapi status --ship --emit site/ship.json",
  ],
};

export const compassHandoff: Lab = {
  id: "compass-handoff",
  series: "add-languages",
  position: 4,
  title: "Hand off, and carry work in one file",
  tagline: "A bilingual file for a translator, and a parcel for another machine.",
  summary:
    "Not every translation happens inside the loop. kapi extract writes a bilingual file per source and language, pre-filled from the content memory, for a translator or a CAT tool, and kapi merge takes it back. A .kpz carries the whole working state between machines without a server. This lab does both.",
  sample: "compass",
  setup: ["kapi store import ./context", "kapi up"],
  concepts: ["extract", "XLIFF", "merge", ".kpz parcel"],
  docs: [DOCS.bilingual, DOCS.sharing],
  minutes: 4,
  chapters: [
    {
      id: "extract",
      title: "A bilingual file per language",
      narration:
        "kapi extract writes XLIFF, pre-filled from the content memory: every German unit comes back as an exact match, so a translator starts from what the project already established.",
      command: "kapi extract --target-lang de",
      expect: ["format=xliff2", "exact=38"],
    },
    {
      id: "out",
      title: "Where it went",
      narration: "One file per source and language, under out/.",
      command: "ls out",
      expect: ["site-locales-en-GB.en-GB-to-de.xliff"],
    },
    {
      id: "xliff",
      title: "Inside the file",
      narration:
        "Each unit carries its source and target, and the revision of each, which kapi merge checks the returned file against.",
      command: "head -24 out/site-locales-en-GB.en-GB-to-de.xliff",
      expect: ["<xliff", "srcLang"],
      look: { file: "out/site-locales-en-GB.en-GB-to-de.xliff", view: "raw" },
    },
    {
      id: "merge",
      title: "Take it back",
      narration:
        "kapi merge reads the returned file, writes the target catalog, and absorbs the pairs into the content memory, so the next extract starts further along.",
      command: "kapi merge -i out/",
      expect: ["de"],
    },
    {
      id: "pack",
      title: "One file carries the project",
      narration:
        "A .kpz is the whole working state: the recipe, the blocks, every locale's overlays, the memory and the terms. It moves between machines or people without a server.",
      command: "kapi pack -o compass.kpz",
      expect: ["Packed"],
    },
    {
      id: "info",
      title: "What is inside",
      narration: "kapi info reads the parcel without unpacking it.",
      command: "kapi info compass.kpz",
      expect: ["blocks", "memory", "terms"],
    },
  ],
  tryNext: [
    "kapi extract --target-lang nl --format po",
    "kapi memory search berth | head -4",
    "kapi unpack compass.kpz --help",
  ],
};

export const COMPASS_LABS: Lab[] = [compassAxis, compassConverge, compassReview, compassHandoff];
