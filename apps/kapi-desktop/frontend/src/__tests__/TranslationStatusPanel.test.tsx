import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, within, fireEvent, waitFor } from "@testing-library/react";

import {
  TranslationStatusPanel,
  type ProjectStatus,
} from "../components/TranslationStatusPanel";
import { api } from "../hooks/useApi";

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

  describe("Re-extract action", () => {
    beforeEach(() => {
      vi.restoreAllMocks();
    });

    it("renders the Re-extract button only when at least one collection declares an archive", () => {
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
      expect(
        document.querySelector("[data-slot='translation-status-reextract']"),
      ).toBeNull();
    });

    it("invokes api.runExtract and surfaces the log", async () => {
      const runExtract = vi
        .spyOn(api, "runExtract")
        .mockResolvedValue({ log: "  ui → @neokapi/kapi-react (3 file(s))\n  i18n/ui.klz ← 12 blocks across 3 documents\n" });

      render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
      const button = document.querySelector(
        "[data-slot='translation-status-reextract']",
      ) as HTMLButtonElement;
      expect(button).not.toBeNull();

      fireEvent.click(button);
      await waitFor(() => expect(runExtract).toHaveBeenCalledWith("t1"));
      await waitFor(() => {
        const log = document.querySelector("[data-slot='translation-status-log']");
        expect(log?.textContent).toContain("12 blocks across 3 documents");
      });
    });

    it("surfaces errors returned by api.runExtract", async () => {
      vi.spyOn(api, "runExtract").mockRejectedValue(new Error("extractor crashed"));
      render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
      const button = document.querySelector(
        "[data-slot='translation-status-reextract']",
      ) as HTMLButtonElement;
      fireEvent.click(button);
      await waitFor(() =>
        expect(screen.getByText(/extractor crashed/i)).toBeInTheDocument(),
      );
    });
  });
});
