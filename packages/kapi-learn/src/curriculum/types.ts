// The curriculum's shape: series of labs, labs of chapters.
//
// A lab is a short session. A terminal lab runs in a sample project: each
// chapter runs one command in the lab shell against the real engine and says
// what to look at. An explorer lab puts one of the engine explorers on the
// stage instead: each chapter sets what the explorer shows and says what to
// look at. A playground has no chapters: Play opens the terminal or the
// explorer with a few things to try, and the reader takes it from there. The
// player steps through chapters on the reader's say-so; the verifier
// (scripts/learn-verify) runs every terminal chapter in Node and holds the lab
// to the exit codes and output it declares. Pure data: no React, no engine
// imports, so the Docusaurus build can read it to make one page per lab.

/**
 * The sample project a lab runs in; the files come from samples/<id>/. An
 * explorer lab runs on the engine's own fixtures, under "engine".
 */
export type SampleId = "northsea" | "compass" | "mart" | "engine";

export type SeriesId = "start-here" | "add-languages" | "context" | "engine" | "explore";

/**
 * A terminal lab types commands into a shell; an explorer lab drives an
 * explorer; a playground opens either with nothing scripted.
 */
export type LabKind = "terminal" | "explorer" | "playground";

/** A text file written into the sandbox, relative to the lab's working directory. */
export interface LabFile {
  path: string;
  content: string;
}

/** What the files pane shows once a chapter has run. */
export interface ChapterLook {
  /** A sandbox-relative file to open. */
  file?: string;
  /** The reading to open it in (default "preview"; "raw" for a JSON or YAML file). */
  view?: "preview" | "blocks" | "raw";
}

/**
 * What an explorer chapter puts on the stage: the explorer by id (the host
 * resolves it; a lab's `explorer` is the default) and the props the chapter
 * gives it, in the explorer's own logical terms (a sample name, a scenario,
 * a target format). The host maps them to the component's props.
 */
export interface ChapterStage {
  explorer?: string;
  [prop: string]: unknown;
}

export interface Chapter {
  /** Stable, URL-safe id (the deep link names it). */
  id: string;
  title: string;
  /** The narration: what this chapter does and why, in one to three sentences. */
  narration: string;
  /**
   * Files written before the command runs, as the edit a reader makes in an
   * editor. The player shows them in the files pane and names them in the
   * transcript.
   */
  files?: LabFile[];
  /** The line the chapter runs, in the lab shell's syntax. A chapter without one only reads. */
  command?: string;
  /** The exit code the command must return (default 0). */
  exit?: number;
  /** Substrings the command's combined output must contain. */
  expect?: string[];
  /** Words to point at in the output, named in the chapter's "notice" line. */
  notice?: string[];
  look?: ChapterLook;
  /** A remark drawn after the output: what to take from it. */
  note?: string;
  /** For an explorer lab: what the stage shows during this chapter. */
  stage?: ChapterStage;
  /** A short label for the chapter list where there is no command (an explorer chapter). */
  hint?: string;
  /** For a lab with a lens: the part of the view this chapter brings forward. */
  focus?: string;
}

export interface Lab {
  /** Stable, URL-safe id: the route is /learn/<id>. */
  id: string;
  series: SeriesId;
  /** Position within its series (1-based), for the index and the "up next" link. */
  position: number;
  title: string;
  /** One line under the title. */
  tagline: string;
  /** What the lab teaches, one paragraph. */
  summary: string;
  sample: SampleId;
  /** Terminal (the default), explorer, or playground. */
  kind?: LabKind;
  /** For an explorer lab or playground: the explorer on the stage, unless a chapter names another. */
  explorer?: string;
  /** For an explorer playground: what the stage shows, in the explorer's own terms. */
  stage?: ChapterStage;
  /**
   * A view drawn above the terminal and rebuilt after every command, by id
   * (the player's lens registry resolves it): "context-graph".
   */
  lens?: string;
  /** Whether the engine boots on Play (default true). An explorer that runs only ML models says false. */
  engine?: boolean;
  /** Plugins (by the plugin manager's id) to download on Play, before the first chapter. */
  plugins?: string[];
  /** Files written over the sample before the first chapter (a lab-specific starting state). */
  files?: LabFile[];
  /** Commands run before the first chapter, silently, to reach the starting state. */
  setup?: string[];
  /** The concepts the lab teaches, as short noun phrases. */
  concepts: string[];
  /** Where to read on. */
  docs: { label: string; href: string }[];
  /** A rough duration, in minutes, for the index card. */
  minutes: number;
  /** The chapters, in order. A playground has none. */
  chapters: Chapter[];
  /**
   * Things to try once the chapters are done, or in a playground from the
   * start. In a terminal lab they are commands the player offers to type; in
   * an explorer lab they are prompts in prose.
   */
  tryNext?: string[];
}

export interface Series {
  id: SeriesId;
  title: string;
  tagline: string;
  description: string;
  /** The sample the series runs in, for the index card. */
  sample: SampleId;
}

/** Facts about a sample for the index and the player's "about" section. */
export interface SampleInfo {
  id: SampleId;
  name: string;
  /** The fiction, in a sentence. */
  blurb: string;
  /** The path the sample is cloned from. */
  source: string;
}
