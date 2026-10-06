// @vitest-environment jsdom
import React from "react";
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import FilesPanel from "../../../../packages/kapi-playground/src/FilesPanel";
import type { KapiRuntime } from "../../../../packages/kapi-playground/src/runtime";

vi.mock("../../../../packages/kapi-playground/src/FilePreview", () => ({ default: () => null }));
afterEach(cleanup);

it("uploads binary files without overwriting existing files and reports failed reads", async () => {
  const files = new Map<string, Uint8Array>([["/project/report.docx", new Uint8Array([1])]]);
  const runtime = {
    cwd: () => "/project",
    vol: {
      exists: (path: string) => path === "/project" || files.has(path),
      readdir: () => [...files.keys()].map((path) => path.split("/").pop()!),
      isDir: () => false,
      writeFile: (path: string, bytes: Uint8Array) => files.set(path, bytes),
    },
  } as unknown as KapiRuntime;
  const onChange = vi.fn();
  render(<FilesPanel runtime={runtime} refreshKey={0} onChange={onChange} />);
  expect(screen.getByRole("button", { name: "Upload files" })).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Choose files to upload"), {
    target: {
      files: [
        { name: "report.docx", arrayBuffer: async () => new Uint8Array([0, 255, 17]).buffer },
        {
          name: "unreadable.txt",
          arrayBuffer: async () => {
            throw new Error("Could not read file");
          },
        },
      ],
    },
  });
  expect(await screen.findByRole("status")).toHaveProperty(
    "textContent",
    "Added to /project: report (1).docx.",
  );
  expect(files.get("/project/report.docx")).toEqual(new Uint8Array([1]));
  expect(files.get("/project/report (1).docx")).toEqual(new Uint8Array([0, 255, 17]));
  expect(screen.getByRole("alert").textContent).toContain("unreadable.txt: Could not read file");
  expect(onChange).toHaveBeenCalledOnce();
});

it("offers to take the workspace over when another tab holds it, and redraws on a change", async () => {
  let storage: { kind: "opfs" | "memory"; reason?: string } = {
    kind: "memory",
    reason: "another-tab",
  };
  const listeners = new Set<() => void>();
  const runtime = {
    cwd: () => "/",
    get storage() {
      return storage;
    },
    onStorageChange: (fn: () => void) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    takeOver: vi.fn(async () => {
      storage = { kind: "opfs" };
      for (const fn of listeners) fn();
      return storage;
    }),
    vol: { exists: () => true, readdir: () => [], isDir: () => false },
  } as unknown as KapiRuntime;
  const onChange = vi.fn();
  const { rerender } = render(<FilesPanel runtime={runtime} refreshKey={0} onChange={onChange} />);
  expect(screen.getByText(/Another tab holds the workspace/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Use the workspace here" }));
  expect(await screen.findByRole("status")).toHaveProperty(
    "textContent",
    "This tab now holds the workspace kept in this browser.",
  );
  expect(runtime.takeOver).toHaveBeenCalledOnce();
  expect(onChange).toHaveBeenCalled();
  rerender(<FilesPanel runtime={runtime} refreshKey={1} onChange={onChange} />);
  expect(screen.queryByRole("button", { name: "Use the workspace here" })).toBeNull();
  expect(document.querySelector('[data-storage="opfs"]')?.textContent).toMatch(
    /kept in this browser/,
  );
});

it("syncs the project's context with a folder the person picks, and keeps the folder", async () => {
  const folder = { kind: "directory", name: "team-context" };
  const picker = vi.fn(async () => folder);
  vi.stubGlobal("showDirectoryPicker", picker);
  const runtime = {
    cwd: () => "/site/docs",
    storage: { kind: "opfs" },
    vol: {
      exists: (path: string) => ["/site/docs", "/site/kapi.yaml"].includes(path),
      readdir: () => [],
      isDir: () => false,
    },
    syncContext: vi.fn(async () => ({ pull: { merged: 2 }, push: { pushed: 3 } })),
  } as unknown as KapiRuntime;
  render(<FilesPanel runtime={runtime} refreshKey={0} onChange={vi.fn()} />);
  const button = screen.getByRole("button", { name: "Sync context with a folder" });
  fireEvent.click(button);
  expect(await screen.findByRole("status")).toHaveProperty(
    "textContent",
    "Synced the context of /site with team-context: pulled 2 and pushed 3 operations.",
  );
  expect(runtime.syncContext).toHaveBeenCalledWith(folder, { project: "/site" });
  fireEvent.click(button);
  await vi.waitFor(() => expect(runtime.syncContext).toHaveBeenCalledTimes(2));
  expect(picker).toHaveBeenCalledOnce();
  vi.unstubAllGlobals();
});
