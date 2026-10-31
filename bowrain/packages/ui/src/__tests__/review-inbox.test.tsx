import { describe, expect, it, vi } from "vite-plus/test";
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

describe("ReviewInbox rules waiting", () => {
  const rules = [
    { projectId: "p1", projectName: "Fernwell", stream: "main", waiting: 3, assigned: 1 },
    { projectId: "p2", projectName: "Docs site", stream: "main", waiting: 2, assigned: 0 },
    { projectId: "p3", projectName: "Help Center", stream: "main", waiting: 0, assigned: 0 },
  ];

  it("counts the questions waiting, by project, and opens a project's Rules", () => {
    const onOpenRules = vi.fn();
    render(
      <ReviewInbox projects={[]} rules={rules} onOpenReview={() => {}} onOpenRules={onOpenRules} />,
    );
    expect(screen.queryByTestId("review-inbox-empty")).toBeNull();
    expect(screen.getByTestId("review-inbox-rules-count")).toHaveTextContent("5");
    expect(screen.getByTestId("review-inbox-rules-p1")).toHaveTextContent("1 assigned to you");
    expect(screen.getByTestId("review-inbox-rules-p2")).not.toHaveTextContent("assigned");
    expect(screen.queryByTestId("review-inbox-rules-p3")).toBeNull();
    fireEvent.click(screen.getByTestId("review-inbox-rules-p2"));
    expect(onOpenRules).toHaveBeenCalledWith("p2", "main");
  });

  it("draws no Rules waiting when no question waits", () => {
    render(<ReviewInbox projects={[]} rules={[rules[2]]} onOpenReview={() => {}} />);
    expect(screen.queryByTestId("review-inbox-rules")).toBeNull();
    expect(screen.getByTestId("review-inbox-empty")).toBeInTheDocument();
  });
});
