import { describe, expect, it } from "vitest";
import { folderRemote } from "./folderremote.ts";
import type { FolderFile, FolderHandle } from "./folderremote.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

function domError(name: string, message: string): Error {
  const e = new Error(message);
  e.name = name;
  return e;
}

/**
 * A directory handle held in memory, with the File System Access API's
 * behaviour where the remote depends on it: a missing entry is NotFoundError,
 * a file opened as a directory is TypeMismatchError, and `move` renames a file
 * in its directory unless `movable` is false.
 */
function memoryFolder(
  name = "shared",
  movable = true,
): FolderHandle & { tree: Map<string, unknown> } {
  type Node = Map<string, unknown>;
  const fileHandle = (dir: Node, entry: string): FolderFile => {
    const handle: FolderFile = {
      kind: "file",
      get name() {
        return entry;
      },
      getFile: async () => {
        const data = dir.get(entry) as Uint8Array;
        return { arrayBuffer: async () => data.slice().buffer };
      },
      createWritable: async () => {
        let pending = new Uint8Array(0);
        return {
          write: async (data: Uint8Array) => {
            pending = data.slice();
          },
          close: async () => {
            dir.set(entry, pending);
          },
        };
      },
    };
    if (movable) {
      handle.move = async (to: string) => {
        dir.set(to, dir.get(entry));
        dir.delete(entry);
        entry = to;
      };
    }
    return handle;
  };
  const dirHandle = (node: Node, dirName: string): FolderHandle => ({
    kind: "directory",
    name: dirName,
    async getDirectoryHandle(child, options) {
      let next = node.get(child);
      if (next === undefined) {
        if (!options?.create) throw domError("NotFoundError", `${child} is not there`);
        next = new Map();
        node.set(child, next);
      }
      if (!(next instanceof Map)) throw domError("TypeMismatchError", `${child} is a file`);
      return dirHandle(next, child);
    },
    async getFileHandle(child, options) {
      const held = node.get(child);
      if (held === undefined) {
        if (!options?.create) throw domError("NotFoundError", `${child} is not there`);
        node.set(child, new Uint8Array(0));
      } else if (held instanceof Map) {
        throw domError("TypeMismatchError", `${child} is a directory`);
      }
      return fileHandle(node, child);
    },
    async removeEntry(child) {
      node.delete(child);
    },
    async *values() {
      for (const [child, v] of node) {
        yield v instanceof Map ? dirHandle(v, child) : fileHandle(node, child);
      }
    },
  });
  const tree: Node = new Map();
  return Object.assign(dirHandle(tree, name), { tree });
}

const segment = "log/w1/0000000000000000000000ab.jsonl";

describe("a folder as a context remote", () => {
  it("names itself after the folder", () => {
    expect(folderRemote(memoryFolder("team-context")).location).toBe("team-context");
    expect(folderRemote(memoryFolder(), "elsewhere").location).toBe("elsewhere");
  });

  it("holds nothing at first", async () => {
    const r = folderRemote(memoryFolder());
    await expect(r.list("log/")).resolves.toEqual([]);
    await expect(r.get(segment)).resolves.toBeNull();
  });

  it("writes objects at their layout paths and lists them by directory", async () => {
    const folder = memoryFolder();
    const r = folderRemote(folder);
    const blob = `blobs/${"a".repeat(64)}`;
    await r.put([
      { name: blob, data: enc.encode("payload") },
      { name: segment, data: enc.encode("{}\n") },
    ]);
    await expect(r.list("log/")).resolves.toEqual([segment]);
    await expect(r.list("blobs/")).resolves.toEqual([blob]);
    expect(dec.decode((await r.get(segment)) ?? new Uint8Array())).toBe("{}\n");
    // The folder holds the layout a file backend keeps on disk.
    const log = folder.tree.get("log") as Map<string, Map<string, Uint8Array>>;
    expect([...log.get("w1")!.keys()]).toEqual(["0000000000000000000000ab.jsonl"]);
  });

  it("creates an object once, and refuses other bytes under its name", async () => {
    const r = folderRemote(memoryFolder());
    await r.put([{ name: segment, data: enc.encode("first\n") }]);
    await r.put([{ name: segment, data: enc.encode("first\n") }]);
    await expect(r.put([{ name: segment, data: enc.encode("second\n") }])).rejects.toMatchObject({
      name: "ObjectExists",
    });
    expect(dec.decode((await r.get(segment))!)).toBe("first\n");
  });

  it("writes in one step where the browser cannot move a file", async () => {
    const folder = memoryFolder("shared", false);
    const r = folderRemote(folder);
    await r.put([{ name: segment, data: enc.encode("x\n") }]);
    await expect(r.list("log/")).resolves.toEqual([segment]);
  });

  it("leaves a write in progress out of a listing", async () => {
    const folder = memoryFolder();
    const r = folderRemote(folder);
    await r.put([{ name: segment, data: enc.encode("x\n") }]);
    const w1 = (folder.tree.get("log") as Map<string, Map<string, Uint8Array>>).get("w1")!;
    w1.set(".tmp-half", enc.encode("half"));
    await expect(r.list("log/")).resolves.toEqual([segment]);
  });

  it("passes on a withdrawn permission for the engine to report", async () => {
    const folder = memoryFolder();
    folder.getDirectoryHandle = async () => {
      throw domError("NotAllowedError", "permission withdrawn");
    };
    await expect(folderRemote(folder).list("log/")).rejects.toMatchObject({
      name: "NotAllowedError",
    });
  });
});
