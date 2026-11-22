#!/usr/bin/env python3
"""Report the pinned tools that have fallen behind their latest release.

Dependabot keeps module, package, action and image versions current. It cannot
see a version written into a workflow `env:` value, a shell script or an action
input: the Go toolchain CI installs, the linter, a native library fetched at
build time. Those went stale without anyone noticing, some for seven minor
releases. This script holds one row per such pin: where it is written, and whom
to ask for the latest release.

Two checks, for every row:

  - every place the pin is written agrees with the others (no network needed);
  - the pin is the latest stable release (asks the registry named in the row).

  scripts/check-tool-versions.py             both checks; exit 1 on a finding
  scripts/check-tool-versions.py --offline   the agreement check only
  scripts/check-tool-versions.py --self-test

The weekly Tool versions workflow runs both, and a finding there opens an issue
through scheduled-failure-issue.yml. `make lint` runs the offline check, so a
pull request that raises one copy of a pin and not the others fails at once.

A row whose `note` names a coupling is not a one-line bump: read the note.
"""

import argparse
import glob
import json
import os
import re
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

# One row per pin. `where` is a list of (glob, regex); the regex has one group,
# the version, and every match in every file must agree. `latest` is (kind,
# argument) for the registry to ask.
PINS = [
    {
        "name": "Go toolchain",
        "where": [
            (".github/workflows/*.yml", r'go-version:\s*\[?"(\d+\.\d+\.\d+)"'),
            (".github/actions/setup-go-modules/action.yml", r'description: Go version to install\n\s+required: false\n\s+default: "(\d+\.\d+\.\d+)"'),
        ],
        "latest": ("go", ""),
        "note": "Patch releases carry security fixes. The go line in go.mod is a minimum and stays.",
    },
    {
        "name": "golangci-lint",
        "where": [(".github/actions/golangci/action.yml", r"version: v(\d+\.\d+\.\d+)")],
        "latest": ("github-release", "golangci/golangci-lint"),
        "note": "A new release brings new linter versions: run make check and fix what it reports in the same change.",
    },
    {
        "name": "Vite+",
        "where": [
            ("pnpm-workspace.yaml", r"^  vite-plus: (\d+\.\d+\.\d+)$"),
            (".github/actions/setup-node-ui/action.yml", r'version: "(\d+\.\d+\.\d+)"'),
            (".github/workflows/dependabot-pnpm-relock.yml", r'version: "(\d+\.\d+\.\d+)"'),
            (".github/workflows/publish-npm.yml", r'version: "(\d+\.\d+\.\d+)"'),
        ],
        "latest": ("npm", "vite-plus"),
        "note": "Moves with the `vite` alias in pnpm-workspace.yaml (scripts/audit-vite-alias.sh). From 1.7.0 the core package has its own version, so that script changes first.",
    },
    {
        "name": "pnpm",
        "where": [("package.json", r'"packageManager": "pnpm@(\d+\.\d+\.\d+)"')],
        "latest": ("npm", "pnpm"),
        "note": "Re-resolve the lockfile and run scripts/check-lockfile-idempotent.sh.",
    },
    {
        "name": "Node.js in CI",
        "where": [
            (".github/workflows/*.yml", r'node-version: "(\d+)"'),
            (".github/actions/setup-node-ui/action.yml", r'description: Node.js version to install\n\s+required: false\n\s+default: "(\d+)"'),
        ],
        "latest": ("node-lts", ""),
        "note": "The newest LTS major. The Docker builds pin Node separately; Dependabot tracks those.",
    },
    {
        "name": "onnxruntime",
        "where": [
            ("scripts/fetch-onnxruntime.sh", r'readonly DEFAULT_VERSION="(\d+\.\d+\.\d+)"'),
            (".github/workflows/*.yml", r'ONNXRUNTIME_VERSION: "(\d+\.\d+\.\d+)"'),
        ],
        "latest": ("nuget", "microsoft.ml.onnxruntime"),
        "note": "Coupled: the version must be the one the onnxruntime_go release below was built for (its ORT_API_VERSION), and scripts/fetch-onnxruntime.sh needs the new SHA-256. Run the nightly vision jobs and a release dry run on the branch.",
    },
    {
        "name": "onnxruntime_go binding",
        "where": [("plugins/*/go.mod", r"github\.com/yalue/onnxruntime_go v(\d+\.\d+\.\d+)")],
        "latest": ("go-module", "github.com/yalue/onnxruntime_go"),
        "note": "Coupled with onnxruntime above; raise the two together.",
    },
    {
        "name": "tokenizers",
        "where": [
            (".github/workflows/*.yml", r'TOKENIZERS_VERSION: "v(\d+\.\d+\.\d+)"'),
            ("plugins/*/go.mod", r"github\.com/daulet/tokenizers v(\d+\.\d+\.\d+)"),
        ],
        "latest": ("github-release", "daulet/tokenizers"),
        "note": "The Go module and the static library the release workflows download move together. Dry-run the sat release.",
    },
    {
        "name": "libpdfium",
        "where": [
            ("scripts/fetch-pdfium.sh", r'readonly DEFAULT_RELEASE="chromium/(\d+)"'),
            (".github/workflows/*.yml", r'PDFIUM_RELEASE: "?chromium/(\d+)'),
        ],
        "latest": ("github-release", "bblanchon/pdfium-binaries"),
        "note": "Coupled: the release must be the one the go-pdfium version in plugins/pdfium/go.mod builds against, and scripts/fetch-pdfium.sh needs the SHA-256 of every platform archive. Dry-run release-pdfium.yml and the nightly PDF job.",
    },
    {
        "name": "ICU (static build)",
        "where": [("scripts/build-icu-static.sh", r'ICU_VERSION="\$\{ICU_VERSION:-(\d+\.\d+)\}"')],
        "latest": ("github-release", "unicode-org/icu"),
        "note": "A new ICU changes segmentation and collation data: run the segmentation parity tests. scripts/build-icu-static.sh needs the SHA-256 of the sources tarball and the data zip, and its smoke check must pass.",
    },
    {
        "name": "whisper.cpp",
        "where": [(".github/workflows/release-asr.yml", r'WHISPER_CPP_VERSION: "v(\d+\.\d+\.\d+)"')],
        "latest": ("github-tag", "ggml-org/whisper.cpp"),
        "note": "Upstream publishes prebuilt binaries under separate build tags (b<number>), not under the version tag. The workflow needs both pins before this can move.",
    },
    {
        "name": "actionlint",
        "where": [(".github/workflows/ci.yml", r"actionlint/cmd/actionlint@v(\d+\.\d+\.\d+)")],
        "latest": ("go-module", "github.com/rhysd/actionlint"),
        "note": "",
    },
    {
        "name": "nfpm",
        "where": [(".github/workflows/release.yml", r"nfpm/v2/cmd/nfpm@v(\d+\.\d+\.\d+)")],
        "latest": ("go-module", "github.com/goreleaser/nfpm/v2"),
        "note": "Build the deb and rpm with both versions and compare the file list and control fields.",
    },
    {
        "name": "govulncheck",
        "where": [("scripts/govulncheck.py", r"cmd/govulncheck@v(\d+\.\d+\.\d+)")],
        "latest": ("go-module", "golang.org/x/vuln"),
        "note": "scripts/govulncheck.py reads the scanner's JSON: check PROTOCOL and RECORDS there against the new release.",
    },
    {
        "name": "cosign",
        "where": [(".github/workflows/*.yml", r"cosign/v2/cmd/cosign@v(\d+\.\d+\.\d+)")],
        "latest": ("go-module", "github.com/sigstore/cosign/v2"),
        "note": "",
    },
    {
        "name": "setup-vp action",
        "where": [(".github/**/*.yml", r"voidzero-dev/setup-vp@v(\d+\.\d+\.\d+)")],
        "latest": ("github-release", "voidzero-dev/setup-vp"),
        "note": "Dependabot raises this one; the row keeps its copies equal.",
    },
    {
        "name": "Versity S3 gateway image",
        "where": [
            ("bowrain/compose*.yaml", r"versity/versitygw:v?([\w.]+)"),
            ("bowrain/deploy/docker/compose.yaml", r"versity/versitygw:v?([\w.]+)"),
            ("bowrain/testutil/testimage/testimage.go", r"versity/versitygw:v?([\w.]+)"),
            (".github/workflows/mirror-images.yml", r"default: v(\d+\.\d+\.\d+)"),
        ],
        "latest": ("github-release", "versity/versitygw"),
        "note": "The local stack and the blob store tests run the same server: raise testimage.go with the compose file, then run the s3blob tests.",
    },
    {
        "name": "AWS CLI image",
        "where": [
            ("bowrain/compose*.yaml", r"amazon/aws-cli:([\w.]+)"),
            ("bowrain/deploy/docker/compose.yaml", r"amazon/aws-cli:([\w.]+)"),
        ],
        "latest": ("github-tag", "aws/aws-cli"),
        "note": "Dependabot raises the compose files; the row keeps every copy equal and none on latest.",
    },
    {
        "name": "Traefik image",
        "where": [
            ("bowrain/compose*.yaml", r"image: traefik:v?([\w.]+)"),
            ("bowrain/deploy/docker/compose.yaml", r"image: traefik:v?([\w.]+)"),
        ],
        "latest": ("github-release", "traefik/traefik"),
        "note": "Pinned to a minor release, so patch releases arrive on the next pull.",
    },
    {
        "name": "ElasticMQ image",
        "where": [
            ("bowrain/compose*.yaml", r"softwaremill/elasticmq-native:([\w.]+)"),
            ("bowrain/deploy/docker/compose.yaml", r"softwaremill/elasticmq-native:([\w.]+)"),
            ("bowrain/testutil/testimage/testimage.go", r"softwaremill/elasticmq-native:([\w.]+)"),
        ],
        "latest": ("github-tag", "softwaremill/elasticmq"),
        "note": "The local stack and the job queue tests run the same server: raise testimage.go with the compose files.",
    },
]


