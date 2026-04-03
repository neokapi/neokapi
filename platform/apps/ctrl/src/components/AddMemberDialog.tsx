import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Input,
  Label,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui";
import { listUsers, addMemberToWorkspace } from "../api";

interface AddMemberDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
}

export function AddMemberDialog({ open, onOpenChange, workspaceId }: AddMemberDialogProps) {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [selectedUserId, setSelectedUserId] = useState("");
  const [selectedUserEmail, setSelectedUserEmail] = useState("");
  const [role, setRole] = useState("member");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  // Debounce search input.
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(search), 300);
    return () => clearTimeout(timer);
  }, [search]);

  const { data: users } = useQuery({
    queryKey: ["admin", "users", debouncedSearch],
    queryFn: () => listUsers({ q: debouncedSearch }),
    enabled: debouncedSearch.length >= 2,
  });

  const mutation = useMutation({
    mutationFn: () => addMemberToWorkspace(workspaceId, selectedUserId, role),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin", "workspace", workspaceId] });
      onOpenChange(false);
      setSearch("");
      setSelectedUserId("");
      setSelectedUserEmail("");
      setRole("member");
    },
  });

  const selectUser = (userId: string, email: string) => {
    setSelectedUserId(userId);
    setSelectedUserEmail(email);
    setSearch(email);
    setDebouncedSearch(""); // hide dropdown
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Add Member</DialogTitle>
          <DialogDescription>
            Search for a user by email and add them to this workspace.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="user-search">User (search by email)</Label>
            <div className="relative">
              <Input
                id="user-search"
                placeholder="user@example.com"
                value={search}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  setSearch(e.target.value);
                  if (selectedUserId) {
                    setSelectedUserId("");
                    setSelectedUserEmail("");
                  }
                }}
              />
              {users && users.length > 0 && !selectedUserId && debouncedSearch.length >= 2 && (
                <div className="absolute z-10 mt-1 w-full rounded-md border bg-popover shadow-md">
                  {users.map((u) => (
                    <button
                      key={u.id}
                      type="button"
                      className="flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-accent text-left"
                      onClick={() => selectUser(u.id, u.email)}
                    >
                      <span className="font-medium">{u.name || u.email}</span>
                      {u.name && <span className="text-muted-foreground">{u.email}</span>}
                    </button>
                  ))}
                </div>
              )}
              {users && users.length === 0 && debouncedSearch.length >= 2 && !selectedUserId && (
                <div className="absolute z-10 mt-1 w-full rounded-md border bg-popover px-3 py-2 shadow-md">
                  <p className="text-sm text-muted-foreground">No users found</p>
                </div>
              )}
            </div>
            {selectedUserEmail && (
              <p className="text-xs text-muted-foreground">Selected: {selectedUserEmail}</p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="role">Role</Label>
            <Select value={role} onValueChange={setRole}>
              <SelectTrigger>
                <SelectValue placeholder="Select role" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="owner">Owner</SelectItem>
                <SelectItem value="admin">Admin</SelectItem>
                <SelectItem value="member">Member</SelectItem>
                <SelectItem value="viewer">Viewer</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {mutation.error && (
            <p className="text-sm text-destructive">
              {mutation.error instanceof Error ? mutation.error.message : "Failed to add member"}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={!selectedUserId || mutation.isPending}
          >
            Add Member
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
