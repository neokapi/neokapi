import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { EyeOff, Eye, Trash2 } from "lucide-react";
import {
  SelectableList,
  type SelectableListColumn,
  type SelectableListAction,
} from "../../components/ui/selectable-list";
import { Badge } from "../../components/ui/badge";

// --- Sample data ---

interface Locale {
  code: string;
  displayName: string;
  isCustom: boolean;
  isHidden: boolean;
}

const sampleLocales: Locale[] = [
  { code: "ar", displayName: "Arabic", isCustom: false, isHidden: false },
  { code: "zh", displayName: "Chinese", isCustom: false, isHidden: false },
  { code: "cs", displayName: "Czech", isCustom: false, isHidden: true },
  { code: "da", displayName: "Danish", isCustom: false, isHidden: false },
  { code: "nl", displayName: "Dutch", isCustom: false, isHidden: false },
  { code: "en", displayName: "English", isCustom: false, isHidden: false },
  { code: "fi", displayName: "Finnish", isCustom: false, isHidden: true },
  { code: "fr", displayName: "French", isCustom: false, isHidden: false },
  { code: "de", displayName: "German", isCustom: false, isHidden: false },
  { code: "gsw", displayName: "Swiss German", isCustom: true, isHidden: false },
  { code: "it", displayName: "Italian", isCustom: false, isHidden: false },
  { code: "ja", displayName: "Japanese", isCustom: false, isHidden: false },
  { code: "ko", displayName: "Korean", isCustom: false, isHidden: false },
  { code: "nb", displayName: "Norwegian Bokmål", isCustom: false, isHidden: false },
  { code: "pt-BR", displayName: "Brazilian Portuguese", isCustom: false, isHidden: false },
  { code: "es", displayName: "Spanish", isCustom: false, isHidden: false },
];

const columns: SelectableListColumn<Locale>[] = [
  {
    header: "Language",
    cell: (item) => <span className="text-sm">{item.displayName}</span>,
    className: "flex-1",
  },
  {
    header: "Code",
    cell: (item) => <span className="font-mono text-xs text-muted-foreground">{item.code}</span>,
    className: "w-24",
  },
  {
    header: "Status",
    cell: (item) => (
      <>
        {item.isCustom && (
          <Badge variant="secondary" className="text-[9px]">
            custom
          </Badge>
        )}
        {item.isHidden && (
          <Badge variant="outline" className="text-[9px]">
            hidden
          </Badge>
        )}
      </>
    ),
    className: "w-20",
  },
];

// --- Stories ---

function LocaleListDemo() {
  const [items, setItems] = useState(sampleLocales);

  const actions: SelectableListAction<Locale>[] = [
    {
      label: (
        <>
          <EyeOff size={12} /> Hide
        </>
      ),
      onAction: (selected) => {
        const codes = new Set(selected.map((s) => s.code));
        setItems((prev) =>
          prev.map((item) => (codes.has(item.code) ? { ...item, isHidden: true } : item)),
        );
      },
      when: (item) => !item.isHidden && !item.isCustom,
    },
    {
      label: (
        <>
          <Eye size={12} /> Show
        </>
      ),
      onAction: (selected) => {
        const codes = new Set(selected.map((s) => s.code));
        setItems((prev) =>
          prev.map((item) => (codes.has(item.code) ? { ...item, isHidden: false } : item)),
        );
      },
      when: (item) => item.isHidden,
    },
    {
      label: (
        <>
          <Trash2 size={12} /> Remove
        </>
      ),
      onAction: (selected) => {
        const codes = new Set(selected.map((s) => s.code));
        setItems((prev) => prev.filter((item) => !codes.has(item.code)));
      },
      when: (item) => item.isCustom,
    },
  ];

  return (
    <div className="max-w-2xl">
      <SelectableList
        items={items}
        getKey={(item) => item.code}
        columns={columns}
        actions={actions}
        filterFn={(item, q) =>
          item.displayName.toLowerCase().includes(q.toLowerCase()) ||
          item.code.toLowerCase().includes(q.toLowerCase())
        }
        filterPlaceholder="Filter locales..."
        rowClassName={(item) => (item.isHidden ? "opacity-50" : "")}
      />
    </div>
  );
}

interface Task {
  id: string;
  title: string;
  status: "todo" | "in-progress" | "done";
  assignee: string;
}

const sampleTasks: Task[] = [
  { id: "1", title: "Design locale selector", status: "done", assignee: "Alice" },
  { id: "2", title: "Implement backend API", status: "in-progress", assignee: "Bob" },
  { id: "3", title: "Write tests", status: "todo", assignee: "Alice" },
  { id: "4", title: "Add Storybook stories", status: "todo", assignee: "Charlie" },
  { id: "5", title: "Code review", status: "todo", assignee: "Bob" },
];

function TaskListDemo() {
  const [items, setItems] = useState(sampleTasks);

  const taskColumns: SelectableListColumn<Task>[] = [
    { header: "Title", cell: (t) => <span className="text-sm font-medium">{t.title}</span> },
    {
      header: "Status",
      cell: (t) => (
        <Badge
          variant={
            t.status === "done" ? "default" : t.status === "in-progress" ? "secondary" : "outline"
          }
          className="text-[10px]"
        >
          {t.status}
        </Badge>
      ),
      className: "w-28",
    },
    {
      header: "Assignee",
      cell: (t) => <span className="text-xs text-muted-foreground">{t.assignee}</span>,
      className: "w-24",
    },
  ];

  return (
    <div className="max-w-2xl">
      <SelectableList
        items={items}
        getKey={(t) => t.id}
        columns={taskColumns}
        actions={[
          {
            label: (
              <>
                <Trash2 size={12} /> Delete
              </>
            ),
            onAction: (sel) => {
              const ids = new Set(sel.map((s) => s.id));
              setItems((prev) => prev.filter((t) => !ids.has(t.id)));
            },
          },
        ]}
        filterFn={(t, q) =>
          t.title.toLowerCase().includes(q.toLowerCase()) ||
          t.assignee.toLowerCase().includes(q.toLowerCase())
        }
        filterPlaceholder="Filter tasks..."
      />
    </div>
  );
}

const meta: Meta<typeof SelectableList> = {
  title: "Foundations/SelectableList",
  component: SelectableList,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "A reusable table with checkbox selection and contextual bulk actions. Built on shadcn Table primitives. Supports filter, select-all, and typed column/action definitions.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SelectableList>;

export const LocaleList: Story = {
  name: "Locale List (Hide/Show/Remove)",
  render: () => <LocaleListDemo />,
};

export const TaskList: Story = {
  name: "Task List (Generic)",
  render: () => <TaskListDemo />,
};