def numbers(version):
    """The integers of a version, for comparison: '1.27.4' -> (1, 27, 4)."""
    return tuple(int(part) for part in re.findall(r"\d+", version))


def stable(tag):
    """The version in a release tag, or None for a pre-release or a build tag."""
    match = re.fullmatch(r"(?:v|release-|chromium/|go)?(\d+(?:[.-]\d+)*)", tag)
    return match.group(1).replace("-", ".") if match else None


def read_pin(row, root=ROOT):
    """Return {version: [places]} for every place the row's pin is written."""
    found = {}
    for pattern, regex in row["where"]:
        for name in sorted(glob.glob(str(root / pattern), recursive=True)):
            text = Path(name).read_text(encoding="utf-8")
            for match in re.finditer(regex, text, re.M):
                line = text.count("\n", 0, match.start(1)) + 1
                found.setdefault(match.group(1), []).append(f"{Path(name).relative_to(root)}:{line}")
    return found


def fetch(url):
    request = urllib.request.Request(url, headers={"User-Agent": "tool-version-check", "Accept": "application/json"})
    token = os.environ.get("GITHUB_TOKEN")
    if token and url.startswith("https://api.github.com/"):
        request.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def newest(versions):
    """The highest stable version among release tags."""
    candidates = [version for version in map(stable, versions) if version]
    return max(candidates, key=numbers) if candidates else None


