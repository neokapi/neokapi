import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Label,
  Alert,
  AlertDescription,
  Loader2,
  CircleCheck,
  useApi,
} from "@neokapi/ui";
import { currentUserQueryOptions } from "../queries";

/**
 * ProfileEmailCard — Bowrain-managed email change.
 *
 * The user enters a new address; the server persists a hashed token, then
 * mails a confirm link to the new address. Verification is what proves
 * mailbox ownership; only on confirm does Keycloak + the local DB get the
 * new value, and refresh tokens are revoked so the user must sign back in.
 */
export function ProfileEmailCard() {
  const api = useApi();
  const queryClient = useQueryClient();
  const { data: user } = useQuery(currentUserQueryOptions(api));
  const [editing, setEditing] = useState(false);
  const [newEmail, setNewEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<{ newEmail: string; expiresAt: string } | null>(null);

  if (!user) {
    return null;
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const resp = await api.requestEmailChange(newEmail.trim().toLowerCase());
      setPending({ newEmail: resp.new_email, expiresAt: resp.expires_at });
      setEditing(false);
      setNewEmail("");
      void queryClient.invalidateQueries({ queryKey: ["currentUser"] });
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to send verification email.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Email</CardTitle>
        <CardDescription>
          Used for sign-in and account notifications. Changing it requires verifying the new
          address.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <div className="text-sm">
            <div className="font-medium">{user.email}</div>
            <div className="text-muted-foreground text-xs">Current email on file</div>
          </div>
          {!editing && (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              Change email
            </Button>
          )}
        </div>

        {pending && (
          <Alert>
            <CircleCheck className="h-4 w-4" />
            <AlertDescription>
              We sent a verification link to <strong>{pending.newEmail}</strong>. Click it to finish
              the change. The link expires {new Date(pending.expiresAt).toLocaleString()}.
            </AlertDescription>
          </Alert>
        )}

        {editing && (
          <form className="space-y-3 border-t pt-4" onSubmit={handleSubmit}>
            <div className="space-y-1.5">
              <Label htmlFor="profile-new-email">New email</Label>
              <Input
                id="profile-new-email"
                type="email"
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
                placeholder="you@new.example"
                autoComplete="email"
                required
              />
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <div className="flex gap-2">
              <Button type="submit" disabled={submitting || !newEmail}>
                {submitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Sending verification…
                  </>
                ) : (
                  "Send verification"
                )}
              </Button>
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  setEditing(false);
                  setNewEmail("");
                  setError("");
                }}
                disabled={submitting}
              >
                Cancel
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
