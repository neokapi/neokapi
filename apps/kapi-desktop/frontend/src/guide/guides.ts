// The guided tours Kapi Desktop offers from Help, the Overview's first steps
// and the home's Get started card. Each step points at an element by its
// data-slot; guides.test.ts checks that every selector names a slot the
// frontend draws.

import { t } from "@neokapi/i18n-react/runtime";
import type { Guide } from "@neokapi/ui-primitives";

/** The tour ids, for the places that start one. */
export const GUIDE_IDS = {
  project: "project-tour",
  decide: "decide-tour",
  check: "check-tour",
  languages: "languages-tour",
  assistant: "assistant-tour",
} as const;

/** What a tour can do in the app around it. */
export interface GuideNavigation {
  /** Open a view, as the rail and the tabs do ("map", "context:rules", …). */
  navigate: (view: string) => void;
  /** A project tab is open. */
  hasProject: () => boolean;
}

/** A selector for one data-slot. */
export const slot = (name: string) => `[data-slot="${name}"]`;

/** A selector for the rail's entry for one view. */
export const railItem = (view: string) => `[data-slot="sidebar-item"][data-view="${view}"]`;

/** How long a step waits for a page that reads from the backend, in ms. */
const PAGE_WAIT = 8000;

/** Open the first file on the Overview in the Reader, unless the Reader is already open. */
function openFirstFile(): void {
  if (document.querySelector(slot("reader"))) return;
  document.querySelector<HTMLElement>(slot("overview-file-open"))?.click();
}

