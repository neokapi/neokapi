import { useNavigate, useParams } from "@tanstack/react-router";
import { ClaimPage } from "../../auth/ClaimPage";

export function ClaimRoute() {
  const navigate = useNavigate();
  const { token } = useParams({ strict: false });

  return (
    <ClaimPage
      token={token ?? ""}
      onClaimed={() =>
        navigate({ to: "/$workspace", params: { workspace: "default" }, replace: true })
      }
    />
  );
}
