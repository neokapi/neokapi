// What the account screen's About section shows: the app's version and build,
// and the EAS Update it runs. expo-updates is read only when its native
// module is in the binary, so a development build made before it was added
// shows no update rather than failing to open the screen.
import * as Application from "expo-application";
import Constants from "expo-constants";

type UpdatesModule = typeof import("expo-updates");

function loadUpdates(): UpdatesModule | null {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require("expo-updates") as UpdatesModule;
  } catch {
    return null;
  }
}

export interface AppInfo {
  version: string;
  build: string | null;
  /** The running update's id; null when the build runs its own bundle. */
  updateId: string | null;
  /** When the running update was published; null for the build's own bundle. */
  updateCreatedAt: Date | null;
  /** Whether the binary takes updates at all. */
  updatesEnabled: boolean;
  channel: string | null;
}

export function appInfo(): AppInfo {
  const updates = loadUpdates();
  const enabled = updates?.isEnabled ?? false;
  return {
    version: Application.nativeApplicationVersion ?? Constants.expoConfig?.version ?? "",
    build: Application.nativeBuildVersion,
    updateId: enabled && updates && !updates.isEmbeddedLaunch ? updates.updateId : null,
    updateCreatedAt:
      enabled && updates && !updates.isEmbeddedLaunch ? (updates.createdAt ?? null) : null,
    updatesEnabled: enabled,
    channel: enabled ? (updates?.channel ?? null) : null,
  };
}
