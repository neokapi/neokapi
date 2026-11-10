import type * as React from "react";

/**
 * One step of a guided tour: a bubble beside the element it is about, with
 * that element lit and the rest of the window dimmed.
 */
export interface GuideStep {
  /** Stable within its guide. */
  id: string;
  /**
   * A CSS selector for the element the step points at, usually
   * `[data-slot="…"]`. With none, or when nothing matches after `waitFor`,
   * the bubble sits in the middle of the window.
   */
  target?: string;
  /** A short heading, sentence case. */
  title: string;
  /** One to three short sentences. A string is rendered as Markdown. */
  body: React.ReactNode;
  /** Which side of the target the bubble prefers; it flips when there is no room. */
  placement?: "top" | "bottom" | "left" | "right";
  /**
   * Runs before the step shows: open the page the step is on, select a tab.
   * The engine then waits for `target` to appear.
   */
  before?: () => void | Promise<void>;
  /** How long to wait for `target` after `before`, in ms. Defaults to 4000. */
  waitFor?: number;
  /**
   * The step is about an element that only some states draw, such as the
   * consequence line of a suggestion that has one. When `target` is not there
   * after `waitFor`, the tour moves past the step in the direction it was
   * going instead of showing the bubble in the middle of the window.
   */
  optional?: boolean;
  /**
   * `next` (default) advances on the bubble's Next button. `target` also
   * advances when the person clicks the lit element, for a step that asks
   * them to do something.
   */
  advanceOn?: "next" | "target";
}

/** A guided tour through one common task. */
export interface Guide {
  id: string;
  /** What the person will have done at the end, as a task: "Check and fix a file". */
  title: string;
  /** One line, shown in the Help panel's list. */
  summary: string;
  steps: GuideStep[];
  /**
   * Whether the tour can start where the person is, for example only inside a
   * project. A guide that cannot start shows why in the Help panel.
   */
  available?: () => { ok: true } | { ok: false; reason: string };
  /** A help topic to read after the tour, by id. */
  topic?: string;
}

/** A short page of help, read inside the app. */
export interface HelpTopic {
  id: string;
  title: string;
  /** One line, shown in the list. */
  summary: string;
  /** Markdown: short sections, lists, the controls named in bold. */
  body: string;
  /** Guides that walk through what the topic describes, by id. */
  guides?: string[];
  /** The page on the website with the full story, shown as the topic's last line. */
  moreUrl?: string;
}

/** What the guide context offers to the app. */
export interface GuideApi {
  /** Start a tour by id; any running tour stops first. */
  start: (guideId: string) => void;
  /** End the running tour. */
  stop: () => void;
  /** The running tour and the index of its step, or null. */
  active: { guide: Guide; step: number } | null;
  /** Open the Help panel, optionally on one topic. */
  openHelp: (topicId?: string) => void;
  /** Guides finished on this machine, by id. */
  completed: ReadonlySet<string>;
}