/** Every tour, built with the app's navigation so a step can open its page. */
export function buildGuides({ navigate, hasProject }: GuideNavigation): Guide[] {
  const needsProject = () =>
    hasProject()
      ? ({ ok: true } as const)
      : ({
          ok: false,
          reason: t("Open a project first.", "why a guided tour cannot start"),
        } as const);

  return [
    {
      id: GUIDE_IDS.project,
      title: t("Find your way around a project", "a guided tour"),
      summary: t(
        "The Overview, its counts and lists, and the places on the left.",
        "a guided tour",
      ),
      available: needsProject,
      topic: "overview",
      steps: [
        {
          id: "counts",
          target: slot("overview-counts"),
          // A sample that was just created takes a moment to open.
          waitFor: 20000,
          before: () => navigate("map"),
          placement: "bottom",
          title: t("Four counts", "tour step title"),
          body: t(
            "A project opens on its Overview. The counts at the top are its files, blocks, rules and findings. Click a count to see what it counts.",
            "tour step",
          ),
        },
        {
          id: "steps",
          target: slot("overview-steps"),
          placement: "bottom",
          title: t("First steps", "tour step title"),
          body: t(
            "A new project lists the steps that set it up. Click a step to open the place that does it. A check marks each step you have done.",
            "tour step",
          ),
        },
        {
          id: "suggestions",
          target: slot("overview-suggestions"),
          placement: "top",
          title: t("Suggestions", "tour step title"),
          body: t(
            "Your assistants' suggestions wait here until you decide on them. **Decide** on a row opens its question.",
            "tour step",
          ),
        },
        {
          id: "files",
          target: slot("overview-files"),
          placement: "top",
          title: t("Files", "tour step title"),
          body: t(
            "Each content file has a row, with the files that have the most findings first. Click a row to read the file in the Reader.",
            "tour step",
          ),
        },
        {
          id: "context",
          target: railItem("context"),
          placement: "right",
          title: t("Context", "tour step title"),
          body: t(
            "**Context** holds Decide, the voice profile, the terms and the content memory of this project.",
            "tour step",
          ),
        },
        {
          id: "review",
          target: railItem("review"),
          placement: "right",
          title: t("Review", "tour step title"),
          body: t(
            "**Review** lists translated blocks one language at a time, for you to approve or reject.",
            "tour step",
          ),
        },
        {
          id: "settings",
          target: railItem("project-settings"),
          placement: "right",
          title: t("Project Settings", "tour step title"),
          body: t(
            "**Project Settings** holds the languages, the content collections, the flows, the assistants and sharing.",
            "tour step",
          ),
        },
        {
          id: "help",
          target: slot("sidebar-help"),
          placement: "right",
          title: t("Help", "tour step title"),
          body: t(
            "Click **Help** at any time for the other tours and a short page on each part of the app.",
            "tour step",
          ),
        },
      ],
    },
    {
      id: GUIDE_IDS.decide,
      title: t("Decide what becomes a rule", "a guided tour"),
      summary: t("Answer a suggestion with Keep or Not a rule.", "a guided tour"),
      available: needsProject,
      topic: "decide",
      steps: [
        {
          id: "list",
          target: slot("decide-item"),
          waitFor: PAGE_WAIT,
          before: () => navigate("context:rules"),
          placement: "right",
          title: t("One row per suggestion", "tour step title"),
          body: t(
            "Each row is something an assistant suggested, such as a name spelled two ways or a word to avoid. Select a row to see its question.",
            "tour step",
          ),
        },
        {
          id: "question",
          target: slot("decide-question"),
          placement: "right",
          title: t("The question", "tour step title"),
          body: t(
            "Each suggestion comes with one question. When it offers more than one option, pick the one the project uses.",
            "tour step",
          ),
        },
        {
          id: "consequence",
          target: slot("decide-consequence"),
          optional: true,
          waitFor: 1500,
          placement: "bottom",
          title: t("What keeping it does", "tour step title"),
          body: t("Here you see what changes if you keep the option you picked.", "tour step"),
        },
        {
          id: "evidence",
          target: slot("decide-evidence"),
          optional: true,
          waitFor: 1500,
          placement: "top",
          title: t("Seen in", "tour step title"),
          body: t(
            "Under **Seen in** is the sentence the suggestion came from, with its file and the assistant that recorded it.",
            "tour step",
          ),
        },
        {
          id: "answer",
          target: slot("decide-actions"),
          placement: "top",
          title: t("Keep or Not a rule", "tour step title"),
          body: t(
            "**Keep** (`k`) makes it a rule of the project. **Not a rule** (`n`) sets it aside, and **Later** (`l`) leaves it for another time. The next question opens after you answer.",
            "tour step",
          ),
        },
      ],
    },
    {
      id: GUIDE_IDS.check,
      title: t("Check the files and fix a finding", "a guided tour"),
      summary: t("Run the checks, open a file in the Reader and fix what failed.", "a guided tour"),
      available: needsProject,
      topic: "checks",
      steps: [
        {
          id: "run",
          target: slot("overview-run"),
          waitFor: PAGE_WAIT,
          before: () => navigate("map"),
          placement: "bottom",
          title: t("Run on everything", "tour step title"),
          body: t(
            "**Run on everything** checks every file against the project's rules. The counts refresh when it finishes.",
            "tour step",
          ),
        },
        {
          id: "findings",
          target: `${slot("overview-counts")} > ${slot("stat")}:last-child`,
          placement: "bottom",
          title: t("Findings", "tour step title"),
          body: t(
            "This count is what the last check found. Click it to see every finding in one list.",
            "tour step",
          ),
        },
        {
          id: "open-file",
          target: slot("overview-file-open"),
          placement: "top",
          advanceOn: "target",
          title: t("Open a file", "tour step title"),
          body: t("Click a file to open it in the Reader. Next opens the first file.", "tour step"),
        },
        {
          id: "finding",
          target: slot("reader-finding"),
          optional: true,
          waitFor: PAGE_WAIT,
          before: openFirstFile,
          placement: "bottom",
          title: t("A finding", "tour step title"),
          body: t(
            "The Reader shows the file's blocks in order. A finding is marked on its words, with a row under the block that shows what failed.",
            "tour step",
          ),
        },
        {
          id: "fix",
          target: slot("reader-fix"),
          optional: true,
          waitFor: 1500,
          placement: "left",
          title: t("Fix", "tour step title"),
          body: t(
            "**Fix** appears when the rule has a replacement. Click it to write the replacement into the file. The Reader then shows the file as it is now.",
            "tour step",
          ),
        },
      ],
    },
    {
      id: GUIDE_IDS.languages,
      title: t("Translate into another language", "a guided tour"),
      summary: t(
        "Add a language, see what a run would do, and bring it up to date.",
        "a guided tour",
      ),
      available: needsProject,
      topic: "languages",
      steps: [
        {
          id: "translate-into",
          target: slot("languages-section"),
          waitFor: PAGE_WAIT,
          before: () => navigate("project-settings"),
          placement: "bottom",
          title: t("Translate into", "tour step title"),
          body: t(
            "Add languages under **Translate into**. **Source language** is the language your files are written in. Click **Save Changes** at the bottom of the window to keep them.",
            "tour step",
          ),
        },
        {
          id: "standing",
          target: slot("language-standing"),
          waitFor: PAGE_WAIT,
          before: () => navigate("map"),
          placement: "bottom",
          title: t("Each language on the Overview", "tour step title"),
          body: t(
            "Once the project translates into a language, the Overview shows how far each language has caught up.",
            "tour step",
          ),
        },
        {
          id: "plan",
          target: slot("hero-plan"),
          placement: "bottom",
          title: t("Plan…", "tour step title"),
          body: t(
            "**Plan…** shows the pending work for each language before anything runs. Nothing is written and no provider is called.",
            "tour step",
          ),
        },
        {
          id: "bring-up-to-date",
          target: slot("hero-bring-up-to-date"),
          placement: "bottom",
          title: t("Bring up to date", "tour step title"),
          body: t(
            "**Bring up to date** reuses earlier translations, translates the rest, and holds back for **Review** what needs a person. The run opens in its own view.",
            "tour step",
          ),
        },
      ],
    },
    {
      id: GUIDE_IDS.assistant,
      title: t("Connect an assistant", "a guided tour"),
      summary: t(
        "Connect Claude Code or another assistant, and see what it records.",
        "a guided tour",
      ),
      available: needsProject,
      topic: "assistants",
      steps: [
        {
          id: "hosts",
          target: slot("assistants-hosts"),
          waitFor: PAGE_WAIT,
          before: () => navigate("project-settings:assistants"),
          placement: "bottom",
          title: t("Assistants", "tour step title"),
          body: t(
            "Each assistant kapi can connect has a row, such as Claude Code or Cursor. A connected one reads **Connected**.",
            "tour step",
          ),
        },
        {
          id: "connect",
          target: slot("assistant-host-connect"),
          placement: "left",
          title: t("Connect", "tour step title"),
          body: t(
            "**Connect** writes the files that assistant reads, the same files `kapi connect` writes. The assistant then reads the project's rules each time it starts.",
            "tour step",
          ),
        },
        {
          id: "reads",
          target: slot("assistants-reads"),
          placement: "bottom",
          title: t("Show what it reads", "tour step title"),
          body: t(
            "**Show what it reads** opens the section kapi writes into `AGENTS.md` and `CLAUDE.md`, and the full brief for any file you choose.",
            "tour step",
          ),
        },
        {
          id: "sessions",
          target: slot("assistants-sessions"),
          placement: "top",
          title: t("Sessions", "tour step title"),
          body: t(
            "Each session you or an assistant worked in has a row. What an assistant suggested waits for you on the Overview and in **Decide**.",
            "tour step",
          ),
        },
      ],
    },
  ];
}
