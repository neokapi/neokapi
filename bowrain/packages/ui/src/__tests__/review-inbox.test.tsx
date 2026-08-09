import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { ReviewInbox, type ReviewInboxTask } from "../components/review/ReviewInbox";

// The dashboard counts open review tasks and its card links to the inbox, so
// a task counted there must be visible here — an inbox that says "Nothing
// awaiting review" while the dashboard says "1 open review task" is the
// defect these tests pin shut.
describe("ReviewInbox tasks section", () => {
  const changesetTask: ReviewInboxTask = {
    id: "t1",
    title: "Review terminology change-set",
    type: "review_terms",
    changesetId: "cs123",
  };

  it("shows an open review task even when no project has pending units", () => {
    const onOpenTask = vi.fn();
    render(
      <ReviewInbox
        projects={[]}
        tasks={[changesetTask]}
        onOpenReview={() => {}}
        onOpenTask={onOpenTask}
      />,
    );
    expect(screen.queryByTestId("review-inbox-empty")).toBeNull();
    expect(screen.getByTestId("review-inbox-tasks")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("review-inbox-task-t1"));
    expect(onOpenTask).toHaveBeenCalledWith(changesetTask);
  });

  it("is empty only when there are neither tasks nor pending projects", () => {
    render(<ReviewInbox projects={[]} tasks={[]} onOpenReview={() => {}} />);
    expect(screen.getByTestId("review-inbox-empty")).toBeInTheDocument();
  });
});
