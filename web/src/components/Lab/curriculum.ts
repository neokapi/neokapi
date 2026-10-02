export type LabMode = "browser" | "recorded" | "mixed";

export interface LabLessonDefinition {
  id: string;
  title: string;
  stage: "framework" | "kapi";
  question: string;
  outcome: string;
  mode: LabMode;
}

export const LAB_MODES: Record<LabMode, { label: string; description: string }> = {
  browser: {
    label: "Live browser experiment",
    description: "Open the experiment when you are ready. Its engine and assets load on demand.",
  },
  recorded: {
    label: "Recorded native experiment",
    description:
      "Inspect saved native runs and their evidence. Changing a recorded case selects an existing result.",
  },
  mixed: {
    label: "Browser experiment and recorded evidence",
    description:
      "Run the supported browser exercise and compare it with the labelled native evidence.",
  },
};

export const LAB_LESSONS: readonly LabLessonDefinition[] = [
  {
    id: "representing-content",
    title: "Representing content",
    stage: "framework",
    question: "What does the engine read from a document?",
    outcome: "Locate text, inline content and document structure in Parts, Blocks and Runs.",
    mode: "browser",
  },
  {
    id: "editing-with-fidelity",
    title: "Editing with fidelity",
    stage: "framework",
    question: "What survives an edit or a change of format?",
    outcome:
      "Distinguish text changes, structural preservation and the limits of a destination format.",
    mode: "browser",
  },
  {
    id: "tools-and-annotations",
    title: "Tools and annotations",
    stage: "framework",
    question: "What does a tool change, and what does it report?",
    outcome: "Compare transformations with checks and inspect the annotations attached to content.",
    mode: "browser",
  },
  {
    id: "segmentation",
    title: "Segmentation",
    stage: "framework",
    question: "Where should a sentence boundary fall?",
    outcome:
      "Compare boundaries against an explained reference and distinguish agreement from correctness.",
    mode: "browser",
  },
  {
    id: "composing-flows",
    title: "Composing flows",
    stage: "framework",
    question: "How does the order of tools affect a processing run?",
    outcome:
      "Follow intermediate content through a flow and relate the result to its configuration.",
    mode: "browser",
  },
  {
    id: "checks-and-coverage",
    title: "Checks and coverage",
    stage: "framework",
    question: "What does a passing check establish?",
    outcome: "Separate findings from coverage and identify claims a literal check cannot assess.",
    mode: "mixed",
  },
  {
    id: "kapi-project",
    title: "From files to a kapi project",
    stage: "kapi",
    question: "What persists between processing runs?",
    outcome:
      "Connect a recipe, its source files, the working state and the output produced by a run.",
    mode: "browser",
  },
  {
    id: "context-and-reuse",
    title: "Context and reuse",
    stage: "kapi",
    question: "Why does this guidance apply to this content?",
    outcome:
      "Inspect resolved guidance and distinguish contextual applicability from content reuse.",
    mode: "recorded",
  },
  {
    id: "decisions-and-change",
    title: "Decisions and change",
    stage: "kapi",
    question: "How does a context decision change subsequent checks?",
    outcome:
      "Follow a suggestion through acceptance and reversal, using the recorded check results as evidence.",
    mode: "recorded",
  },
  {
    id: "context-informed-ai",
    title: "Context-informed AI",
    stage: "kapi",
    question: "What changes when an agent receives applicable context?",
    outcome:
      "Compare prompts, inspected sources and generated documents while keeping human judgement explicit.",
    mode: "recorded",
  },
];

export const LAB_ELECTIVES = [
  {
    to: "/lab/structure",
    title: "Structure and layout",
    description: "Inspect reading order and geometry recovered from a PDF.",
  },
  {
    to: "/lab/vision",
    title: "Vision",
    description: "Examine text recognition and layout on images and scans.",
  },
  {
    to: "/lab/media",
    title: "Audio and video",
    description: "Inspect speech recognition and text associated with a time or frame.",
  },
  {
    to: "/kbf-lab",
    title: "KBF anatomy",
    description: "Read the bundle representation and inspect its serialization.",
  },
  {
    to: "/lab/convert",
    title: "File conversion",
    description: "Explore destination formats after the fidelity lesson.",
  },
] as const;

export function lessonPath(lesson: LabLessonDefinition): string {
  return `/lab/${lesson.id}`;
}
