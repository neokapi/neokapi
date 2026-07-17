import { useNavigate, useSearch } from "@tanstack/react-router";
import { WelcomePage } from "../../auth/WelcomePage";

/** A safe internal return path, or null. */
function safeReturnTo(raw: unknown): string | null {
  if (typeof raw !== "string") return null;
  // Internal, absolute-path only: no protocol-relative or authority tricks.
  if (!raw.startsWith("/") || raw.startsWith("//") || /[\\@]/.test(raw)) return null;
  return raw;
}

export function WelcomeRoute() {
  const navigate = useNavigate();
  const { return_to } = useSearch({ strict: false }) as { return_to?: string };
  const returnTo = safeReturnTo(return_to);
  return (
    <WelcomePage
      onComplete={(slug) => {
        if (returnTo) {
          // A hard navigation re-bootstraps the app with the fresh workspace,
          // and carries any query string (e.g. installation_id) intact.
          window.location.assign(returnTo);
          return;
        }
        void navigate({ to: "/$workspace", params: { workspace: slug }, replace: true });
      }}
    />
  );
}
