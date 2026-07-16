import { useNavigate } from "@tanstack/react-router";
import { WelcomePage } from "../../auth/WelcomePage";

export function WelcomeRoute() {
  const navigate = useNavigate();
  return (
    <WelcomePage
      onComplete={(slug) =>
        navigate({ to: "/$workspace", params: { workspace: slug }, replace: true })
      }
    />
  );
}
