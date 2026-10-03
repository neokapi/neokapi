/**
 * The prompts a person answers while a change is waiting on them: each open
 * prompt has exactly one commit waiting on it, and no commit waits on a prompt
 * nobody can answer.
 */
import { describe, it, expect } from "vite-plus/test";
import { act, render as renderWith, renderHook, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { useContentChanges, type CommitOutcome } from "../hooks/useContentChanges";
import { setTranslation } from "../api/contentChanges";
import { StaleChangeDialog } from "../components/editor/StaleChangeDialog";
import { createMockAdapter } from "../stories/mock-adapter";
import { mockRevision } from "../stories/mockContentChanges";
import type { BlockInfo, Workspace } from "../types/api";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

const block: BlockInfo = {
  id: "b1",
  source: "Hello",
  targets: { fr: { text: "Bonjour", status: "translated" } },
  translatable: true,
  has_spans: false,
  properties: {},
};

function setup() {
  const adapter = createMockAdapter([structuredClone(block)]);
  const wrapper = ({ children }: { children: ReactNode }) => (
    <ApiProvider adapter={adapter}>
      <WorkspaceProvider initialWorkspace={workspace}>{children}</WorkspaceProvider>
    </ApiProvider>
  );
  const hook = renderHook(() => useContentChanges("proj-1"), { wrapper });
  return { adapter, hook };
}

/** A save of `text` that met a translation someone else changed. */
function staleSave(
  adapter: ReturnType<typeof setup>["adapter"],
  commit: ReturnType<typeof useContentChanges>["commit"],
  text: string,
): Promise<CommitOutcome> {
  const rendered = mockRevision(block, "fr");
  adapter.editElsewhere("b1", "fr", `${text} (theirs)`);
  return commit(
    (ifMatch) => setTranslation("messages.json", block, "fr", { text }, ifMatch),
    rendered,
    { action: "save", locale: "fr", mine: text },
  );
}

describe("useContentChanges", () => {
  it("settles an open prompt as kept when another replaces it", async () => {
    const { adapter, hook } = setup();
    let first!: Promise<CommitOutcome>;
    act(() => {
      first = staleSave(adapter, hook.result.current.commit, "Salut");
    });
    await waitFor(() => expect(hook.result.current.staleDialog?.mine).toBe("Salut"));

    let second!: Promise<CommitOutcome>;
    act(() => {
      second = staleSave(adapter, hook.result.current.commit, "Coucou");
    });
    await expect(first).resolves.toMatchObject({ status: "kept" });
    await waitFor(() => expect(hook.result.current.staleDialog?.mine).toBe("Coucou"));

    act(() => hook.result.current.staleDialog?.onKeep());
    await expect(second).resolves.toMatchObject({ status: "kept" });
  });

  it("settles an open prompt as kept when the surface unmounts", async () => {
    const { adapter, hook } = setup();
    let pending!: Promise<CommitOutcome>;
    act(() => {
      pending = staleSave(adapter, hook.result.current.commit, "Salut");
    });
    await waitFor(() => expect(hook.result.current.staleDialog).not.toBeNull());

    hook.unmount();
    await expect(pending).resolves.toMatchObject({ status: "kept" });
  });

  it("answers a prompt once, however often it is clicked", async () => {
    const { adapter, hook } = setup();
    let pending!: Promise<CommitOutcome>;
    act(() => {
      pending = staleSave(adapter, hook.result.current.commit, "Salut");
    });
    await waitFor(() => expect(hook.result.current.staleDialog).not.toBeNull());
    const dialog = hook.result.current.staleDialog;
    act(() => {
      dialog?.onReapply();
      dialog?.onKeep();
    });
    await expect(pending).resolves.toMatchObject({ status: "applied" });
    expect(adapter.opsOf("set_content")).toHaveLength(2);
  });
});

describe("StaleChangeDialog", () => {
  const noop = () => {};
  const render = (ui: ReactNode) =>
    renderWith(
      <QueryClientProvider client={new QueryClient()}>
        <ApiProvider adapter={createMockAdapter([])}>
          <WorkspaceProvider initialWorkspace={workspace}>{ui}</WorkspaceProvider>
        </ApiProvider>
      </QueryClientProvider>,
    );

  it("offers the decision again on a translation that changed", () => {
    render(
      <StaleChangeDialog
        state={{
          action: "establish",
          locale: "fr",
          current: { rev: "r:0000000000000001", text: "Salut" },
          onReapply: noop,
          onKeep: noop,
        }}
      />,
    );
    expect(screen.getByTestId("stale-reapply").textContent).toBe("Approve this version");
  });

  it("offers only to close when the translation was removed", () => {
    render(
      <StaleChangeDialog
        state={{
          action: "establish",
          locale: "fr",
          current: { rev: "absent" },
          onReapply: noop,
          onKeep: noop,
        }}
      />,
    );
    expect(screen.queryByTestId("stale-reapply")).not.toBeInTheDocument();
    expect(screen.getByTestId("stale-keep").textContent).toBe("Close");
    expect(screen.getByText(/removed this translation/)).toBeInTheDocument();
  });

  it("still offers a save over a translation that was removed", () => {
    render(
      <StaleChangeDialog
        state={{
          action: "save",
          locale: "fr",
          mine: "Salut",
          current: { rev: "absent" },
          onReapply: noop,
          onKeep: noop,
        }}
      />,
    );
    expect(screen.getByTestId("stale-reapply").textContent).toBe("Save my version");
  });
});
