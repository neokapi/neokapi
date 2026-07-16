import { useSearch } from "@tanstack/react-router";
import { ConfirmEmailPage } from "../../auth/ConfirmEmailPage";

export function ConfirmEmailRoute() {
  const { token } = useSearch({ strict: false }) as { token?: string };
  return <ConfirmEmailPage token={token ?? ""} />;
}
