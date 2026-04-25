import { useEffect, useState } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
  Alert,
  AlertDescription,
  Loader2,
  CircleCheck,
  useApi,
} from "@neokapi/ui";

interface ConfirmEmailPageProps {
  token: string;
}

type Status =
  | { kind: "loading" }
  | { kind: "ok"; newEmail: string }
  | { kind: "error"; message: string };

/**
 * ConfirmEmailPage — handles `/account/confirm-email?token=…` links.
 *
 * Calls the server confirm endpoint, which writes the new email through to
 * Keycloak and revokes refresh tokens. The user is then signed out and
 * prompted to log back in with their new email.
 */
export function ConfirmEmailPage({ token }: ConfirmEmailPageProps) {
  const api = useApi();
  const [status, setStatus] = useState<Status>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    if (!token) {
      setStatus({ kind: "error", message: "Missing token. Open the link from your email." });
      return;
    }
    void (async () => {
      try {
        const resp = await api.confirmEmailChange(token);
        if (cancelled) return;
        setStatus({ kind: "ok", newEmail: resp.new_email });
      } catch (e: unknown) {
        if (cancelled) return;
        setStatus({
          kind: "error",
          message: e instanceof Error ? e.message : "Could not confirm the email change.",
        });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token, api]);

  return (
    <div className="mx-auto w-full max-w-lg pt-16">
      <Card>
        <CardHeader>
          <CardTitle>Confirm email change</CardTitle>
          <CardDescription>Verifying the link from your inbox.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {status.kind === "loading" && (
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <Loader2 className="h-4 w-4 animate-spin" /> Confirming…
            </div>
          )}

          {status.kind === "ok" && (
            <>
              <div className="flex items-start gap-3 rounded-md border border-emerald-500/40 bg-emerald-500/5 p-4">
                <CircleCheck className="mt-0.5 h-5 w-5 text-emerald-600 dark:text-emerald-400" />
                <div className="space-y-1 text-sm">
                  <p className="font-medium">
                    Your Bowrain email is now <code>{status.newEmail}</code>.
                  </p>
                  <p className="text-muted-foreground">
                    For your security we&apos;ve signed you out. Sign back in with the new email to
                    continue.
                  </p>
                </div>
              </div>
              <Button asChild className="w-full">
                <a href="/api/v1/auth/login">Sign in</a>
              </Button>
            </>
          )}

          {status.kind === "error" && (
            <Alert variant="destructive">
              <AlertDescription>{status.message}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
