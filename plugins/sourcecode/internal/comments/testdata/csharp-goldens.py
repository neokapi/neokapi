#!/usr/bin/env python3
"""Writes the comment spans the C# comment tests compare with.

The spans come from Roslyn, the C# compiler's own parser, published as
Microsoft.CodeAnalysis.CSharp and run by the program in csharp-comments/, which
shares no code with the grammar the plugin reads C# with. Each fixture under
corpus/csharp/ has a golden beside it, <fixture>.roslyn, holding the Roslyn
version, the fixture's sha256, and one line per comment: start and end byte
offsets, then the lengths of its opening and closing markers. Roslyn reports
offsets in UTF-16 code units, so each is converted to a byte offset. A comment's
opening marker is the longest of the markers the language declares that it
starts with: `///` or `//` for a line comment, and `/*` for a block.

Usage, from the repository root, with the .NET SDK installed:

  python3 plugins/sourcecode/internal/comments/testdata/csharp-goldens.py
"""

import hashlib
import pathlib
import subprocess
import tempfile

HERE = pathlib.Path(__file__).resolve().parent
CORPUS = HERE / "corpus" / "csharp"
PROJECT = HERE / "csharp-comments" / "csharp-comments.csproj"
ROSLYN = "Microsoft.CodeAnalysis.CSharp 4.14.0"


def byte_offsets(data: bytes) -> list[int]:
    """Maps each UTF-16 offset in the text Roslyn reads to its byte offset. Roslyn
    reads a file without its UTF-8 byte order mark, so offsets start after it."""
    bom = b"\xef\xbb\xbf"
    at = len(bom) if data.startswith(bom) else 0
    offsets = [at]
    for ch in data[at:].decode("utf-8"):
        at += len(ch.encode("utf-8"))
        offsets.extend([at] * (2 if ord(ch) > 0xFFFF else 1))
    return offsets


def main() -> None:
    fixtures = sorted(CORPUS.glob("*.txt"))
    with tempfile.TemporaryDirectory() as artifacts:
        subprocess.run(
            ["dotnet", "build", str(PROJECT), "--artifacts-path", artifacts, "-nologo", "-v", "q"],
            check=True,
            capture_output=True,
        )
        program = next(d for d in pathlib.Path(artifacts).rglob("csharp-comments.dll") if "bin" in d.parts)
        out = subprocess.run(
            ["dotnet", str(program), *map(str, fixtures)], check=True, capture_output=True, text=True
        ).stdout
    spans = {str(f): [] for f in fixtures}
    for line in out.splitlines():
        path, start, end, kind = line.split("\t")
        spans[path].append((int(start), int(end), kind))
    for fixture in fixtures:
        data = fixture.read_bytes()
        at = byte_offsets(data)
        lines = [
            f"# comment spans from Roslyn, {ROSLYN}",
            "# source authored",
            f"# sha256 {hashlib.sha256(data).hexdigest()}",
        ]
        for start, end, kind in sorted(spans[str(fixture)]):
            s, e = at[start], at[end]
            if kind == "line":
                opener = 3 if data[s : s + 3] == b"///" else 2
                lines.append(f"{s} {e} {opener} 0")
            else:
                lines.append(f"{s} {e} 2 2")
        fixture.with_name(fixture.name + ".roslyn").write_text("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
