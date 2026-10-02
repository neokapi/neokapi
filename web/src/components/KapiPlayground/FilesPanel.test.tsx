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
