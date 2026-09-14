#!/usr/bin/env python3
"""Writes the comment spans the Java comment tests compare with.

The spans come from javac's own tokenizer, run by java-comments/JavaComments.java,
which shares no code with the grammar the plugin reads Java with. Each fixture
under corpus/java/ has a golden beside it, <fixture>.javac, holding the JDK
version, the file the fixture was copied from, the fixture's sha256, and one
line per comment: start and end byte offsets, then the lengths of its opening
and closing markers. javac reports offsets in UTF-16 code units, so each is
converted to a byte offset.

Usage, from the repository root, with a JDK 17 or later:

  python3 plugins/sourcecode/internal/comments/testdata/java-goldens.py
      regenerates every golden
  python3 plugins/sourcecode/internal/comments/testdata/java-goldens.py --add <file>...
      copies files into the corpus and writes their goldens
"""

import hashlib
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
CORPUS = HERE / "corpus" / "java"
PROGRAM = HERE / "java-comments" / "JavaComments.java"
EXPORTS = [
    "--add-exports", "jdk.compiler/com.sun.tools.javac.parser=ALL-UNNAMED",
    "--add-exports", "jdk.compiler/com.sun.tools.javac.util=ALL-UNNAMED",
]


def byte_offsets(data: bytes) -> list[int]:
    """Maps each UTF-16 offset in the decoded text to its byte offset."""
    offsets = [0]
    at = 0
    for ch in data.decode("utf-8"):
        at += len(ch.encode("utf-8"))
        units = 2 if ord(ch) > 0xFFFF else 1
        offsets.extend([at] * units)
    return offsets


def jdk_version() -> str:
    out = subprocess.run(["java", "-version"], check=True, capture_output=True, text=True)
    return out.stderr.splitlines()[0]


def source_of(golden: pathlib.Path) -> str:
    if golden.exists():
        for line in golden.read_text().splitlines():
            if line.startswith("# source "):
                return line[len("# source "):]
    return "authored"


def write(fixtures: list[pathlib.Path], sources: dict[pathlib.Path, str]) -> None:
    version = jdk_version()
    out = subprocess.run(
        ["java", *EXPORTS, str(PROGRAM), *map(str, fixtures)], check=True, capture_output=True, text=True
    ).stdout
    spans = {str(f): [] for f in fixtures}
    for line in out.splitlines():
        path, start, end, style = line.split("\t")
        spans[path].append((int(start), int(end), style))
    for fixture in fixtures:
        data = fixture.read_bytes()
        at = byte_offsets(data)
        lines = [
            f"# comment spans from javac's tokenizer, {version}",
            f"# source {sources.get(fixture) or source_of(fixture.with_name(fixture.name + '.javac'))}",
            f"# sha256 {hashlib.sha256(data).hexdigest()}",
        ]
        for start, end, style in spans[str(fixture)]:
            close = 0 if style == "LINE" else 2
            lines.append(f"{at[start]} {at[end]} 2 {close}")
        fixture.with_name(fixture.name + ".javac").write_text("\n".join(lines) + "\n")


def main() -> None:
    args = sys.argv[1:]
    if args[:1] == ["--add"]:
        CORPUS.mkdir(parents=True, exist_ok=True)
        sources = {}
        for arg in args[1:]:
            path = pathlib.Path(arg).resolve()
            top = pathlib.Path(
                subprocess.run(["git", "rev-parse", "--show-toplevel"], cwd=path.parent, check=True, capture_output=True, text=True).stdout.strip()
            )
            fixture = CORPUS / ("__".join(path.parts[-2:]) + ".txt")
            fixture.write_bytes(path.read_bytes())
            sources[fixture] = f"{top.name}/{path.relative_to(top).as_posix()}"
            print(f"added {fixture.name} from {sources[fixture]}")
        write(sorted(sources), sources)
        return
    write(sorted(CORPUS.glob("*.txt")), {})


if __name__ == "__main__":
    main()
