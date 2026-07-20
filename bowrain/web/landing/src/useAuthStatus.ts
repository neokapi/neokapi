import { useEffect, useState } from "react";
import { whoamiUrl } from "./links";

// The signed-in identity the landing renders (display-only — no token ever
// crosses the origin boundary). `null` means signed-out / unknown.
export interface AuthStatus {
  name: string;
  email: string;
  avatarUrl: string;
}

// Shape of GET /api/v1/auth/whoami (trimmed, display-only server response).
interface WhoAmI {
  authenticated: boolean;
  name?: string;
  email?: string;
  avatar_url?: string;
}

// Non-HttpOnly hint cookie the app sets on the parent domain alongside the
// session, so the landing can render the correct CTA on first paint before the
// whoami fetch resolves (no flash). It carries no secret — just presence.
const HINT_COOKIE = "bowrain_signed_in";

function hasSignedInHint(): boolean {
  if (typeof document === "undefined") return false;
  return document.cookie.split("; ").some((c) => c === `${HINT_COOKIE}=1`);
}

// Resolves the visitor's app session for CTA purposes. Reads the no-flash hint
// cookie synchronously for the initial state, then confirms with a credentialed
// whoami fetch to the app API. Any network/parse error — or an unauthenticated
// answer — collapses to signed-out; the landing degrades to "Sign in".
//
// Returns { status, resolved }: `resolved` flips true once whoami answers, so
// the caller can prefer the confirmed state over the optimistic hint.
export function useAuthStatus(): { status: AuthStatus | null; resolved: boolean } {
  const [status, setStatus] = useState<AuthStatus | null>(() =>
    hasSignedInHint() ? { name: "", email: "", avatarUrl: "" } : null,
  );
  const [resolved, setResolved] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetch(whoamiUrl(), { credentials: "include" })
      .then((r) => (r.ok ? (r.json() as Promise<WhoAmI>) : null))
      .then((data) => {
        if (cancelled) return;
        setResolved(true);
        setStatus(
          data?.authenticated
            ? {
                name: data.name ?? "",
                email: data.email ?? "",
                avatarUrl: data.avatar_url ?? "",
              }
            : null,
        );
      })
      .catch(() => {
        if (cancelled) return;
        setResolved(true);
        setStatus(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { status, resolved };
}
