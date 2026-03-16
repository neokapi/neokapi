import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, act } from "@testing-library/react";
import { TaskBoard } from "../components/TaskBoard";
import type { TaskInfo } from "../types/api";

function makeTask(overrides: Partial<TaskInfo> = {}): TaskInfo {
  return {
    id: "task-1",
    workspace_id: "ws-1",
    project_id: "proj-1",
    stream: "",
    type: "translate",
    status: "open",
    priority: "normal",
    title: "Translate homepage",
    description: "French translation needed",
    assignee_id: "user-2",
    created_by: "user-1",
    completed_by: "",
    data: {},
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("TaskBoard", () => {
  it("renders empty state when no tasks", () => {
    render(<TaskBoard tasks={[]} />);
    expect(screen.getByText("No tasks yet")).toBeInTheDocument();
  });

  it("renders loading state when loading with no tasks", () => {
    render(<TaskBoard tasks={[]} loading />);
    expect(screen.getByText("Loading tasks...")).toBeInTheDocument();
  });

  it("renders task cards in list view", () => {
    const tasks = [
      makeTask({ id: "t1", title: "Translate homepage" }),
      makeTask({ id: "t2", title: "Review strings", type: "review" }),
    ];
    render(<TaskBoard tasks={tasks} />);
    expect(screen.getByText("Translate homepage")).toBeInTheDocument();
    expect(screen.getByText("Review strings")).toBeInTheDocument();
  });

  it("shows priority badge", () => {
    render(<TaskBoard tasks={[makeTask({ priority: "high" })]} />);
    expect(screen.getByText("high")).toBeInTheDocument();
  });

  it("shows type label", () => {
    render(<TaskBoard tasks={[makeTask({ type: "review" })]} />);
    expect(screen.getByText("Review")).toBeInTheDocument();
  });

  it("shows Complete and Cancel buttons for active tasks", () => {
    const onComplete = vi.fn();
    const onCancel = vi.fn();
    render(
      <TaskBoard
        tasks={[makeTask({ id: "t1", status: "open" })]}
        onCompleteTask={onComplete}
        onCancelTask={onCancel}
      />,
    );
    expect(screen.getByText("Complete")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  it("calls onCompleteTask when Complete is clicked", () => {
    const onComplete = vi.fn();
    render(
      <TaskBoard tasks={[makeTask({ id: "t1", status: "open" })]} onCompleteTask={onComplete} />,
    );
    act(() => screen.getByText("Complete").click());
    expect(onComplete).toHaveBeenCalledWith("t1");
  });

  it("calls onCancelTask when Cancel is clicked", () => {
    const onCancel = vi.fn();
    render(<TaskBoard tasks={[makeTask({ id: "t1", status: "open" })]} onCancelTask={onCancel} />);
    act(() => screen.getByText("Cancel").click());
    expect(onCancel).toHaveBeenCalledWith("t1");
  });

  it("does not show action buttons for completed tasks", () => {
    render(
      <TaskBoard
        tasks={[makeTask({ status: "completed" })]}
        onCompleteTask={vi.fn()}
        onCancelTask={vi.fn()}
      />,
    );
    expect(screen.queryByText("Complete")).not.toBeInTheDocument();
    expect(screen.queryByText("Cancel")).not.toBeInTheDocument();
  });

  it("calls onTaskClick when a task card is clicked", () => {
    const onClick = vi.fn();
    const task = makeTask();
    render(<TaskBoard tasks={[task]} onTaskClick={onClick} />);
    act(() => screen.getByText("Translate homepage").closest("[role='button']")!.click());
    expect(onClick).toHaveBeenCalledWith(task);
  });

  it("shows Overdue label for overdue active tasks", () => {
    const pastDate = new Date(Date.now() - 86400000).toISOString(); // 1 day ago
    render(<TaskBoard tasks={[makeTask({ due_at: pastDate, status: "open" })]} />);
    expect(screen.getByText("Overdue")).toBeInTheDocument();
  });

  it("shows description when provided", () => {
    render(<TaskBoard tasks={[makeTask({ description: "Urgent translation" })]} />);
    expect(screen.getByText("Urgent translation")).toBeInTheDocument();
  });

  it("toggles between list and board view", () => {
    const tasks = [
      makeTask({ id: "t1", title: "Open task", status: "open" }),
      makeTask({ id: "t2", title: "Done task", status: "completed" }),
    ];
    render(<TaskBoard tasks={tasks} />);

    // Default is list view - should see both tasks
    expect(screen.getByText("Open task")).toBeInTheDocument();
    expect(screen.getByText("Done task")).toBeInTheDocument();

    // Switch to board view
    act(() => screen.getAllByText("Board")[0].click());

    // Board view shows column headers with counts
    expect(screen.getByText(/^Open \(/)).toBeInTheDocument();
    expect(screen.getByText(/^Completed \(/)).toBeInTheDocument();
  });
});
