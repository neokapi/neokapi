import type { Meta, StoryObj } from "@storybook/react-vite";
import { cn } from "@neokapi/ui-primitives";

interface AdminUser {
  id: string;
  email: string;
  name: string;
  workspace_count: number;
  last_login: string | null;
  created_at: string;
}

interface UserTableProps {
  users: AdminUser[];
  loading: boolean;
  selectedUserId: string | null;
  onRowClick: (id: string) => void;
}

function UserTable({ users, loading, selectedUserId, onRowClick }: UserTableProps) {
  if (loading) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        Loading users...
      </div>
    );
  }

  if (users.length === 0) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        No users found.
      </div>
    );
  }

  return (
    <div className="rounded-lg border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-muted/50">
            <th className="px-4 py-2 text-left font-medium">Email</th>
            <th className="px-4 py-2 text-left font-medium">Name</th>
            <th className="px-4 py-2 text-right font-medium">Workspaces</th>
            <th className="px-4 py-2 text-left font-medium">Last Login</th>
            <th className="px-4 py-2 text-left font-medium">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {users.map((user) => (
            <tr
              key={user.id}
              onClick={() => onRowClick(user.id)}
              className={cn(
                "hover:bg-muted/30 cursor-pointer",
                selectedUserId === user.id && "bg-muted/50",
              )}
            >
              <td className="px-4 py-2 font-medium">{user.email}</td>
              <td className="px-4 py-2">{user.name}</td>
              <td className="px-4 py-2 text-right tabular-nums">{user.workspace_count}</td>
              <td className="px-4 py-2 text-muted-foreground">
                {user.last_login ? new Date(user.last_login).toLocaleDateString() : "Never"}
              </td>
              <td className="px-4 py-2 text-muted-foreground">
                {new Date(user.created_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

const meta: Meta<typeof UserTable> = {
  title: "Ctrl/UserTable",
  component: UserTable,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof UserTable>;

const sampleUsers: AdminUser[] = [
  {
    id: "u-1",
    email: "admin@acme.com",
    name: "Alice Admin",
    workspace_count: 3,
    last_login: "2026-03-19T10:30:00Z",
    created_at: "2025-01-15T00:00:00Z",
  },
  {
    id: "u-2",
    email: "bob@widget.io",
    name: "Bob Builder",
    workspace_count: 1,
    last_login: "2026-03-18T14:00:00Z",
    created_at: "2025-06-01T00:00:00Z",
  },
  {
    id: "u-3",
    email: "charlie@startup.co",
    name: "Charlie Dev",
    workspace_count: 2,
    last_login: null,
    created_at: "2026-02-20T00:00:00Z",
  },
  {
    id: "u-4",
    email: "diana@enterprise.com",
    name: "Diana Ops",
    workspace_count: 5,
    last_login: "2026-03-20T08:00:00Z",
    created_at: "2024-11-01T00:00:00Z",
  },
];

export const Default: Story = {
  args: { users: sampleUsers, loading: false, selectedUserId: null, onRowClick: () => {} },
};

export const Loading: Story = {
  args: { users: [], loading: true, selectedUserId: null, onRowClick: () => {} },
};

export const Empty: Story = {
  args: { users: [], loading: false, selectedUserId: null, onRowClick: () => {} },
};

export const WithSelection: Story = {
  args: { users: sampleUsers, loading: false, selectedUserId: "u-2", onRowClick: () => {} },
};
