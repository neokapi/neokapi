#!/usr/bin/env python3
"""Writes the comment spans the C and C++ comment tests compare with.

The spans come from libclang, the C and C++ compiler's own lexer, reached through
its C API with ctypes, which shares no code with the grammars the plugin reads C
and C++ with. Each fixture under corpus/c/ and corpus/cpp/ has a golden beside
it, <fixture>.clang, holding the clang version, the file the fixture was copied
from, the fixture's sha256, and one line per comment: start and end byte
offsets, then the lengths of its opening and closing markers. A comment's opening
marker is the longest of the markers the language declares that it starts with:
`///`, `//!` or `//` for a line comment, and `/*` for a block.

libclang lexes every line of a file, the lines of a skipped #if branch included,
so a comment there is a comment too.

Usage, from the repository root, with libclang installed. LIBCLANG names the
library when it is not in a usual place:

  python3 plugins/sourcecode/internal/comments/testdata/clang-goldens.py
      regenerates every golden
  python3 plugins/sourcecode/internal/comments/testdata/clang-goldens.py --add c|cpp <file>...
      copies repository files into the corpus and writes their goldens
"""

import ctypes
import glob
import hashlib
import os
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
CORPUS = HERE / "corpus"
LANGUAGES = {"c": ("c", "-std=c17"), "cpp": ("c++", "-std=c++20")}
CANDIDATES = [
    "/Library/Developer/CommandLineTools/usr/lib/libclang.dylib",
    "/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/lib/libclang.dylib",
    *sorted(glob.glob("/usr/lib/llvm-*/lib/libclang.so*"), reverse=True),
    *sorted(glob.glob("/usr/lib/x86_64-linux-gnu/libclang*.so*"), reverse=True),
]


class CXString(ctypes.Structure):
    _fields_ = [("data", ctypes.c_void_p), ("flags", ctypes.c_uint)]


class CXSourceLocation(ctypes.Structure):
    _fields_ = [("ptr_data", ctypes.c_void_p * 2), ("int_data", ctypes.c_uint)]


class CXSourceRange(ctypes.Structure):
    _fields_ = [("ptr_data", ctypes.c_void_p * 2), ("begin_int_data", ctypes.c_uint), ("end_int_data", ctypes.c_uint)]


class CXToken(ctypes.Structure):
    _fields_ = [("int_data", ctypes.c_uint * 4), ("ptr_data", ctypes.c_void_p)]


CXToken_Comment = 4
CXTranslationUnit_DetailedPreprocessingRecord = 0x01


def load() -> ctypes.CDLL:
    path = os.environ.get("LIBCLANG") or next((p for p in CANDIDATES if os.path.exists(p)), None)
    if not path:
        raise SystemExit("no libclang found; set LIBCLANG to its path")
    lib = ctypes.CDLL(path)
    lib.clang_createIndex.restype = ctypes.c_void_p
    lib.clang_createIndex.argtypes = [ctypes.c_int, ctypes.c_int]
    lib.clang_parseTranslationUnit.restype = ctypes.c_void_p
    lib.clang_parseTranslationUnit.argtypes = [ctypes.c_void_p, ctypes.c_char_p, ctypes.POINTER(ctypes.c_char_p), ctypes.c_int, ctypes.c_void_p, ctypes.c_uint, ctypes.c_uint]
    lib.clang_disposeTranslationUnit.argtypes = [ctypes.c_void_p]
    lib.clang_getFile.restype = ctypes.c_void_p
    lib.clang_getFile.argtypes = [ctypes.c_void_p, ctypes.c_char_p]
    lib.clang_getLocationForOffset.restype = CXSourceLocation
    lib.clang_getLocationForOffset.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_uint]
    lib.clang_getRange.restype = CXSourceRange
    lib.clang_getRange.argtypes = [CXSourceLocation, CXSourceLocation]
    lib.clang_tokenize.argtypes = [ctypes.c_void_p, CXSourceRange, ctypes.POINTER(ctypes.POINTER(CXToken)), ctypes.POINTER(ctypes.c_uint)]
    lib.clang_disposeTokens.argtypes = [ctypes.c_void_p, ctypes.POINTER(CXToken), ctypes.c_uint]
    lib.clang_getTokenKind.argtypes = [CXToken]
    lib.clang_getTokenExtent.restype = CXSourceRange
    lib.clang_getTokenExtent.argtypes = [ctypes.c_void_p, CXToken]
    lib.clang_getRangeStart.restype = CXSourceLocation
    lib.clang_getRangeStart.argtypes = [CXSourceRange]
    lib.clang_getRangeEnd.restype = CXSourceLocation
    lib.clang_getRangeEnd.argtypes = [CXSourceRange]
    lib.clang_getFileLocation.argtypes = [CXSourceLocation, ctypes.POINTER(ctypes.c_void_p), ctypes.POINTER(ctypes.c_uint), ctypes.POINTER(ctypes.c_uint), ctypes.POINTER(ctypes.c_uint)]
    lib.clang_getClangVersion.restype = CXString
    lib.clang_getCString.restype = ctypes.c_char_p
    lib.clang_getCString.argtypes = [CXString]
    return lib


