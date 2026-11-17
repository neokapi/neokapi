import type { ContextCard as Card, ContextRuleExamples } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactElement } from "react";

import "../lib/i18n";
import { ContextCard } from "../components/ContextCard";
import { regexTokens } from "../lib/regexTokens";

const mockExamples = jest.fn<Promise<ContextRuleExamples>, unknown[]>();
jest.mock("../lib/auth", () => ({
  useAuth: () => ({ client: { contextRuleExamples: (...a: unknown[]) => mockExamples(...a) } }),
}));

const hype: Card = {
  item: { kind: "voice_rule", id: "ab12", profile: "docs" },
  kind: "voice_rule",
  revision: "r1",
  pattern: {
    list: "prohibited",
    regex: String.raw`\b(powerful|blazing)\b`,
    description: "Marketing superlatives",
    words: ["powerful", "blazing"],
  },
  where: { level: "project" },
  author: { kind: "person", agent_authored: false },
  own_work: false,
  actions: ["keep", "later"],
};

async function wrap(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("regexTokens", () => {
  it("splits a regex into parts that join back to it", () => {
    const src = String.raw`(?i)\b(?:game[- ]changing|power\w+)s?{1,2}$`;
    const toks = regexTokens(src);
    expect(toks.map((t) => t.text).join("")).toBe(src);
    expect(toks.map((t) => t.kind)).toEqual([
      "group",
      "escape",
      "group",
      "literal",
      "set",
      "literal",
      "alternative",
      "literal",
      "escape",
      "repeat",
      "group",
      "literal",
      "repeat",
      "repeat",
      "anchor",
    ]);
  });
});

describe("voice rule card", () => {
  beforeEach(() => mockExamples.mockReset());

  it("leads with the description and the words, with the pattern behind a toggle", async () => {
    mockExamples.mockResolvedValue({
      examples: [{ before: "A ", match: "powerful", after: " engine.", item: "docs/intro.md" }],
      scanned: 3,
      complete: false,
    });
    await wrap(<ContextCard card={hype} scope={{ ws: "hq", project: "p1" }} />);
    expect(screen.getByTestId("context-pattern-description")).toHaveTextContent(
      "Marketing superlatives",
    );
    expect(screen.getByTestId("context-pattern-words")).toHaveTextContent("powerfulblazing");
    expect(screen.queryByTestId("context-pattern")).toBeNull();
    expect(await screen.findByTestId("rule-example")).toHaveTextContent(
      "A powerful engine.docs/intro.md",
    );
    expect(mockExamples).toHaveBeenCalledWith("hq", "p1", { profile: "docs", id: "ab12" });

    await fireEvent.press(screen.getByTestId("context-pattern-toggle"));
    expect(screen.getByTestId("context-pattern")).toHaveTextContent(
      String.raw`\b(powerful|blazing)\b`,
    );
  });

  it("says when nothing in the content matches", async () => {
    mockExamples.mockResolvedValue({ examples: [], scanned: 40, complete: true });
    await wrap(<ContextCard card={hype} scope={{ ws: "hq", project: "p1" }} />);
    expect(await screen.findByTestId("rule-examples-none")).toHaveTextContent(
      "Nothing in your content matches this rule.",
    );
  });

  it("shows the regex when the rule has neither a description nor a word list", async () => {
    const bare: Card = { ...hype, pattern: { list: "prohibited", regex: String.raw`\b\w+ly\b` } };
    await wrap(<ContextCard card={bare} />);
    expect(screen.getByTestId("context-pattern")).toHaveTextContent(String.raw`\b\w+ly\b`);
    expect(screen.queryByTestId("context-pattern-toggle")).toBeNull();
    expect(mockExamples).not.toHaveBeenCalled();
  });
});
