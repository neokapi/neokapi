"""Run one shell command in a pseudo-terminal and answer its prompts.

Usage: pty-run.py <command> [line ...]

The command runs under /bin/sh -c with a pty as its stdin, stdout and stderr,
so a program that asks only at a terminal asks. Each further argument is one
line typed once the output has been quiet for PTY_QUIET_SEC seconds, the way a
person answers once the question is on screen. The terminal echoes each line,
so the recording shows what was typed.

Terminal queries are answered as a terminal answers them: the background
colour (OSC 11) and the cursor position (CSI 6n). Output goes to stdout as the
terminal showed it, without escape sequences or carriage returns. The exit
code is the command's; a command still running after PTY_TIMEOUT_SEC is
killed.
"""

import os
import pty
import re
import select
import sys
import time

QUIET = float(os.environ.get("PTY_QUIET_SEC", "0.8"))
TIMEOUT = float(os.environ.get("PTY_TIMEOUT_SEC", "120"))
DEBUG = bool(os.environ.get("PTY_DEBUG"))

QUERIES = [
    (b"\x1b]11;?\x1b\\", b"\x1b]11;rgb:0000/0000/0000\x1b\\"),
    (b"\x1b]11;?\x07", b"\x1b]11;rgb:0000/0000/0000\x07"),
    (b"\x1b[6n", b"\x1b[1;1R"),
]
ESCAPES = re.compile(r"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]")


def main() -> int:
    command, lines = sys.argv[1], list(sys.argv[2:])
    pid, fd = pty.fork()
    if pid == 0:
        os.execv("/bin/sh", ["/bin/sh", "-c", command])
    out = bytearray()
    last = time.monotonic()
    deadline = last + TIMEOUT
    while time.monotonic() < deadline:
        ready, _, _ = select.select([fd], [], [], 0.1)
        if ready:
            try:
                chunk = os.read(fd, 4096)
            except OSError:
                break
            if not chunk:
                break
            if DEBUG:
                sys.stderr.write(repr(chunk) + "\n")
            for query, answer in QUERIES:
                for _ in range(chunk.count(query)):
                    os.write(fd, answer)
            out += chunk
            last = time.monotonic()
        elif lines and time.monotonic() - last >= QUIET:
            line = lines.pop(0)
            if DEBUG:
                sys.stderr.write("<< " + line + "\n")
            os.write(fd, (line + "\n").encode())
            last = time.monotonic()
    else:
        os.kill(pid, 9)
    _, status = os.waitpid(pid, 0)
    text = ESCAPES.sub("", out.decode("utf-8", "replace"))
    sys.stdout.write(text.replace("\r\n", "\n").replace("\r", ""))
    sys.stdout.flush()
    return os.waitstatus_to_exitcode(status)


if __name__ == "__main__":
    sys.exit(main())