def spans(lib: ctypes.CDLL, index: int, path: pathlib.Path, language: str) -> list[tuple[int, int]]:
    """Returns the byte span of every comment libclang lexes in the file."""
    lang, std = LANGUAGES[language]
    size = path.stat().st_size
    args = (ctypes.c_char_p * 3)(b"-x", lang.encode(), std.encode())
    tu = lib.clang_parseTranslationUnit(index, str(path).encode(), args, 3, None, 0, CXTranslationUnit_DetailedPreprocessingRecord)
    if not tu:
        raise SystemExit(f"libclang could not read {path}")
    try:
        file = lib.clang_getFile(tu, str(path).encode())
        whole = lib.clang_getRange(lib.clang_getLocationForOffset(tu, file, 0), lib.clang_getLocationForOffset(tu, file, size))
        tokens = ctypes.POINTER(CXToken)()
        count = ctypes.c_uint()
        lib.clang_tokenize(tu, whole, ctypes.byref(tokens), ctypes.byref(count))

        def offset(location: CXSourceLocation) -> int:
            out = ctypes.c_uint()
            lib.clang_getFileLocation(location, None, None, None, ctypes.byref(out))
            return out.value

        out = []
        for i in range(count.value):
            if lib.clang_getTokenKind(tokens[i]) == CXToken_Comment:
                extent = lib.clang_getTokenExtent(tu, tokens[i])
                out.append((offset(lib.clang_getRangeStart(extent)), offset(lib.clang_getRangeEnd(extent))))
        lib.clang_disposeTokens(tu, tokens, count)
        return sorted(out)
    finally:
        lib.clang_disposeTranslationUnit(tu)


def source_of(golden: pathlib.Path) -> str:
    if golden.exists():
        for line in golden.read_text().splitlines():
            if line.startswith("# source "):
                return line[len("# source "):]
    return "authored"


def write(lib: ctypes.CDLL, index: int, fixture: pathlib.Path, language: str, source: str) -> None:
    data = fixture.read_bytes()
    lines = [
        f"# comment spans from libclang, {lib.clang_getCString(lib.clang_getClangVersion()).decode()}",
        f"# source {source}",
        f"# sha256 {hashlib.sha256(data).hexdigest()}",
    ]
    for start, end in spans(lib, index, fixture, language):
        if data[start : start + 2] == b"//":
            opener = 3 if data[start : start + 3] in (b"///", b"//!") else 2
            lines.append(f"{start} {end} {opener} 0")
        else:
            lines.append(f"{start} {end} 2 2")
    fixture.with_name(fixture.name + ".clang").write_text("\n".join(lines) + "\n")


def main() -> None:
    lib = load()
    index = lib.clang_createIndex(0, 0)
    args = sys.argv[1:]
    if args[:1] == ["--add"]:
        language = args[1]
        root = pathlib.Path(
            subprocess.run(["git", "rev-parse", "--show-toplevel"], cwd=HERE, check=True, capture_output=True, text=True).stdout.strip()
        )
        for arg in args[2:]:
            path = pathlib.Path(arg).resolve()
            fixture = CORPUS / language / ("__".join(path.parts[-2:]) + ".txt")
            fixture.parent.mkdir(parents=True, exist_ok=True)
            fixture.write_bytes(path.read_bytes())
            write(lib, index, fixture, language, path.relative_to(root).as_posix())
            print(f"added {fixture.name}")
        return
    for language in LANGUAGES:
        for fixture in sorted((CORPUS / language).glob("*.txt")):
            write(lib, index, fixture, language, source_of(fixture.with_name(fixture.name + ".clang")))


if __name__ == "__main__":
    main()
