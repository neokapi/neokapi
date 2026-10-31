// Where the project screens lead in the web app: each ScreenTarget the app
// opens, as one of the project's routes.

import { useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ScreenTarget } from "@neokapi/project-screens";
import type { ScreensScope } from "./project-screens-port";

/** Opens a target the web app opens (bowrainOpens), and ignores any other. */
export function useScreenNavigate(scope: ScreensScope): (target: ScreenTarget) => void {
  const navigate = useNavigate();
  const { ws, projectId, stream } = scope;
  return useCallback(
    (target: ScreenTarget) => {
      const params = { workspace: ws, projectId, stream };
      switch (target.kind) {
        case "map":
          void navigate({ to: "/$workspace/p/$projectId/s/$stream/map", params });
          return;
        case "reader":
          void navigate({
            to: "/$workspace/p/$projectId/s/$stream/read/$",
            params: { ...params, _splat: target.path },
            search: target.block ? { block: target.block } : {},
          });
          return;
        case "rules":
          void navigate({
            to: "/$workspace/p/$projectId/s/$stream/rules",
            params,
            search: target.row ? { row: target.row } : {},
          });
          return;
        case "kept":
          void navigate({ to: "/$workspace/p/$projectId/s/$stream/rules", params, search: {} });
          return;
        case "blocks":
          void navigate({ to: "/$workspace/p/$projectId/s/$stream/source", params });
          return;
      }
    },
    [navigate, ws, projectId, stream],
  );
}
