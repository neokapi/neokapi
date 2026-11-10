// The help topics Kapi Desktop shows in its Help panel: a short page on each
// part of the app, read without leaving it, with a link to the website page
// that has the full story.

import { t } from "@neokapi/i18n-react/runtime";
import type { HelpTopic } from "@neokapi/ui-primitives";
import { GUIDE_IDS } from "./guides";

/** The topic ids, for the places that open one. */
export const TOPIC_IDS = {
  desktop: "kapi-desktop",
  overview: "overview",
  decide: "decide",
  checks: "checks",
  reader: "reader",
  languages: "languages",
  voiceTerms: "voice-and-terms",
  assistants: "assistants",
  bowrain: "bowrain",
  commandLine: "command-line",
} as const;

const KAPI_DOCS = "https://neokapi.github.io/kapi";

/** Paragraphs and lists, each translated on its own, joined as Markdown. */
const md = (...parts: string[]) => parts.join("\n\n");
const list = (...items: string[]) => items.map((item) => `- ${item}`).join("\n");
const heading = (text: string) => `### ${text}`;

/** Every help topic, in the order the Help panel lists them. */
export function buildTopics(): HelpTopic[] {
  return [
    {
      id: TOPIC_IDS.desktop,
      title: t("What Kapi Desktop does", "a help topic"),
      summary: t("The app, its places, and where to start.", "a help topic"),
      guides: [GUIDE_IDS.project, GUIDE_IDS.decide, GUIDE_IDS.assistant],
      moreUrl: `${KAPI_DOCS}/desktop/overview`,
      body: md(
        t(
          "Kapi Desktop opens a folder of content files as a project. It reads the files, checks them against the project's rules, and shows what your assistants suggested so you can decide what becomes a rule.",
          "help topic text",
        ),
        heading(t("The places on the left", "help topic heading")),
        list(
          t(
            "**Home** lists every project kapi has run in, and what is new in each.",
            "help topic list item",
          ),
          t(
            "**Overview** is the page a project opens on: its counts, suggestions and files.",
            "help topic list item",
          ),
          t(
            "**Context** holds Decide, the voice profile, the terms and the content memory.",
            "help topic list item",
          ),
          t(
            "**Review** lists translated blocks for you to approve or reject.",
            "help topic list item",
          ),
          t(
            "**Project Settings** holds the languages, content, flows, assistants and sharing.",
            "help topic list item",
          ),
        ),
        heading(t("Where to start", "help topic heading")),
        t(
          "Open a folder, or click **Try the sample** on the home to open the KapiMart sample. Then take one of the tours below.",
          "help topic text",
        ),
        t(
          "If you hid **Get started** on the home, open **App Settings** and click **Show it again** beside **Get started on the home**.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.overview,
      title: t("The Overview", "a help topic"),
      summary: t("The page a project opens on.", "a help topic"),
      guides: [GUIDE_IDS.project],
      moreUrl: `${KAPI_DOCS}/desktop/tour`,
      body: md(
        t(
          "The Overview shows when kapi last read the project's files, and what waits for you.",
          "help topic text",
        ),
        list(
          t(
            "**The counts**: files, blocks, rules and findings. Click a count to see what it counts.",
            "help topic list item",
          ),
          t(
            "**First steps**: on a new project, the steps that set it up. Click a step to open the place that does it.",
            "help topic list item",
          ),
          t(
            "**Suggestions**: what your assistants suggested and nobody has decided. **Decide** on a row opens its question.",
            "help topic list item",
          ),
          t(
            "**Files**: one row per content file, the files with the most findings first. Click a row to open it in the Reader.",
            "help topic list item",
          ),
        ),
        t(
          "**Run on everything** checks every file again. **Deeper look with AI** reads the files with your connected model and adds what it finds to **Suggestions**.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.decide,
      title: t("Suggestions and Decide", "a help topic"),
      summary: t("Turn what an assistant noticed into a rule, or set it aside.", "a help topic"),
      guides: [GUIDE_IDS.decide],
      moreUrl: `${KAPI_DOCS}/desktop/recipes/review-your-assistants-suggestions`,
      body: md(
        t(
          "An assistant that works in the project records what it finds: a name spelled two ways, a word to avoid, a note about tone. Each one is a suggestion until you decide on it.",
          "help topic text",
        ),
        t(
          "Open **Decide** from the Overview, or from **Context**. Select a row and the pane beside it shows one question, the same question `kapi rules` asks.",
          "help topic text",
        ),
        list(
          t("**Keep** (`k`) makes it a rule of the project.", "help topic list item"),
          t("**Not a rule** (`n`) sets it aside.", "help topic list item"),
          t("**Later** (`l`) leaves it for another time.", "help topic list item"),
          t("**Other…** lets you write a form of your own.", "help topic list item"),
        ),
        t(
          "Under **Seen in** is the sentence the suggestion came from, with its file and the assistant that recorded it. A kept rule applies to the next check and to every assistant that reads the project.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.checks,
      title: t("Checks and findings", "a help topic"),
      summary: t("What a check reads, and what a finding means.", "a help topic"),
      guides: [GUIDE_IDS.check],
      moreUrl: `${KAPI_DOCS}/recipes/verify-content`,
      body: md(
        t(
          "A check reads every block of every file and compares it with the project's rules: its terms, its voice profile and the rules you kept in Decide. Each place a block breaks a rule is a finding.",
          "help topic text",
        ),
        list(
          t(
            "**Run on everything** on the Overview checks the whole project.",
            "help topic list item",
          ),
          t("The **findings** count opens every finding in one list.", "help topic list item"),
          t(
            "Each file on the Overview shows its own findings. Open it to see them in place.",
            "help topic list item",
          ),
        ),
        t(
          "A word rule fails the check unless it is marked advisory. A tone rule only reports until someone marks it to fail.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.reader,
      title: t("Fixing in the Reader", "a help topic"),
      summary: t("Read a file with its findings, and fix them.", "a help topic"),
      guides: [GUIDE_IDS.check],
      moreUrl: `${KAPI_DOCS}/get-started/first-project`,
      body: md(
        t(
          "The Reader shows one file's blocks in file order. Each finding is marked on its words, with a row under the block. The rules that hold at the file run down the margin.",
          "help topic text",
        ),
        list(
          t(
            "**Fix** beside a finding replaces the words with the rule's wording.",
            "help topic list item",
          ),
          t(
            "**Fix 2 in this file** (with the file's own count) fixes every finding in the file at once.",
            "help topic list item",
          ),
        ),
        t(
          "The rest of the file stays as it was. Findings about tone or style, and findings on words that hold a link or other formatting, have no **Fix**. Edit those words yourself.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.languages,
      title: t("Languages and Bring up to date", "a help topic"),
      summary: t("Translate a project and keep each language caught up.", "a help topic"),
      guides: [GUIDE_IDS.languages],
      moreUrl: `${KAPI_DOCS}/get-started/add-languages`,
      body: md(
        heading(t("Add a language", "help topic heading")),
        t(
          "Open **Project Settings**, add the language under **Translate into**, and click **Save Changes**.",
          "help topic text",
        ),
        heading(t("Bring it up to date", "help topic heading")),
        t("The Overview then shows how far each language has caught up.", "help topic text"),
        list(
          t(
            "**Plan…** shows the pending work for each language. Nothing is written and no provider is called.",
            "help topic list item",
          ),
          t(
            "**Bring up to date** reuses earlier translations from the content memory, translates the rest, and holds back for **Review** what needs a person.",
            "help topic list item",
          ),
        ),
        t(
          "A language that is behind is pending work, and it never breaks a build.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.voiceTerms,
      title: t("Voice profiles and terms", "a help topic"),
      summary: t("How the project should sound, and the words it uses.", "a help topic"),
      moreUrl: `${KAPI_DOCS}/recipes/keep-source-on-brand`,
      body: md(
        t("Both live under **Context**.", "help topic text"),
        heading(t("Voice", "help topic heading")),
        t(
          "**Voice** shows the voice profile that applies: tone, style rules, pattern rules, examples, and the adjustments for a language, channel or persona. Editing it writes the YAML file in your project, the same file a reviewer reads in a pull request.",
          "help topic text",
        ),
        heading(t("Terms", "help topic heading")),
        t(
          "**Terms** lists the words decided for the project, one concept per row. Select a concept to see its definition, the forms to avoid with what to write instead, and its terms in each language. **Relate** links it to another concept.",
          "help topic text",
        ),
        t(
          "**Content Memory**, beside them, holds approved wording one pair per row: a source sentence and its translation.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.assistants,
      title: t("Connecting an assistant", "a help topic"),
      summary: t("Give Claude Code or another assistant the project's rules.", "a help topic"),
      guides: [GUIDE_IDS.assistant],
      moreUrl: `${KAPI_DOCS}/desktop/recipes/connect-an-assistant`,
      body: md(
        t(
          "Open **Project Settings** and the **Assistants** tab. Each assistant kapi can connect has a row.",
          "help topic text",
        ),
        list(
          t(
            "**Connect** writes the files that assistant reads, the same files `kapi connect` writes.",
            "help topic list item",
          ),
          t(
            "**Show what it reads** opens the section kapi writes into `AGENTS.md` and `CLAUDE.md`, and the full brief for any file.",
            "help topic list item",
          ),
          t(
            "**Sessions** lists what you and the assistants recorded, one session per row.",
            "help topic list item",
          ),
        ),
        t(
          "What a connected assistant suggests appears on the Overview under **Suggestions**, and on the home under **What's new**. You decide on it in **Decide**.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.bowrain,
      title: t("kapi and Bowrain", "a help topic"),
      summary: t("Share a project's context beyond this Mac.", "a help topic"),
      moreUrl: "https://bowrain.cloud/docs/getting-started/kapi-vs-bowrain",
      body: md(
        t(
          "kapi and Kapi Desktop work on your machine. The rules, terms, voice profiles and content memory stay in your project and your workspace.",
          "help topic text",
        ),
        t(
          "**Share**, a tab of **Project Settings**, shows how far the project's rules reach, as three rungs:",
          "help topic text",
        ),
        list(
          t(
            "**On this Mac**: every clone and worktree on this machine reads them.",
            "help topic list item",
          ),
          t(
            "**With your team**: a shared folder or git carries them to every machine that syncs the project.",
            "help topic list item",
          ),
          t(
            "**Managed on Bowrain**: a team queue, who kept what, and the same screens in a browser.",
            "help topic list item",
          ),
        ),
        t(
          "To connect a project to a Bowrain server, install the `kapi-bowrain` plugin and run `kapi share --via bowrain <server-url>` in the project.",
          "help topic text",
        ),
      ),
    },
    {
      id: TOPIC_IDS.commandLine,
      title: t("The same work on the command line", "a help topic"),
      summary: t("The kapi command for each thing you do in the app.", "a help topic"),
      moreUrl: `${KAPI_DOCS}/get-started/cli-vs-desktop`,
      body: md(
        t(
          "Kapi Desktop and the `kapi` command work on the same project files, so you can switch between them.",
          "help topic text",
        ),
        list(
          t("**Run on everything**: `kapi check`", "help topic list item"),
          t("**Fix**: `kapi fix`", "help topic list item"),
          t("**Decide**: `kapi rules`", "help topic list item"),
          t("The rules that hold at a file: `kapi context <path>`", "help topic list item"),
          t("**Connect** an assistant: `kapi connect`", "help topic list item"),
          t("**Bring up to date**: `kapi up`", "help topic list item"),
          t("**Share**: `kapi share`", "help topic list item"),
        ),
        t("Run `kapi --help` in a project for the full list.", "help topic text"),
      ),
    },
  ];
}
