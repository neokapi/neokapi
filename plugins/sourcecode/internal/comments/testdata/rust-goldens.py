#!/usr/bin/env python3
"""Writes the comment spans the Rust comment tests compare with.

The spans come from rustc's own lexer, published on crates.io as
ra-ap-rustc_lexer and run by the program in rust-lexer/, which shares no code
with the grammar the plugin reads Rust with. Each fixture under corpus/rust/
has a golden beside it, <fixture>.rustc, holding the lexer version, the
fixture's sha256, and one line per comment: start and end byte offsets, then
the lengths of its opening and closing markers.

rustc reads a file with CRLF line endings as if it had LF ones, so a line
comment ends before its carriage return. A comment's opening marker is the
longest of the markers the language declares that it starts with: `///`, `//!`
or `//` for a line comment, and `/*` for a block.

Usage, from the repository root, with cargo installed:

  python3 plugins/sourcecode/internal/comments/testdata/rust-goldens.py
"""

import hashlib
import os
import pathlib
import subprocess
import tempfile

HERE = pathlib.Path(__file__).resolve().parent
CORPUS = HERE / "corpus" / "rust"
LEXER = HERE / "rust-lexer"
LEXER_VERSION = "ra-ap-rustc_lexer 0.174.0"


def main() -> None:
    fixtures = sorted(CORPUS.glob("*.txt"))
    target = pathlib.Path(os.environ.get("CARGO_TARGET_DIR") or pathlib.Path(tempfile.gettempdir()) / "kapi-rust-lexer")
    subprocess.run(
        ["cargo", "build", "--release", "--quiet", "--manifest-path", str(LEXER / "Cargo.toml"), "--target-dir", str(target)],
        check=True,
    )
    out = subprocess.run(
        [str(target / "release" / "rust-lexer"), *map(str, fixtures)], check=True, capture_output=True, text=True
    ).stdout
    spans = {str(f): [] for f in fixtures}
    for line in out.splitlines():
        path, start, end, kind = line.split("\t")
        spans[path].append((int(start), int(end), kind))
    for fixture in fixtures:
        data = fixture.read_bytes()
        lines = [
            f"# comment spans from rustc's lexer, {LEXER_VERSION}",
            "# source authored",
            f"# sha256 {hashlib.sha256(data).hexdigest()}",
        ]
        for start, end, kind in spans[str(fixture)]:
            if kind == "line":
                if data[end - 1 : end] == b"\r":
                    end -= 1
                opener = 3 if data[start : start + 3] in (b"///", b"//!") else 2
                lines.append(f"{start} {end} {opener} 0")
            else:
                lines.append(f"{start} {end} 2 2")
        fixture.with_name(fixture.name + ".rustc").write_text("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
