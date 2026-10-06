import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import type { ChangeResult, ChangeSet } from "@neokapi/contract-types";

import { KeptConflicts } from "../components/KeptConflicts";
import type { ChangeClient } from "../lib/changes";
import {
  documentConflict,
  editConflict,
  fileConflict,
  rebasedDocumentConflict,
} from "../stories/fixtures/keptConflicts";

function client(status: "applied" | "refused" = "applied") {
  const sent: ChangeSet[] = [];
  const c: ChangeClient = {
    read: async () => null,
    apply: async (set) => {
      sent.push(set);
      return {
        status,
        ops: set.ops.map((op, i) => ({
          i,
          op: op.op,
          status,
          ...(status === "refused" ? { error: { code: "stale", message: "moved" } } : {}),
        })),
        docs: [],
      } as unknown as ChangeResult;
    },
    describe: async () => null,
    history: async () => null,
  };
  return { c, sent };
}

describe("KeptConflicts", () => {
  it("draws nothing when there is no conflict", () => {
    const { container } = render(<KeptConflicts tabID="t1" conflicts={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("keeps the held wording of an edit as a set_content guarded by the held revision", async () => {
    const { c, sent } = client();
    const release = vi.fn(async () => {});
    render(<KeptConflicts tabID="t1" conflicts={[editConflict]} client={c} release={release} />);
    await userEvent.click(screen.getByRole("button", { name: "Keep this wording" }));
    await waitFor(() => expect(screen.queryByTestId("kept-conflicts")).not.toBeInTheDocument());
    expect(sent).toHaveLength(1);
    expect(sent[0].ops[0]).toMatchObject({
      op: "set_content",
      at: { doc: "src/en.json", block: "title", edition: "nl" },
      if_match: "r:7e19d9b8516eac94",
      text: "Tijvenster",
    });
    expect(release).not.toHaveBeenCalled();
  });

  it("takes the other wording of an edit", async () => {
    const { c, sent } = client();
    render(<KeptConflicts tabID="t1" conflicts={[editConflict]} client={c} release={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: "Use this wording" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].ops[0]).toMatchObject({
      text: "Getijdenvenster",
      if_match: "r:7e19d9b8516eac94",
    });
  });

  it("writes a new wording", async () => {
    const { c, sent } = client();
    render(<KeptConflicts tabID="t1" conflicts={[editConflict]} client={c} release={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: "Write another wording" }));
    const editor = screen.getByLabelText("New wording");
    await userEvent.clear(editor);
    await userEvent.type(editor, "Getijvenster");
    await userEvent.click(screen.getByRole("button", { name: "Save this wording" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].ops[0]).toMatchObject({ text: "Getijvenster" });
  });

  it("keeps a file's wording by releasing the workspace's copy alone", async () => {
    const { c, sent } = client();
    const release = vi.fn(async () => {});
    render(<KeptConflicts tabID="t1" conflicts={[fileConflict]} client={c} release={release} />);
    await userEvent.click(screen.getAllByRole("button", { name: "Keep the file's wording" })[0]);
    await waitFor(() => expect(release).toHaveBeenCalledWith("locales/en.json", "fr", ["title"]));
    expect(sent).toHaveLength(0);
  });

  it("writes the kept wording into the file, then releases the workspace's copy", async () => {
    const { c, sent } = client();
    const release = vi.fn(async () => {});
    render(<KeptConflicts tabID="t1" conflicts={[fileConflict]} client={c} release={release} />);
    await userEvent.click(screen.getAllByRole("button", { name: "Write it into the file" })[0]);
    await waitFor(() => expect(release).toHaveBeenCalledWith("locales/en.json", "fr", ["title"]));
    expect(sent[0].ops[0]).toMatchObject({
      at: { doc: "locales/en.json", block: "title", edition: "fr" },
      if_match: "r:1a2b3c4d5e6f7081",
      text: "Fenêtre de marée",
    });
  });

  it("shows a stale refusal and releases nothing", async () => {
    const { c } = client("refused");
    const release = vi.fn(async () => {});
    render(<KeptConflicts tabID="t1" conflicts={[fileConflict]} client={c} release={release} />);
    await userEvent.click(screen.getAllByRole("button", { name: "Write it into the file" })[0]);
    expect(await screen.findByText(/changed since this conflict was read/)).toBeInTheDocument();
    expect(release).not.toHaveBeenCalled();
    expect(screen.getByTestId("kept-conflicts")).toBeInTheDocument();
  });

  it("offers a document's other version to rebase or discard, and lists no block", () => {
    render(<KeptConflicts tabID="t1" conflicts={[documentConflict]} rebase={vi.fn()} />);
    expect(screen.getByTestId("kept-conflict")).toHaveAttribute("data-kind", "document");
    expect(screen.getByText(/work\.kpz!messages\.json holds another version/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Rebase onto the document" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Discard this version" })).toBeEnabled();
    expect(screen.queryByTestId("kept-conflict-block")).not.toBeInTheDocument();
  });

  it("rebases a document's other version that leaves nothing contested", async () => {
    const rebase = vi.fn(async () => ({ carried: 2, contested: 0 }));
    render(<KeptConflicts tabID="t1" conflicts={[documentConflict]} rebase={rebase} />);
    await userEvent.click(screen.getByRole("button", { name: "Rebase onto the document" }));
    await waitFor(() => expect(screen.queryByTestId("kept-conflicts")).not.toBeInTheDocument());
    expect(rebase).toHaveBeenCalledWith("work.kpz!messages.json", "0pcn2v7k9wq1d4e8h3s5t6u0");
  });

  it("shows why a rebase was refused and keeps the conflict", async () => {
    const rebase = vi.fn(async () => ({ carried: 1, contested: 0, refused: "the gate failed" }));
    render(<KeptConflicts tabID="t1" conflicts={[documentConflict]} rebase={rebase} />);
    await userEvent.click(screen.getByRole("button", { name: "Rebase onto the document" }));
    expect(await screen.findByText("the gate failed")).toBeInTheDocument();
    expect(screen.getByTestId("kept-conflicts")).toBeInTheDocument();
  });

  it("discards a document's other version", async () => {
    const discard = vi.fn(async () => {});
    render(<KeptConflicts tabID="t1" conflicts={[documentConflict]} discard={discard} />);
    await userEvent.click(screen.getByRole("button", { name: "Discard this version" }));
    await waitFor(() => expect(screen.queryByTestId("kept-conflicts")).not.toBeInTheDocument());
    expect(discard).toHaveBeenCalledWith("work.kpz!messages.json", "0pcn2v7k9wq1d4e8h3s5t6u0");
  });

  it("decides a block a rebase left, naming the block's edition", async () => {
    const { c, sent } = client();
    render(<KeptConflicts tabID="t1" conflicts={[rebasedDocumentConflict]} client={c} />);
    expect(screen.getAllByTestId("kept-conflict-block")).toHaveLength(2);
    await userEvent.click(screen.getAllByRole("button", { name: "Use this wording" })[0]);
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].ops[0]).toEqual({
      op: "set_content",
      at: { doc: "work.kpz!messages.json", block: "greeting" },
      if_match: "r:3c5e7a9b1d2f4c6e",
      text: "Hello from the team",
    });
    await userEvent.click(screen.getByRole("button", { name: "Keep this wording" }));
    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[1].ops[0]).toMatchObject({
      at: { doc: "work.kpz!messages.json", block: "greeting", edition: "fr" },
      if_match: "r:8a6b4c2d0e1f3a5b",
      text: "Bonjour",
    });
    await waitFor(() => expect(screen.queryByTestId("kept-conflicts")).not.toBeInTheDocument());
  });

  it("keeps the document's wording in every block a rebase left", async () => {
    const discard = vi.fn(async () => {});
    render(<KeptConflicts tabID="t1" conflicts={[rebasedDocumentConflict]} discard={discard} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Keep the document's wording in every block left" }),
    );
    await waitFor(() => expect(screen.queryByTestId("kept-conflicts")).not.toBeInTheDocument());
    expect(discard).toHaveBeenCalledWith("work.kpz!messages.json", "0pcn2v7k9wq1d4e8h3s5t6u0");
  });
});
