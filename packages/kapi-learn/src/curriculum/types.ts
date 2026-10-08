// The curriculum's shape: series of labs, labs of chapters.
//
// A lab is a short, scripted session in a sample project: each chapter runs
// one command in the lab shell against the real engine and says what to look
// at. The player plays chapters like a video plays scenes; the verifier
// (scripts/learn-verify) runs every chapter in Node and holds the lab to the
// exit codes and output it declares. Pure data: no React, no engine imports,
// so the Docusaurus build can read it to make one page per lab.

/** The sample project a lab runs in; the files come from samples/<id>/. */
export type SampleId = "northsea" | "compass" | "mart";

export type SeriesId = "start-here" | "add-languages" | "engine";

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
}

export interface Lab {
  /** Stable, URL-safe id: the route is /learn/<id>. */
  id: string;
  series: SeriesId;
  /** Position within its series (1-based), for the index and the "up next" rail. */
  position: number;
  title: string;
  /** One line under the title. */
  tagline: string;
  /** What the lab teaches, one paragraph. */
  summary: string;
  sample: SampleId;
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
  chapters: Chapter[];
  /** Things to try once the chapters are done, as commands or short prompts. */
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

/** Facts about a sample for the index and the player's "about" panel. */
export interface SampleInfo {
  id: SampleId;
  name: string;
  /** The fiction, in a sentence. */
  blurb: string;
  /** The path the sample is cloned from. */
  source: string;
}
