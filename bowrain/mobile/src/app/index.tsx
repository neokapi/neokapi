// Entry: the workspace list when signed in, the sign-in screen when not.
import { Redirect } from "expo-router";

import { useAuth } from "../lib/auth";

export default function Index() {
  const { session } = useAuth();
  return <Redirect href={session ? "/workspaces" : "/sign-in"} />;
}
