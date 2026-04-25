import { useEffect, useMemo, useRef, useState } from "react";
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
  type SlugCheckResponse,
} from "@neokapi/ui";

const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;

type SlugState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "ok" }
  | { kind: "invalid"; message: string }
  | { kind: "taken" }
  | { kind: "reserved" };

function reasonToMessage(reason: string | undefined): string {
  switch (reason) {
    case "invalid":
      return "Use 2–64 lowercase letters, numbers, and hyphens.";
    case "taken":
      return "That handle is already in use.";
    case "reserved":
      return "That handle was used recently and is reserved for a few weeks.";
    default:
      return "Handle is unavailable.";
  }
}

interface WelcomePageProps {
  onComplete: (slug: string) => void;
}

/**
 * WelcomePage — first-run handle picker.
 *
 * Renders after OIDC sign-in for users without a personal workspace. Live
 * server-side availability check prevents collisions with active workspaces
 * and recently renamed slugs (within their 30-day reservation window).
 */
export function WelcomePage({ onComplete }: WelcomePageProps) {
  const api = useApi();
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugState, setSlugState] = useState<SlugState>({ kind: "idle" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const checkSeq = useRef(0);

  // Load suggested slug from /auth/me/onboarding.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const status = await api.getOnboardingStatus();
        if (cancelled) return;
        setEmail(status.email);
        setDisplayName(status.display_name ?? "");
        if (status.suggested_slug) {
          setSlug(status.suggested_slug);
        }
      } catch (e: unknown) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : "Could not load onboarding state.");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api]);

  // Debounced availability check.
  useEffect(() => {
    if (!slug) {
      setSlugState({ kind: "idle" });
      return;
    }
    if (!SLUG_PATTERN.test(slug)) {
      setSlugState({ kind: "invalid", message: reasonToMessage("invalid") });
      return;
    }
    setSlugState({ kind: "checking" });
    const seq = ++checkSeq.current;
    const handle = setTimeout(() => {
      void (async () => {
        try {
          const result: SlugCheckResponse = await api.checkSlug(slug);
          if (seq !== checkSeq.current) return;
          if (result.available) {
            setSlugState({ kind: "ok" });
          } else if (result.reason === "taken") {
            setSlugState({ kind: "taken" });
          } else if (result.reason === "reserved") {
            setSlugState({ kind: "reserved" });
          } else {
            setSlugState({ kind: "invalid", message: reasonToMessage(result.reason) });
          }
        } catch {
          if (seq !== checkSeq.current) return;
          setSlugState({ kind: "idle" });
        }
      })();
    }, 250);
    return () => clearTimeout(handle);
  }, [slug, api]);

  const submitDisabled = useMemo(
    () => submitting || slugState.kind !== "ok" || slug.length < 2,
    [submitting, slugState, slug],
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await api.completeOnboarding(slug, displayName);
      onComplete(slug);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to complete onboarding.");
    } finally {
      setSubmitting(false);
    }
  };

  const slugHint = (() => {
    switch (slugState.kind) {
      case "checking":
        return (
          <span className="flex items-center gap-1 text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" /> Checking…
          </span>
        );
      case "ok":
        return (
          <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
            <CircleCheck className="h-3 w-3" /> Available
          </span>
        );
      case "invalid":
        return <span className="text-destructive">{slugState.message}</span>;
      case "taken":
        return <span className="text-destructive">{reasonToMessage("taken")}</span>;
      case "reserved":
        return <span className="text-destructive">{reasonToMessage("reserved")}</span>;
      default:
        return null;
    }
  })();

  return (
    <div className="mx-auto w-full max-w-lg pt-16">
      <Card>
        <CardHeader>
          <CardTitle>Welcome to Bowrain</CardTitle>
          <CardDescription>
            Pick a handle for your personal workspace. You can change it later from your profile.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-5" onSubmit={handleSubmit}>
            <div className="space-y-1.5">
              <Label htmlFor="welcome-email">Signed in as</Label>
              <Input id="welcome-email" value={email} disabled readOnly />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="welcome-display-name">Display name</Label>
              <Input
                id="welcome-display-name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="How you want to be addressed"
                autoComplete="name"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="welcome-slug">Handle</Label>
              <Input
                id="welcome-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value.toLowerCase())}
                placeholder="your-handle"
                autoComplete="off"
                spellCheck={false}
              />
              <p className="text-xs">{slugHint}</p>
              <p className="text-xs text-muted-foreground">
                Your personal workspace will live at{" "}
                <code>app.bowrain.cloud/{slug || "your-handle"}</code>.
              </p>
            </div>

            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            <Button type="submit" disabled={submitDisabled} className="w-full">
              {submitting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Setting up your workspace…
                </>
              ) : (
                "Continue"
              )}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
