import { useState, useCallback } from "react";
import type { TaskInfo, CreateTaskRequest, TaskStatus, TaskPriority } from "../types/api";
import { cn } from "../lib/utils";
import { Badge } from "./ui/badge";

export interface TaskBoardProps {
  tasks: TaskInfo[];
  loading?: boolean;
  currentUserId?: string;
  onCreateTask?: (task: CreateTaskRequest) => void;
  onCompleteTask?: (taskId: string) => void;
  onCancelTask?: (taskId: string) => void;
  onAssignTask?: (taskId: string, assigneeId: string) => void;
  onTaskClick?: (task: TaskInfo) => void;
}

const priorityColors: Record<TaskPriority, string> = {
  low: "bg-muted text-muted-foreground",
  normal: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  high: "bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200",
  urgent: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
};

const statusLabels: Record<TaskStatus, string> = {
  open: "Open",
  in_progress: "In Progress",
  completed: "Completed",
  cancelled: "Cancelled",
};

const typeLabels: Record<string, string> = {
  translate: "Translation",
  review: "Review",
  review_terms: "Term Review",
  fix_quality: "QA Fix",
  fix_brand_voice: "Brand Fix",
  fix_terminology: "Terminology Fix",
  connector_setup: "Connector Setup",
  custom: "Custom",
};

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(dateStr).toLocaleDateString();
}

function TaskCard({
  task,
  onComplete,
  onCancel,
  onClick,
}: {
  task: TaskInfo;
  onComplete?: () => void;
  onCancel?: () => void;
  onClick?: () => void;
}) {
  const isActive = task.status === "open" || task.status === "in_progress";
  const isOverdue = task.due_at && new Date(task.due_at) < new Date() && isActive;

  return (
    <div
      className={cn(
        "rounded-lg border p-3 transition-colors",
        "hover:bg-accent/50",
        onClick ? "cursor-pointer" : "",
        isOverdue && "border-destructive/50",
      )}
      onClick={onClick}
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium leading-snug truncate">{task.title}</p>
          {task.description && (
            <p className="text-xs text-muted-foreground mt-1 line-clamp-2">{task.description}</p>
          )}
        </div>
        <Badge variant="outline" className={cn("text-[10px] shrink-0", priorityColors[task.priority])}>
          {task.priority}
        </Badge>
      </div>
      <div className="flex items-center gap-2 mt-2 flex-wrap">
        <Badge variant="secondary" className="text-[10px]">
          {typeLabels[task.type] || task.type}
        </Badge>
        {isOverdue && (
          <span className="text-[10px] text-destructive font-medium">Overdue</span>
        )}
        {task.due_at && !isOverdue && isActive && (
          <span className="text-[10px] text-muted-foreground">
            Due {timeAgo(task.due_at)}
          </span>
        )}
        <span className="text-[10px] text-muted-foreground ml-auto">
          {timeAgo(task.created_at)}
        </span>
      </div>
      {isActive && (onComplete || onCancel) && (
        <div className="flex gap-1 mt-2">
          {onComplete && (
            <button
              type="button"
              className="text-[11px] px-2 py-0.5 rounded bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200 hover:opacity-80 transition-opacity"
              onClick={(e) => { e.stopPropagation(); onComplete(); }}
            >
              Complete
            </button>
          )}
          {onCancel && (
            <button
              type="button"
              className="text-[11px] px-2 py-0.5 rounded bg-muted text-muted-foreground hover:opacity-80 transition-opacity"
              onClick={(e) => { e.stopPropagation(); onCancel(); }}
            >
              Cancel
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export function TaskBoard({
  tasks,
  loading,
  currentUserId,
  onCreateTask,
  onCompleteTask,
  onCancelTask,
  onAssignTask,
  onTaskClick,
}: TaskBoardProps) {
  const [view, setView] = useState<"list" | "board">("list");

  if (loading && tasks.length === 0) {
    return (
      <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
        Loading tasks...
      </div>
    );
  }

  if (tasks.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 gap-2">
        <p className="text-sm text-muted-foreground">No tasks yet</p>
      </div>
    );
  }

  const viewToggle = (
    <div className="flex items-center justify-end mb-3">
      <div className="flex gap-1 rounded-md border p-0.5">
        <button
          type="button"
          className={cn("text-xs px-2 py-1 rounded", view === "list" && "bg-accent")}
          onClick={() => setView("list")}
        >
          List
        </button>
        <button
          type="button"
          className={cn("text-xs px-2 py-1 rounded", view === "board" && "bg-accent")}
          onClick={() => setView("board")}
        >
          Board
        </button>
      </div>
    </div>
  );

  if (view === "board") {
    const columns: TaskStatus[] = ["open", "in_progress", "completed", "cancelled"];
    const grouped = columns.reduce(
      (acc, status) => {
        acc[status] = tasks.filter((t) => t.status === status);
        return acc;
      },
      {} as Record<TaskStatus, TaskInfo[]>,
    );

    return (
      <div>
        {viewToggle}
        <div className="grid grid-cols-4 gap-3">
          {columns.map((status) => (
            <div key={status} className="space-y-2">
              <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider px-1">
                {statusLabels[status]} ({grouped[status].length})
              </h3>
              <div className="space-y-2">
                {grouped[status].map((task) => (
                  <TaskCard
                    key={task.id}
                    task={task}
                    onComplete={onCompleteTask ? () => onCompleteTask(task.id) : undefined}
                    onCancel={onCancelTask ? () => onCancelTask(task.id) : undefined}
                    onClick={onTaskClick ? () => onTaskClick(task) : undefined}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div>
      {viewToggle}
      <div className="space-y-2">
        {tasks.map((task) => (
          <TaskCard
            key={task.id}
            task={task}
            onComplete={onCompleteTask ? () => onCompleteTask(task.id) : undefined}
            onCancel={onCancelTask ? () => onCancelTask(task.id) : undefined}
            onClick={onTaskClick ? () => onTaskClick(task) : undefined}
          />
        ))}
      </div>
    </div>
  );
}
