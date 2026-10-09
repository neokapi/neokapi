import { describe, expect, it } from "vitest";
import { parseLine, lexLine } from "./tokenize.ts";
import { resolvePath, runLine, type ShellHost, type ShellSinks } from "./shell.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

/** An in-memory host: a flat map of absolute paths, plus a scripted kapi. */
function makeHost(kapi?: (argv: string[], sinks: ShellSinks) => Promise<number> | number) {
  const files = new Map<string, Uint8Array>();
  const dirs = new Set<string>(["/", "/work"]);
  let cwd = "/work";
  const host: ShellHost = {
    runKapi: async (argv, sinks) => (kapi ? kapi(argv, sinks) : 0),
    readFile: (p) => {
      const f = files.get(p);
      if (!f) throw new Error(`ENOENT ${p}`);
      return f;
    },
    writeFile: (p, d) => {
      files.set(p, d);
    },
    exists: (p) => files.has(p) || dirs.has(p),
    isDir: (p) => dirs.has(p),
    readdir: (p) => {
      const prefix = p.replace(/\/$/, "") + "/";
      const names = new Set<string>();
      for (const k of [...files.keys(), ...dirs]) {
        if (k.startsWith(prefix) && k !== prefix) names.add(k.slice(prefix.length).split("/")[0]);
      }
      return [...names];
    },
    mkdirp: (p) => {
      const parts = p.split("/").filter(Boolean);
      let acc = "";
      for (const part of parts) {
        acc += "/" + part;
        dirs.add(acc);
      }
    },
    remove: (p) => {
      files.delete(p);
      dirs.delete(p);
    },
    cwd: () => cwd,
    chdir: (d) => {
      cwd = d;
    },
  };
  const write = (p: string, s: string) => files.set(p, enc.encode(s));
  const read = (p: string) => dec.decode(files.get(p));
  return { host, write, read, files };
}

function sinks() {
  const outChunks: string[] = [];
  const errChunks: string[] = [];
  return {
    sinks: { out: (s: string) => outChunks.push(s), err: (s: string) => errChunks.push(s) },
    out: () => outChunks.join(""),
    err: () => errChunks.join(""),
  };
}

describe("lexLine / parseLine", () => {
  it("groups quotes and drops them", () => {
    expect(parseLine(`ksed -i 's/a b/c d/' file.html`)).toEqual({
      segments: [{ argv: ["ksed", "-i", "s/a b/c d/", "file.html"] }],
    });
  });

  it("reads redirections and pipes", () => {
    expect(parseLine("kapi check --json > findings.json 2>/dev/null")).toEqual({
      segments: [
        {
          argv: ["kapi", "check", "--json"],
          stdout: { path: "findings.json", append: false },
          stderr: { path: "/dev/null", append: false },
        },
      ],
    });
    expect(parseLine("kapi apply dutch.json 2>&1 | tail -3")).toEqual({
      segments: [
        { argv: ["kapi", "apply", "dutch.json"], stderr: "stdout" },
        { argv: ["tail", "-3"] },
      ],
    });
    expect(parseLine("echo hi >> log.txt")).toEqual({
      segments: [{ argv: ["echo", "hi"], stdout: { path: "log.txt", append: true } }],
    });
  });

  it("keeps operators inside quotes literal", () => {
    const r = parseLine(`echo '{"a": "x > y | z"}' > d.jsonl`);
    expect(r).toEqual({
      segments: [
        { argv: ["echo", '{"a": "x > y | z"}'], stdout: { path: "d.jsonl", append: false } },
      ],
    });
  });

  it("reports unterminated quotes and dangling operators", () => {
    expect(lexLine("echo 'open")).toEqual({ error: "unterminated single quote" });
    expect(parseLine("kapi check >")).toEqual({ error: "syntax error: `>` needs a file" });
    expect(parseLine("| tail")).toEqual({ error: "syntax error near `|`" });
  });
});

describe("resolvePath", () => {
  it("folds dots against the working directory", () => {
    expect(resolvePath("/work", "a/../b/./c")).toBe("/work/b/c");
    expect(resolvePath("/work", "/x/y/..")).toBe("/x");
    expect(resolvePath("/", "..")).toBe("/");
  });
});

