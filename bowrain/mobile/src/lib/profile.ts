// The signed-in person as the server knows them: name and email for the
// account screen and the avatar. The session holds what sign-in read, so the
// avatar draws at once and offline; the query refreshes it.
import type { User } from "@neokapi/bowrain-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useAuth } from "./auth";

export interface Profile {
  /** What keeps the avatar's colour: the user id, else the email. */
  key: string;
  /** The name to show, as the server answers it: the name the person set, else their handle. */
  name: string;
  /** The name the person set themselves, if any. */
  ownName?: string;
  email?: string;
}

export function profileOf(
  user: Partial<User> | undefined,
  session: { userId?: string; email?: string; name?: string } | null,
): Profile {
  const email = user?.email || session?.email || undefined;
  const name =
    user?.shown_name || user?.name || session?.name || (email ? email.split("@")[0] : "");
  const profile: Profile = { key: user?.id || session?.userId || email || name, name, email };
  if (user?.name) profile.ownName = user.name;
  return profile;
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

/** Saves the name the person set; an empty name clears it. */
export function useSetName() {
  const { client } = useAuth();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => client.updateMe({ name }),
    onSuccess: (user) => {
      qc.setQueryData(["me"], user);
      // Feed entries carry names, and the person's own may have changed.
      void qc.invalidateQueries({ queryKey: ["feed"] });
    },
  });
}
