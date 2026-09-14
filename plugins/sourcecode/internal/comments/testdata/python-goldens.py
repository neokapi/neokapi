#!/usr/bin/env python3
"""Writes the comment spans the Python comment tests compare with.

The spans come from the standard library's tokenize module, which shares no
code with the grammar the plugin reads Python with, so the Go tests compare two
independent readings of the same bytes and need no Python of their own. Each
fixture under corpus/python/ has a golden beside it, <fixture>.tokenize,
holding the Python version, the file the fixture was copied from, the fixture's
sha256, and one line per comment: start and end byte offsets, then the lengths
of its opening and closing markers. tokenize reports a column in characters, so
each is converted to a byte offset on its line.

Usage, from the repository root:

  python3 plugins/sourcecode/internal/comments/testdata/python-goldens.py
      regenerates every golden
  python3 plugins/sourcecode/internal/comments/testdata/python-goldens.py --add <file>...
      copies repository files into the corpus and writes their goldens
"""

import hashlib
import io
import pathlib
import subprocess
import sys
import tokenize

HERE = pathlib.Path(__file__).resolve().parent
CORPUS = HERE / "corpus" / "python"


def spans(data: bytes):
    lines = data.splitlines(keepends=True)
    starts = [0]
    for line in lines:
        starts.append(starts[-1] + len(line))

    def offset(row: int, col: int) -> int:
        return starts[row - 1] + len(lines[row - 1].decode("utf-8")[:col].encode("utf-8"))

    return [
        (offset(*tok.start), offset(*tok.end), 1, 0)
        for tok in tokenize.tokenize(io.BytesIO(data).readline)
        if tok.type == tokenize.COMMENT
    ]


def source_of(golden: pathlib.Path) -> str:
    if golden.exists():
        for line in golden.read_text().splitlines():
            if line.startswith("# source "):
                return line[len("# source "):]
    return "authored"


def write(fixture: pathlib.Path, source: str) -> None:
    data = fixture.read_bytes()
    lines = [
        f"# comment spans from tokenize, Python {sys.version.split()[0]}",
        f"# source {source}",
        f"# sha256 {hashlib.sha256(data).hexdigest()}",
    ]
    lines += [" ".join(map(str, span)) for span in spans(data)]
    fixture.with_name(fixture.name + ".tokenize").write_text("\n".join(lines) + "\n")


def main() -> None:
    args = sys.argv[1:]
    if args[:1] == ["--add"]:
        root = pathlib.Path(
            subprocess.run(["git", "rev-parse", "--show-toplevel"], cwd=HERE, check=True, capture_output=True, text=True).stdout.strip()
        )
        CORPUS.mkdir(parents=True, exist_ok=True)
        for arg in args[1:]:
            path = pathlib.Path(arg).resolve()
            rel = path.relative_to(root).as_posix()
            fixture = CORPUS / ("__".join(rel.split("/")[-2:]) + ".txt")
            fixture.write_bytes(path.read_bytes())
            write(fixture, rel)
            print(f"added {fixture.relative_to(root)}")
        return
    for fixture in sorted(CORPUS.glob("*.txt")):
        write(fixture, source_of(fixture.with_name(fixture.name + ".tokenize")))


if __name__ == "__main__":
    main()
