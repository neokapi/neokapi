import { useNavigate, useParams } from "@tanstack/react-router";
import { JoinPage } from "../../auth/JoinPage";

export function JoinRoute() {
  const navigate = useNavigate();
  const { code } = useParams({ strict: false });

  return (
    <JoinPage
      code={code ?? ""}
      onJoined={() =>
        navigate({ to: "/$workspace", params: { workspace: "default" }, replace: true })
      }
    />
  );
}