def latest(kind, argument):
    """Ask the registry for the latest stable release."""
    if kind == "go":
        return newest(release["version"] for release in fetch("https://go.dev/dl/?mode=json") if release.get("stable"))
    if kind == "github-release":
        return stable(fetch(f"https://api.github.com/repos/{argument}/releases/latest")["tag_name"])
    if kind == "github-tag":
        return newest(tag["name"] for tag in fetch(f"https://api.github.com/repos/{argument}/tags?per_page=100"))
    if kind == "npm":
        return fetch(f"https://registry.npmjs.org/{argument}/latest")["version"]
    if kind == "go-module":
        return stable(fetch(f"https://proxy.golang.org/{argument}/@latest")["Version"])
    if kind == "nuget":
        return newest(fetch(f"https://api.nuget.org/v3-flatcontainer/{argument}/index.json")["versions"])
    if kind == "node-lts":
        return newest(release["version"].split(".")[0] for release in fetch("https://nodejs.org/dist/index.json") if release.get("lts"))
    raise ValueError(f"unknown registry kind {kind}")


def behind(pinned, upstream):
    """Whether the pin is older than upstream, compared to the pin's precision."""
    have, want = numbers(pinned), numbers(upstream)
    return have < want[: len(have)]


def check(rows, offline, ask=latest, root=ROOT):
    """Return (report lines, finding lines) for the rows."""
    report, findings = [], []
    for row in rows:
        found = read_pin(row, root)
        if not found:
            findings.append(f"{row['name']}: no pin found. The pattern in scripts/check-tool-versions.py no longer matches the tree.")
            continue
        if len(found) > 1:
            places = "; ".join(f"{version} at {', '.join(where[:3])}" for version, where in sorted(found.items()))
            findings.append(f"{row['name']}: the copies of this pin disagree: {places}")
            continue
        pinned = next(iter(found))
        if offline:
            report.append(f"{row['name']}: {pinned} ({len(found[pinned])} place(s))")
            continue
        try:
            upstream = ask(*row["latest"])
        except Exception as error:  # noqa: BLE001 - one registry failing must not hide the other rows
            findings.append(f"{row['name']}: could not ask for the latest release: {error}")
            continue
        if not upstream:
            findings.append(f"{row['name']}: the registry returned no stable release")
        elif behind(pinned, upstream):
            note = f"\n    {row['note']}" if row["note"] else ""
            findings.append(f"{row['name']}: pinned {pinned}, latest {upstream} ({found[pinned][0]} and {len(found[pinned]) - 1} more){note}")
        else:
            report.append(f"{row['name']}: {pinned} is current (latest {upstream})")
    return report, findings


