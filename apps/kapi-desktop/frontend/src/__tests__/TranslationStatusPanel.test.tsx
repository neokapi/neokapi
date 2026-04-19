import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import { TranslationStatusPanel, type ProjectStatus } from "../components/TranslationStatusPanel";
import { api } from "../hooks/useApi";

function statusFixture(overrides: Partial<ProjectStatus> = {}): ProjectStatus {
  return {
    projectPath: "/tmp/project.kapi",
    projectName: "Test",
    collections: [
      {
        name: "ui",
        targetLanguages: ["fr", "de", "ja"],
      },
    ],
    ...overrides,
  };
}

describe("TranslationStatusPanel", () => {
  it("renders one row per declared target locale", () => {
    render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
    const list = document.querySelector("[data-slot='locale-coverage']") as HTMLElement;
    expect(list).not.toBeNull();
    expect(list.querySelectorAll("[data-locale]")).toHaveLength(3);
  });

  it("shows 'pending' when no blockstore coverage data is available yet", () => {
    render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
    const pending = screen.getAllByText("pending");
    expect(pending.length).toBeGreaterThanOrEqual(3);
  });

  it("shows per-locale coverage when blockCount is available", () => {
    render(
      <TranslationStatusPanel
        tabID="t1"
        status={statusFixture({
          collections: [
            {
              name: "ui",
              blockCount: 100,
              coverage: { fr: 50, ja: 100 },
              targetLanguages: ["fr", "de", "ja"],
            },
          ],
        })}
      />,
    );
    expect(screen.getByText("50/100")).toBeInTheDocument();
    expect(screen.getByText("100/100 · complete")).toBeInTheDocument();
    expect(screen.getByText("not translated")).toBeInTheDocument();
  });

  it("reports an empty project when the recipe declares no collections", () => {
    render(<TranslationStatusPanel tabID="t1" status={statusFixture({ collections: [] })} />);
    expect(screen.getByText(/No content collections defined/)).toBeInTheDocument();
  });

  describe("Re-extract action", () => {
    beforeEach(() => {
      vi.restoreAllMocks();
    });

    it("hides the Re-extract button when the project has no collections", () => {
      render(<TranslationStatusPanel tabID="t1" status={statusFixture({ collections: [] })} />);
      expect(document.querySelector("[data-slot='translation-status-reextract']")).toBeNull();
    });

    it("invokes api.runExtract and surfaces the log", async () => {
      const runExtract = vi.spyOn(api, "runExtract").mockResolvedValue({
        log: "  ui → @neokapi/kapi-react (3 file(s))\n",
      });

      render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
      const button = document.querySelector(
        "[data-slot='translation-status-reextract']",
      ) as HTMLButtonElement;
      expect(button).not.toBeNull();

      fireEvent.click(button);
      await waitFor(() => expect(runExtract).toHaveBeenCalledWith("t1"));
      await waitFor(() => {
        const log = document.querySelector("[data-slot='translation-status-log']");
        expect(log?.textContent).toContain("3 file(s)");
      });
    });

    it("surfaces errors returned by api.runExtract", async () => {
      vi.spyOn(api, "runExtract").mockRejectedValue(new Error("extractor crashed"));
      render(<TranslationStatusPanel tabID="t1" status={statusFixture()} />);
      const button = document.querySelector(
        "[data-slot='translation-status-reextract']",
      ) as HTMLButtonElement;
      fireEvent.click(button);
      await waitFor(() => expect(screen.getByText(/extractor crashed/i)).toBeInTheDocument());
    });
  });
});
