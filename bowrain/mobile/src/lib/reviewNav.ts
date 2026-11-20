// Where the review plan's steps open, for the Home tab and the Review tab
// alike. Each opener records the project as the one last opened, so Home and
// the Activity tab follow the person to it.
import type { PlanLevel, ReviewInboxLanguage, ReviewInboxProject } from "@neokapi/bowrain-client";
import { router } from "expo-router";

import { setRecentProject } from "./recentProject";

function remember(p: ReviewInboxProject) {
  setRecentProject({ ws: p.workspace, project: p.project, name: p.projectName });
}

/** Opens the review setup for a project, then the step the person asked for. */
export function openSetup(p: ReviewInboxProject, next?: PlanLevel) {
  remember(p);
  router.push({
    pathname: "/[ws]/[project]/languages",
    params: next
      ? { ws: p.workspace, project: p.project, setup: "1", next }
      : { ws: p.workspace, project: p.project, setup: "1" },
  });
}

/**
 * Opens one step of a project's plan; before the setup, the setup first. The
 * coordinates open their map, narrowed to the languages the person follows.
 */
export function openStep(p: ReviewInboxProject, level: PlanLevel) {
  remember(p);
  if (!p.saved) {
    openSetup(p, level);
    return;
  }
  if (level === "coordinates") {
    router.push({
      pathname: "/[ws]/[project]/coordinates",
      params: { ws: p.workspace, project: p.project, languages: p.followed.join(",") },
    });
    return;
  }
  router.push({
    pathname: "/[ws]/[project]/plan",
    params: { ws: p.workspace, project: p.project, step: level },
  });
}

/** Opens a project's context cards. */
export function openContext(e: { workspace: string; project: string; project_name: string }) {
  setRecentProject({ ws: e.workspace, project: e.project, name: e.project_name });
  router.push({
    pathname: "/[ws]/[project]/context",
    params: { ws: e.workspace, project: e.project },
  });
}

/** Opens one language's review deck over the tabs. */
export function openDeck(e: ReviewInboxLanguage) {
  setRecentProject({ ws: e.workspace, project: e.project, name: e.project_name });
  router.push({
    pathname: "/[ws]/[project]/[locale]",
    params: { ws: e.workspace, project: e.project, locale: e.locale },
  });
}
