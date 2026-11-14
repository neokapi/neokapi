// The signed-in person as the server knows them: name and email for the
// account screen and the avatar. The session holds what sign-in read, so the
// avatar draws at once and offline; the query refreshes it.
import type { User } from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";

import { useAuth } from "./auth";

export interface Profile {
  /** What keeps the avatar's colour: the user id, else the email. */
  key: string;
  /** The name to show: the person's name, else their email. */
  name: string;
  email?: string;
}

export function profileOf(
  user: Partial<User> | undefined,
  session: { userId?: string; email?: string; name?: string } | null,
): Profile {
  const email = user?.email || session?.email || undefined;
  const name = user?.name || session?.name || email || "";
  return { key: user?.id || session?.userId || email || name, name, email };
}

export function useProfile(): Profile {
  const { client, session } = useAuth();
  const me = useQuery({
    queryKey: ["me"],
    queryFn: () => client.me(),
    enabled: !!session,
    staleTime: 60 * 60 * 1000,
  });
  return profileOf(me.data, session);
}
