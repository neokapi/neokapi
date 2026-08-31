"""Recipe edits the regeneration makes to its scratch copy.

The sample's own recipe is never modified by this: the copy under the scratch
tree is pinned to the recycle flow so the whole run stays offline, and the
sample ships with its ordinary defaults.
"""

import sys

RECYCLE_FLOW = """flows:
  recycle:
    steps:
      - tool: recycle
"""


def pin_recycle(path: str) -> None:
    """Point the project's default flow at recycle.

    Convergence then reuses approved wording and calls no provider, so the run
    needs no credential and produces the same targets on every machine.
    """
    text = open(path, encoding="utf-8").read()

    if "\n  recycle:\n" not in text:
        text = text.replace("flows:\n", RECYCLE_FLOW, 1)

    marker = "  memory_source:"
    line_end = text.index("\n", text.index(marker))
    text = text[:line_end] + "\n  flow: recycle" + text[line_end:]

    open(path, "w", encoding="utf-8").write(text)


if __name__ == "__main__":
    if len(sys.argv) != 3 or sys.argv[1] != "pin-recycle":
        print("usage: recipe.py pin-recycle <kapi.yaml>", file=sys.stderr)
        raise SystemExit(2)
    pin_recycle(sys.argv[2])
