import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import type { ChangeResult, ChangeSet } from "@neokapi/contract-types";

import { WorkspaceConflicts } from "../components/WorkspaceConflicts";
import type { ChangeClient } from "../lib/changes";
import { documentConflict, rebasedDocumentConflict } from "../stories/fixtures/keptConflicts";

const loose = { ...documentConflict, doc: "/home/me/handoff/loose.kpz!messages.json" };

describe("WorkspaceConflicts", () => {
  it("draws nothing outside the app, where no conflict is read", () => {
    const { container } = render(<WorkspaceConflicts />);
    expect(container).toBeEmptyDOMElement();
  });

  it("rebases a .kpz document named by its absolute path", async () => {
    const rebase = vi.fn(async () => ({ carried: 1, contested: 0 }));
    render(<WorkspaceConflicts conflicts={[loose]} rebase={rebase} />);
    expect(screen.getByText(/loose\.kpz!messages\.json holds another version/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Rebase onto the document" }));
    await waitFor(() => expect(screen.queryByTestId("kept-conflicts")).not.toBeInTheDocument());
    expect(rebase).toHaveBeenCalledWith(loose.doc, loose.edit);
  });

  it("decides a block a rebase left through the change service it is given", async () => {
    const sent: ChangeSet[] = [];
    const client: ChangeClient = {
      read: async () => null,
      apply: async (set) => {
        sent.push(set);
        return { status: "applied", ops: [], docs: [] } as unknown as ChangeResult;
      },
      describe: async () => null,
      history: async () => null,
    };
    render(
      <WorkspaceConflicts
        conflicts={[{ ...rebasedDocumentConflict, doc: loose.doc }]}
        client={client}
      />,
    );
    await userEvent.click(screen.getAllByRole("button", { name: "Keep this wording" })[0]);
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].ops[0]).toMatchObject({
      at: { doc: loose.doc, block: "greeting" },
      text: "Hello, edited on the desktop",
    });
  });
});
