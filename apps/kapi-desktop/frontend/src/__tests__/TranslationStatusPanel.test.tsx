import { describe, it, expect } from "vitest";
import { render, screen, within } from "@testing-library/react";

import {
  TranslationStatusPanel,
  type ProjectStatus,
} from "../components/TranslationStatusPanel";

function statusFixture(overrides: Partial<ProjectStatus> = {}): ProjectStatus {
  return {
    projectPath: "/tmp/project.kapi",
    projectName: "Test",
    collections: [
      {
        name: "ui",
        archive: "i18n/ui.klz",
        archiveExists: true,
        blockCount: 100,
        coverage: { fr: 50, ja: 100 },
        targetLanguages: ["fr", "de", "ja"],
      },
    ],
    ...overrides,
  };
}

describe("TranslationStatusPanel", () => {
  it("renders a coverage row per declared locale", () => {
    render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
    const panel = screen.getByText("ui").closest("[data-slot='locale-coverage']");
    // The panel contains <li data-locale>...</li> items — find them by attribute.
    const list = document.querySelector("[data-slot='locale-coverage']") as HTMLElement;
    expect(list).not.toBeNull();
    expect(list.querySelectorAll("[data-locale]")).toHaveLength(3);
  });

  it("labels partially-translated locales with the count", () => {
    render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
    expect(screen.getByText("50/100")).toBeInTheDocument();
  });

  it("marks fully-translated locales complete", () => {
    render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
    expect(screen.getByText("100/100 · complete")).toBeInTheDocument();
  });

  it("shows 'not translated' for locales with zero coverage", () => {
    render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
    const de = document.querySelector("[data-locale='de']") as HTMLElement;
    expect(de).not.toBeNull();
    expect(within(de).getByText("not translated")).toBeInTheDocument();
  });

  it("flags missing archives", () => {
    render(
      <TranslationStatusPanel
        tabID="t1"
        status={statusFixture({
          collections: [
            {
              name: "ui",
              archive: "i18n/ui.klz",
              archiveExists: false,
              blockCount: 0,
              coverage: {},
              targetLanguages: ["fr"],
            },
          ],
        })}
      />,
    );
    expect(screen.getByText(/archive missing/i)).toBeInTheDocument();
  });

  it("treats collections without archive as file-based", () => {
    render(
      <TranslationStatusPanel
        tabID="t1"
        status={statusFixture({
          collections: [
            {
              name: "legacy",
              archive: "",
              archiveExists: false,
              blockCount: 0,
              coverage: {},
              targetLanguages: [],
            },
          ],
        })}
      />,
    );
    expect(screen.getByText(/file-based flow/i)).toBeInTheDocument();
  });
});
