import { describe, it, expect } from "vitest";
import { render, waitFor } from "./testUtils";

import { ReviewPage } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { CheckWarning } from "../types/api";

const WARNINGS: CheckWarning[] = [
  {
    code: "format.no_reader",
    source: "pkg/doc.idml",
    message:
      'no reader for format "okf_idml" is installed, so pkg/doc.idml was not listed for review; install the plugin that supplies it (kapi plugins install okapi-bridge)',
  },
];

describe("ReviewPage over content with no installed reader", () => {
  it("names the files it could not list and never calls the queue fully reviewed", async () => {
    render(
      <ErrorProvider>
        <ReviewPage tabID="t1" items={[]} warnings={WARNINGS} />
      </ErrorProvider>,
    );
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-unread']")).not.toBeNull(),
    );
    const notice = document.querySelector("[data-slot='review-unread']");
    expect(notice?.textContent).toContain("pkg/doc.idml");
    expect(notice?.textContent).toContain("kapi plugins install okapi-bridge");
    expect(document.body.textContent).not.toContain("Every translated unit is reviewed.");
  });

  it("keeps the empty state when every declared file was read", async () => {
    render(
      <ErrorProvider>
        <ReviewPage tabID="t1" items={[]} warnings={[]} />
      </ErrorProvider>,
    );
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-empty']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-slot='review-unread']")).toBeNull();
    expect(document.body.textContent).toContain("Every translated unit is reviewed.");
  });
});