def self_test():
    import tempfile

    assert numbers("1.27.4") == (1, 27, 4)
    for tag, want in [("v2.14.0", "2.14.0"), ("release-79-1", "79.1"), ("chromium/8165", "8165"), ("go1.27.4", "1.27.4"), ("b5907", None), ("v3.2.0-rc.1", None), ("1.31.0-dev.20260911", None)]:
        assert stable(tag) == want, (tag, stable(tag))
    assert newest(["v1.9.1", "b5907", "v1.9.5", "v1.10.0-rc1"]) == "1.9.5"
    assert behind("1.27.1", "1.27.4") and not behind("1.27.4", "1.27.4") and not behind("1.28.0", "1.27.9")
    assert behind("22", "24.11.1") and not behind("24", "24.11.1") and behind("78.3", "79.1")

    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        (root / "a").mkdir()
        (root / "a/one.yml").write_text('x: 1\nTOOL_VERSION: "1.2.3"\n', encoding="utf-8")
        (root / "a/two.yml").write_text('TOOL_VERSION: "1.2.3"\n', encoding="utf-8")
        row = {"name": "tool", "where": [("a/*.yml", r'TOOL_VERSION: "(\d+\.\d+\.\d+)"')], "latest": ("npm", "tool"), "note": "read me"}
        assert read_pin(row, root) == {"1.2.3": ["a/one.yml:2", "a/two.yml:1"]}

        report, findings = check([row], offline=False, ask=lambda kind, argument: "1.2.3", root=root)
        assert not findings and "is current" in report[0], (report, findings)
        report, findings = check([row], offline=False, ask=lambda kind, argument: "1.3.0", root=root)
        assert len(findings) == 1 and "pinned 1.2.3, latest 1.3.0" in findings[0] and "read me" in findings[0], findings

        def unreachable(kind, argument):
            raise OSError("no network")

        report, findings = check([row], offline=False, ask=unreachable, root=root)
        assert len(findings) == 1 and "could not ask" in findings[0], findings
        report, findings = check([row], offline=True, ask=unreachable, root=root)
        assert not findings, findings

        (root / "a/two.yml").write_text('TOOL_VERSION: "1.2.4"\n', encoding="utf-8")
        report, findings = check([row], offline=True, root=root)
        assert len(findings) == 1 and "disagree" in findings[0], findings

        missing = dict(row, where=[("a/*.yml", r"NOT_THERE: (\d+)")])
        report, findings = check([missing], offline=True, root=root)
        assert len(findings) == 1 and "no pin found" in findings[0], findings

    # Every real row still finds its pin in this tree, with one value.
    report, findings = check(PINS, offline=True)
    assert not findings, findings
    print("check-tool-versions: self-test passed")


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--offline", action="store_true", help="check only that the copies of each pin agree")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0
    report, findings = check(PINS, args.offline)
    for line in report:
        print(line)
    if findings:
        print("\n" + "\n".join(findings), file=sys.stderr)
        print(f"\ncheck-tool-versions: {len(findings)} pin(s) need attention", file=sys.stderr)
        return 1
    print("check-tool-versions: every pin agrees" + ("" if args.offline else " and is current"))
    return 0


if __name__ == "__main__":
    sys.exit(main())