describe("runLine", () => {
  it("writes echo output to a file and reads it back with cat", async () => {
    const { host, read } = makeHost();
    const s = sinks();
    const code = await runLine(host, `echo '{"op":"term"}' > decisions.jsonl`, s.sinks);
    expect(code).toBe(0);
    expect(read("/work/decisions.jsonl")).toBe('{"op":"term"}\n');
    const s2 = sinks();
    await runLine(host, "cat decisions.jsonl", s2.sinks);
    expect(s2.out()).toBe('{"op":"term"}\n');
  });

  it("hands unknown verbs to kapi, stripping a leading kapi", async () => {
    const seen: string[][] = [];
    const { host } = makeHost((argv, sk) => {
      seen.push(argv);
      sk.out("ok\n");
      return 3;
    });
    const s = sinks();
    expect(await runLine(host, "kapi check --ship", s.sinks)).toBe(3);
    expect(await runLine(host, "ksed -i 's/a/b/' f.md", s.sinks)).toBe(3);
    expect(seen).toEqual([
      ["check", "--ship"],
      ["ksed", "-i", "s/a/b/", "f.md"],
    ]);
    expect(s.out()).toBe("ok\nok\n");
  });

  it("pipes kapi output through tail and head, merging stderr with 2>&1", async () => {
    const { host } = makeHost((_argv, sk) => {
      sk.out("one\ntwo\nthree\n");
      sk.err("four\n");
      return 0;
    });
    const a = sinks();
    await runLine(host, "kapi apply x.json | tail -2", a.sinks);
    expect(a.out()).toBe("two\nthree\n");
    expect(a.err()).toBe("four\n");
    const b = sinks();
    await runLine(host, "kapi apply x.json 2>&1 | tail -2", b.sinks);
    expect(b.out()).toBe("three\nfour\n");
    expect(b.err()).toBe("");
    const c = sinks();
    await runLine(host, "kapi x | head -n 1", c.sinks);
    expect(c.out()).toBe("one\n");
  });

  it("redirects stderr to /dev/null and keeps the exit code", async () => {
    const { host, read } = makeHost((_argv, sk) => {
      sk.out('{"pass":false}\n');
      sk.err("Error: quality gate failed\n");
      return 3;
    });
    const s = sinks();
    expect(await runLine(host, "kapi check --json > findings.json 2>/dev/null", s.sinks)).toBe(3);
    expect(read("/work/findings.json")).toBe('{"pass":false}\n');
    expect(s.err()).toBe("");
    expect(s.out()).toBe("");
  });

  it("greps with -A and reports no match as exit 1", async () => {
    const { host, write } = makeHost();
    write("/work/notes.txt", "alpha\nberth\nbeta\ngamma\n");
    const s = sinks();
    expect(await runLine(host, "grep -A1 berth notes.txt", s.sinks)).toBe(0);
    expect(s.out()).toBe("berth\nbeta\n");
    const t = sinks();
    expect(await runLine(host, "grep nothing notes.txt", t.sinks)).toBe(1);
    const u = sinks();
    expect(await runLine(host, "grep -c -i A notes.txt", u.sinks)).toBe(0);
    expect(u.out()).toBe("3\n");
  });

  it("printf repeats its format over the arguments", async () => {
    const { host, read } = makeHost();
    const s = sinks();
    await runLine(host, `printf '%s\\n%s\\n' '{"a":1}' '{"b":2}' > two.jsonl`, s.sinks);
    expect(read("/work/two.jsonl")).toBe('{"a":1}\n{"b":2}\n');
  });

  it("lists, makes and changes directories", async () => {
    const { host, write } = makeHost();
    write("/work/a.md", "x");
    const s = sinks();
    await runLine(host, "mkdir -p docs/api", s.sinks);
    await runLine(host, "ls", s.sinks);
    expect(s.out()).toBe("a.md  docs/\n");
    await runLine(host, "cd docs/api", s.sinks);
    const p = sinks();
    await runLine(host, "pwd", p.sinks);
    expect(p.out()).toBe("/work/docs/api\n");
    const bad = sinks();
    expect(await runLine(host, "cd nowhere", bad.sinks)).toBe(1);
    expect(bad.err()).toContain("No such directory");
  });

  it("refuses to pipe into kapi", async () => {
    const { host } = makeHost(() => 0);
    const s = sinks();
    expect(await runLine(host, "echo x | kapi apply", s.sinks)).toBe(1);
    expect(s.err()).toContain("cannot pipe into kapi");
  });
});
