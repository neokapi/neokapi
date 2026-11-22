import type {
  FeedEntry,
  PlanStep,
  PlanWait,
  Project,
  ReviewInboxLanguage,
  ReviewInboxProject,
} from "@neokapi/bowrain-client";
import { fireEvent, render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { ActivityStrip, recentActors } from "../components/home/ActivityStrip";
import { LanguageCard, ProjectCard, WorkspaceCard } from "../components/home/HomeCards";
import { Monogram, monogramGradient } from "../components/home/Monogram";
import { CaughtUpCard, NextUpCard } from "../components/home/NextUpCard";
import { ReviewMix } from "../components/home/ReviewMix";
import { summarize, projectKey } from "../lib/home";

const nb: ReviewInboxLanguage = {
  workspace: "acme",
  workspace_name: "Acme",
  project: "p1",
  project_name: "Website",
  stream: "main",
  locale: "nb",
  waiting: 12,
  fast: 8,
  closer: 3,
  desk: 1,
  unclassified: 0,
};

const project: Project = {
  id: "p1",
  name: "Website",
  default_source_language: "en",
  target_languages: ["de", "nb", "fr", "es", "it", "ja", "ko", "pt-BR"],
};

describe("Monogram", () => {
  it("shows the initials and keeps one gradient per seed", async () => {
    await render(<Monogram name="Acme Docs" seed="ws-1" testID="mark" />);
    expect(screen.getByText("AD", { includeHiddenElements: true })).toBeTruthy();
    expect(monogramGradient("ws-1")).toBe(monogramGradient("ws-1"));
  });
});

describe("ReviewMix", () => {
  it("names each group that holds anything, in deck order", async () => {
    await render(<ReviewMix testID="mix" mix={{ fast: 8, closer: 0, desk: 1 }} />);
    expect(screen.getByLabelText("8 quick · 1 for the desk")).toBeTruthy();
    expect(screen.getByTestId("mix-fast")).toBeTruthy();
    expect(screen.queryByTestId("mix-closer")).toBeNull();
  });

  it("draws nothing when nothing waits", async () => {
    await render(<ReviewMix testID="mix" mix={{ fast: 0 }} />);
    expect(screen.queryByTestId("mix")).toBeNull();
  });
});

describe("NextUpCard", () => {
  const website: ReviewInboxProject = {
    workspace: "acme",
    workspaceName: "Acme",
    project: "p1",
    projectName: "Website",
    followed: ["nb"],
    saved: true,
    canManage: false,
    waiting: 12,
  };
  const step = (level: PlanStep["level"], extra: Partial<PlanStep> = {}): PlanStep => ({
    level,
    state: "open",
    count: 1,
    progress: 0,
    done: false,
    changed: 0,
    ...extra,
  });
  const quota: PlanWait = {
    level: "context",
    reason: "waiting_quota",
    quotaResetsAt: "2026-12-01T00:00:00Z",
  };

  it("opens the deck and links to the other decks", async () => {
    const onOpen = jest.fn();
    const onMore = jest.fn();
    await render(
      <NextUpCard
        next={{
          kind: "deck",
          project: website,
          step: step("interfaces"),
          entry: nb,
          waiting: [],
          more: 2,
        }}
        showWorkspace
        onOpen={onOpen}
        onMore={onMore}
      />,
    );
    expect(screen.getByText("Website · Acme")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("Continue reviewing")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("home-next-open"));
    expect(onOpen).toHaveBeenCalled();
    expect(screen.getByText("2 more decks waiting")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("home-next-more"));
    expect(onMore).toHaveBeenCalled();
  });

  it("shows the waiting step beside the deck", async () => {
    await render(
      <NextUpCard
        next={{
          kind: "deck",
          project: website,
          step: step("exceptions"),
          entry: nb,
          waiting: [quota],
          more: 0,
        }}
        showWorkspace={false}
        onOpen={() => {}}
      />,
    );
    expect(screen.getByTestId("home-next-waiting-context")).toBeTruthy();
    expect(screen.getByText("Waits for the AI limit to reset on December 1, 2026")).toBeTruthy();
  });

  it("offers the setup before the person has chosen their languages", async () => {
    const onOpen = jest.fn();
    await render(
      <NextUpCard
        next={{ kind: "setup", project: { ...website, saved: false }, waiting: [], more: 0 }}
        showWorkspace={false}
        onOpen={onOpen}
      />,
    );
    expect(screen.getByText("Set up review")).toBeTruthy();
    expect(screen.getByText("Choose your languages")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("home-next-open"));
    expect(onOpen).toHaveBeenCalled();
    expect(screen.queryByTestId("home-next-more")).toBeNull();
  });

  it("asks for the coordinates to be confirmed", async () => {
    await render(
      <NextUpCard
        next={{ kind: "step", project: website, step: step("coordinates"), waiting: [], more: 3 }}
        showWorkspace={false}
        onOpen={() => {}}
        onMore={() => {}}
      />,
    );
    expect(screen.getByText("Confirm the coordinates")).toBeTruthy();
    expect(screen.getByText("Not confirmed yet")).toBeTruthy();
    expect(screen.getByText("Open the map")).toBeTruthy();
    expect(screen.getByText("3 decks waiting")).toBeTruthy();
  });

  it("opens the context with what waits there", async () => {
    await render(
      <NextUpCard
        next={{
          kind: "step",
          project: website,
          step: step("context", { count: 4 }),
          waiting: [],
          more: 0,
        }}
        showWorkspace={false}
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("Review context")).toBeTruthy();
    expect(screen.getByText("4 context cards")).toBeTruthy();
  });

  it("states what waits when no step can be acted on", async () => {
    await render(
      <NextUpCard
        next={{ kind: "waiting", project: website, waiting: [quota], more: 0 }}
        showWorkspace={false}
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("Nothing to decide yet")).toBeTruthy();
    expect(screen.getByText("Context")).toBeTruthy();
    expect(screen.getByText("Waits for the AI limit to reset on December 1, 2026")).toBeTruthy();
    expect(screen.queryByTestId("home-next-open")).toBeNull();
  });

  it("counts the person's own context decisions, not the team's", async () => {
    await render(
      <NextUpCard
        next={{
          kind: "step",
          project: website,
          step: step("context", { count: 9, mine: 2 }),
          waiting: [],
          more: 0,
        }}
        showWorkspace={false}
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("2 context cards")).toBeTruthy();
  });

  it("names the languages whose terms other reviewers decide", async () => {
    await render(
      <NextUpCard
        next={{
          kind: "waiting",
          project: website,
          waiting: [{ level: "context", reason: "other_languages", locales: ["de"] }],
          more: 0,
        }}
        showWorkspace={false}
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("Nothing to decide yet")).toBeTruthy();
    expect(screen.getByText("Terms missing in German, reviewed by others")).toBeTruthy();
  });

  it("says when nothing waits", async () => {
    await render(<CaughtUpCard />);
    expect(screen.getByText("All caught up")).toBeTruthy();
  });
});

describe("WorkspaceCard", () => {
  it("sums what waits and opens the workspace", async () => {
    const onPress = jest.fn();
    const ws = { id: "w1", name: "Acme", slug: "acme" };
    await render(
      <WorkspaceCard
        workspace={ws}
        waiting={{ translations: 12, context: 4, languages: 1 }}
        onPress={onPress}
      />,
    );
    expect(screen.getByText("12 translations · 4 context items")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("workspace-acme"));
    expect(onPress).toHaveBeenCalledWith(ws);
  });
});

describe("ProjectCard", () => {
  it("puts the languages with work waiting first and folds the rest", async () => {
    const waiting = summarize({ context: [], languages: [nb], projects: [] }).byProject.get(
      projectKey("acme", "p1"),
    );
    await render(<ProjectCard project={project} waiting={waiting} onPress={() => {}} />);
    const badges = screen
      .getAllByText(/^[A-Z]{2}(-[A-Z]{2})?$/, { includeHiddenElements: true })
      .map((n) => n.props.children)
      .filter((c) => c !== "WE");
    expect(badges[0]).toBe("NB");
    expect(badges).toHaveLength(6);
    expect(screen.getByText("+2")).toBeTruthy();
    expect(screen.getByText("12 translations in 1 language")).toBeTruthy();
  });

  it("says when nothing waits", async () => {
    await render(<ProjectCard project={project} onPress={() => {}} />);
    expect(screen.getByText("Nothing waiting for you")).toBeTruthy();
  });
});

describe("LanguageCard", () => {
  const nb: ReviewInboxLanguage = {
    workspace: "acme",
    workspace_name: "Acme",
    project: "p1",
    project_name: "Docs",
    stream: "main",
    locale: "nb",
    waiting: 34229,
    fast: 0,
    closer: 25576,
    desk: 2822,
    unclassified: 5831,
  };

  it("shows the inbox's waiting count, the groups, and opens the language", async () => {
    const onPress = jest.fn();
    await render(<LanguageCard locale="nb" waiting={nb} onPress={onPress} />);
    expect(screen.getByTestId("language-nb-mix")).toBeTruthy();
    expect(screen.getByText("34,229")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("language-nb"));
    expect(onPress).toHaveBeenCalledWith("nb");
  });

  it("says nothing waits for a language the inbox does not list", async () => {
    await render(<LanguageCard locale="nb" onPress={() => {}} />);
    expect(screen.queryByTestId("language-nb-mix")).toBeNull();
    expect(screen.getByText("Nothing waiting for you")).toBeTruthy();
  });
});

function entry(id: string, actorId: string, name: string): FeedEntry {
  return {
    id,
    kind: "push",
    at: new Date().toISOString(),
    actor: { id: actorId, kind: "person", name },
    counts: { blocks: 3 },
    stream: "main",
    link: { screen: "stream" },
  };
}

describe("ActivityStrip", () => {
  const entries = [
    entry("1", "a", "Ada Lovelace"),
    entry("2", "a", "Ada Lovelace"),
    entry("3", "b", "Grace Hopper"),
  ];

  it("keeps one avatar per actor", () => {
    expect(recentActors(entries).map((e) => e.id)).toEqual(["1", "3"]);
  });

  it("reads the newest entry as a sentence", async () => {
    const onPress = jest.fn();
    await render(<ActivityStrip entries={entries} onPress={onPress} />);
    expect(screen.getByText("AL", { includeHiddenElements: true })).toBeTruthy();
    expect(screen.getByText("GH", { includeHiddenElements: true })).toBeTruthy();
    await fireEvent.press(screen.getByTestId("project-activity"));
    expect(onPress).toHaveBeenCalled();
  });

  it("draws nothing for an empty feed", async () => {
    await render(<ActivityStrip entries={[]} onPress={() => {}} />);
    expect(screen.queryByTestId("project-activity")).toBeNull();
  });
});
