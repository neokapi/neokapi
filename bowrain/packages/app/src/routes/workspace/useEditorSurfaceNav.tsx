import { useNavigate, useParams } from "@tanstack/react-router";
import { EditorSurfaceTabs, type EditorSurface, useStream } from "@neokapi/ui";

/**
 * Shared hook that renders the cross-surface switcher (Pre-process / Translate
 * / Review) for the per-file editor routes, navigating between the three
 * sibling routes via TanStack Router. Keeps the wiring in one place so the
 * three routes don't each re-implement it.
 */
export function useEditorSurfaceNav(active: EditorSurface): React.ReactNode {
  const navigate = useNavigate();
  const { workspace, projectId, itemName } = useParams({ strict: false });
  const { activeStream } = useStream();

  const go = (surface: EditorSurface) => {
    if (surface === active || !projectId || !itemName) return;
    const path =
      surface === "translate"
        ? "/$workspace/p/$projectId/s/$stream/$itemName/translate"
        : surface === "review"
          ? "/$workspace/p/$projectId/s/$stream/$itemName/review"
          : "/$workspace/p/$projectId/s/$stream/$itemName/pre-process";
    void navigate({
      to: path,
      params: {
        workspace: workspace ?? "",
        projectId,
        stream: activeStream,
        itemName,
      },
    });
  };

  return <EditorSurfaceTabs active={active} onSelect={go} />;
}
