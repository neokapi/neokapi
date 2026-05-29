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
  const { workspace, projectId, itemId } = useParams({ strict: false });
  const { activeStream } = useStream();

  const go = (surface: EditorSurface) => {
    if (surface === active || !projectId || !itemId) return;
    const path =
      surface === "translate"
        ? "/$workspace/p/$projectId/s/$stream/$itemId/translate"
        : surface === "review"
          ? "/$workspace/p/$projectId/s/$stream/$itemId/review"
          : "/$workspace/p/$projectId/s/$stream/$itemId/pre-process";
    void navigate({
      to: path,
      params: {
        workspace: workspace ?? "",
        projectId,
        stream: activeStream,
        itemId,
      },
    });
  };

  return <EditorSurfaceTabs active={active} onSelect={go} />;
}
