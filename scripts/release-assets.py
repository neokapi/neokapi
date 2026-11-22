#!/usr/bin/env python3
"""Hold releases and registry updates to what scripts/lib/release-assets.json promises.

    release-assets.py check-release <track> <tag> [--complete]
        Fail when the GitHub release <tag> lacks an asset the track's 'ci'
        list names. With --complete, the 'signed_out_of_band' Windows assets
        must be there too (run after make release-windows / release-bowrain-windows).

    release-assets.py check-artifacts <track> <run-id>
        Fail when the workflow run lacks a Windows build artifact the track's
        'run_artifacts' list names, or when one has expired. Those artifacts
        are what the out-of-band signing step downloads.

    release-assets.py registry-args <plugin> <version> <checksums> <base-url>
        Print, one per line, the --platform/--url/--sha256/--signature
        arguments registry-update takes for every platform the plugin
        declares. <checksums> is a checksums.txt file or a directory of
        *.sha256 files ("<sha256>  <file>"). Fail when a declared platform
        has no checksum: a release never registers part of what it promises.

    release-assets.py check-checksums <plugin> <checksums>
        Like registry-args without a version or URL: fail when a platform the
        plugin declares has no archive checksum. The build jobs' final gate,
        run before anything is published, on tag pushes and dry runs alike.

    release-assets.py check-registry <registry.json> <plugin> <version> <platform>...
        Fail when the version would be registered without a platform the
        plugin's previous version in the registry carries, unless the
        'removed' section of release-assets.json names that platform.

    release-assets.py check-workflows
        Fail when a plugin's release workflow builds a platform the registry
        entry above does not declare, or the reverse.

    release-assets.py --self-test

A missing asset used to leave a release green: thirteen release candidates
shipped no Windows CLI, and from 1.2.0 on the registry carried no Windows
kapi-bowrain although every release built and uploaded one, so Windows
installs stayed on 1.1.0. These checks make each of those a failed run.

Exit status: 0 complete, 1 something is missing, 2 bad input.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import tempfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SPEC = os.path.join(ROOT, "scripts", "lib", "release-assets.json")


def load_spec(path: str = SPEC) -> dict:
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def die(msg: str, code: int = 2) -> None:
    print(f"release-assets: {msg}", file=sys.stderr)
    sys.exit(code)


def version_of(spec: dict, track: str, tag: str) -> str:
    prefix = spec["tracks"][track]["tag_prefix"]
    if not tag.startswith(prefix):
        die(f"tag {tag} does not start with the {track} prefix {prefix}")
    return tag[len(prefix):]


def missing_assets(spec: dict, track: str, version: str, present: set[str], complete: bool) -> list[str]:
    t = spec["tracks"][track]
    want = list(t["ci"]) + (list(t["signed_out_of_band"]) if complete else [])
    return [n.format(v=version) for n in want if n.format(v=version) not in present]


def gh_json(args: list[str]):
    out = subprocess.run(["gh", *args], check=True, capture_output=True, text=True).stdout
    return json.loads(out)


def cmd_check_release(spec: dict, track: str, tag: str, complete: bool) -> int:
    version = version_of(spec, track, tag)
    repo = os.environ.get("GITHUB_REPOSITORY", "neokapi/neokapi")
    data = gh_json(["release", "view", tag, "--repo", repo, "--json", "assets"])
    present = {a["name"] for a in data["assets"]}
    missing = missing_assets(spec, track, version, present, complete)
    for m in missing:
        print(f"release-assets: {tag} is missing {m}", file=sys.stderr)
    if missing:
        what = "including the signed Windows assets" if complete else "that this run publishes"
        print(f"release-assets: {tag} lacks {len(missing)} asset(s) {what}", file=sys.stderr)
        return 1
    print(f"release-assets: {tag} carries every {'' if complete else 'CI '}asset scripts/lib/release-assets.json lists")
    return 0


def missing_artifacts(spec: dict, track: str, artifacts: list[dict]) -> list[str]:
    live = {a["name"] for a in artifacts if not a.get("expired")}
    return [n for n in spec["tracks"][track]["run_artifacts"] if n not in live]


def cmd_check_artifacts(spec: dict, track: str, run_id: str) -> int:
    repo = os.environ.get("GITHUB_REPOSITORY", "neokapi/neokapi")
    data = gh_json(["api", f"repos/{repo}/actions/runs/{run_id}/artifacts?per_page=100"])
    missing = missing_artifacts(spec, track, data.get("artifacts", []))
    for m in missing:
        print(f"release-assets: run {run_id} has no live artifact {m}", file=sys.stderr)
    if missing:
        return 1
    print(f"release-assets: run {run_id} holds every Windows build artifact the signing step needs")
    return 0


def read_checksums(path: str) -> dict[str, str]:
    files = []
    if os.path.isdir(path):
        files = [os.path.join(path, f) for f in sorted(os.listdir(path)) if f.endswith(".sha256") or f == "checksums.txt"]
    elif os.path.isfile(path):
        files = [path]
    else:
        die(f"no checksums at {path}")
    sums: dict[str, str] = {}
    for f in files:
        with open(f, encoding="utf-8") as fh:
            for line in fh:
                parts = line.split()
                if len(parts) >= 2 and re.fullmatch(r"[0-9a-f]{64}", parts[0]):
                    sums[os.path.basename(parts[-1].lstrip("*"))] = parts[0]
    return sums


def registry_args(spec: dict, plugin: str, version: str, sums: dict[str, str], base_url: str) -> tuple[list[str], list[str]]:
    entry = spec["registry"].get(plugin)
    if entry is None:
        die(f"no registry entry for plugin {plugin} in {SPEC}")
    args, missing = [], []
    for plat, ext in entry["platforms"].items():
        os_, arch = plat.split("/")
        name = f"{entry['archive']}_{version}_{os_}_{arch}.{ext}"
        sha = sums.get(name)
        if not sha:
            missing.append(f"{plat} ({name})")
            continue
        url = f"{base_url.rstrip('/')}/{name}"
        args += ["--platform", plat, "--url", url, "--sha256", sha, "--signature", f"{url}.sigstore.json"]
    return args, missing


def cmd_registry_args(spec: dict, plugin: str, version: str, checksums: str, base_url: str) -> int:
    args, missing = registry_args(spec, plugin, version, read_checksums(checksums), base_url)
    if missing:
        for m in missing:
            print(f"release-assets: {plugin} {version} has no checksum for {m}", file=sys.stderr)
        print("release-assets: refusing to register a version without a platform it declares", file=sys.stderr)
        return 1
    print("\n".join(args))
    return 0


def cmd_check_checksums(spec: dict, plugin: str, checksums: str) -> int:
    """Every declared platform has a checksum, whatever version the files carry.

    A build-only dispatch names its archives after the branch rather than a
    tag, so the version is read from the files instead of the ref.
    """
    sums = read_checksums(checksums)
    archive = spec["registry"][plugin]["archive"]
    versions = set()
    for name in sums:
        m = re.fullmatch(re.escape(archive) + r"_(.+)_[a-z]+_[a-z0-9]+\.(?:tar\.gz|zip)", name)
        if m:
            versions.add(m.group(1))
    if len(versions) != 1:
        print(f"release-assets: {checksums} holds {plugin} archives for {sorted(versions) or 'no version'}; want one",
              file=sys.stderr)
        return 1
    version = versions.pop()
    _, missing = registry_args(spec, plugin, version, sums, "https://example.invalid")
    for m in missing:
        print(f"release-assets: {plugin} {version} has no checksum for {m}", file=sys.stderr)
    if missing:
        return 1
    print(f"release-assets: {plugin} {version} built every platform it registers: "
          f"{', '.join(spec['registry'][plugin]['platforms'])}")
    return 0


def previous_platforms(registry: dict, plugin: str, version: str) -> tuple[str, set[str]] | None:
    versions = (registry.get("plugins", {}).get(plugin) or {}).get("versions") or {}
    others = [(v, info) for v, info in versions.items() if v != version]
    if not others:
        return None
    # The last one registered: the registry keeps insertion order, and the
    # released date breaks nothing when two land on one day.
    order = {v: i for i, v in enumerate(versions)}
    v, info = max(others, key=lambda x: (x[1].get("released", ""), order[x[0]]))
    return v, set((info.get("platforms") or {}).keys())


def lost_platforms(spec: dict, registry: dict, plugin: str, version: str, platforms: list[str]) -> tuple[str | None, list[str]]:
    prev = previous_platforms(registry, plugin, version)
    if prev is None:
        return None, []
    prev_version, prev_plats = prev
    removed = set((spec.get("removed") or {}).get(plugin, {}))
    return prev_version, sorted(prev_plats - set(platforms) - removed)


def cmd_check_registry(spec: dict, registry_path: str, plugin: str, version: str, platforms: list[str]) -> int:
    with open(registry_path, encoding="utf-8") as fh:
        registry = json.load(fh)
    prev, lost = lost_platforms(spec, registry, plugin, version, platforms)
    if lost:
        print(f"release-assets: {plugin} {version} would drop {', '.join(lost)}, which {prev} carries; "
              "build them, or list them under 'removed' in scripts/lib/release-assets.json", file=sys.stderr)
        return 1
    print(f"release-assets: {plugin} {version} keeps every platform {prev or 'no earlier version'} had")
    return 0


def workflow_platforms(path: str) -> set[str]:
    """The os/arch pairs a release workflow's build matrices produce."""
    with open(path, encoding="utf-8") as fh:
        text = fh.read()
    plats = set(re.findall(r"platform:\s*([a-z]+/[a-z0-9]+)", text))
    for goos, goarch in re.findall(r"goos:\s*([a-z]+),\s*goarch:\s*([a-z0-9]+)", text):
        plats.add(f"{goos}/{goarch}")
    return plats


def cmd_check_workflows(spec: dict) -> int:
    """Each plugin's registered platforms are the platforms its workflow builds."""
    problems = []
    sources = {name: f"release-{name}.yml" for name in spec["registry"] if name not in ("kapi", "bowrain")}
    sources["bowrain"] = "release-bowrain.yml"
    for name, wf in sorted(sources.items()):
        path = os.path.join(ROOT, ".github", "workflows", wf)
        if not os.path.exists(path):
            problems.append(f"{name}: {wf} does not exist")
            continue
        built = {p for p in workflow_platforms(path) if p.split("/")[0] in ("darwin", "linux", "windows")}
        declared = set(spec["registry"][name]["platforms"])
        removed = set((spec.get("removed") or {}).get(name, {}))
        for p in sorted(built - declared - removed):
            problems.append(f"{name}: {wf} builds {p}, which release-assets.json does not register")
        for p in sorted(declared - built):
            problems.append(f"{name}: release-assets.json registers {p}, which {wf} does not build")
    for p in problems:
        print(f"release-assets: {p}", file=sys.stderr)
    if problems:
        return 1
    print("release-assets: every plugin registers exactly the platforms its release workflow builds")
    return 0


def self_test() -> int:
    spec = load_spec()
    failures = []

    def expect(cond: bool, what: str) -> None:
        if not cond:
            failures.append(what)

    v = "9.9.9"
    full = {n.format(v=v) for t in ["ci", "signed_out_of_band"] for n in spec["tracks"]["kapi"][t]}
    expect(missing_assets(spec, "kapi", v, full, True) == [], "a complete kapi release is reported")
    ci_only = {n.format(v=v) for n in spec["tracks"]["kapi"]["ci"]}
    expect(missing_assets(spec, "kapi", v, ci_only, False) == [], "a CI-complete release is reported")
    expect(len(missing_assets(spec, "kapi", v, ci_only, True)) == 6, "missing signed Windows assets pass --complete")
    gap = set(ci_only) - {f"kapi-cli_{v}_linux_arm64.tar.gz"}
    expect(missing_assets(spec, "kapi", v, gap, False) == [f"kapi-cli_{v}_linux_arm64.tar.gz"], "a missing CI asset passes")
    bow = {n.format(v=v) for n in spec["tracks"]["bowrain"]["ci"]} - {f"kapi-bowrain_{v}_windows_arm64.zip"}
    expect(missing_assets(spec, "bowrain", v, bow, False) == [f"kapi-bowrain_{v}_windows_arm64.zip"], "a missing Windows plugin zip passes")

    arts = [{"name": n, "expired": False} for n in spec["tracks"]["kapi"]["run_artifacts"]]
    expect(missing_artifacts(spec, "kapi", arts) == [], "live artifacts are reported")
    arts[0]["expired"] = True
    expect(missing_artifacts(spec, "kapi", arts) == ["kapi-cli-windows-amd64"], "an expired artifact passes")

    with tempfile.TemporaryDirectory() as tmp:
        sums = os.path.join(tmp, "checksums.txt")
        with open(sums, "w") as fh:
            for plat, ext in spec["registry"]["bowrain"]["platforms"].items():
                o, a = plat.split("/")
                fh.write(f"{'a' * 64}  kapi-bowrain_{v}_{o}_{a}.{ext}\n")
        args, missing = registry_args(spec, "bowrain", v, read_checksums(sums), "https://x/r")
        expect(missing == [] and args.count("--platform") == 5, "every bowrain platform is registered")
        expect(f"https://x/r/kapi-bowrain_{v}_windows_arm64.zip" in args, "the Windows plugin zip is not registered")
        partial = {k: s for k, s in read_checksums(sums).items() if "windows_amd64" not in k}
        _, missing = registry_args(spec, "bowrain", v, partial, "https://x/r")
        expect(missing == [f"windows/amd64 (kapi-bowrain_{v}_windows_amd64.zip)"], "a missing platform checksum passes")
        os.mkdir(os.path.join(tmp, "sat"))
        for plat in spec["registry"]["sat"]["platforms"]:
            o, a = plat.split("/")
            with open(os.path.join(tmp, "sat", f"{o}_{a}.sha256"), "w") as fh:
                fh.write(f"{'c' * 64}  kapi-sat_my-branch_{o}_{a}.tar.gz\n")
        expect(cmd_check_checksums(spec, "sat", os.path.join(tmp, "sat")) == 0, "a complete dry-run build is reported")
        os.remove(os.path.join(tmp, "sat", "windows_arm64.sha256"))
        expect(cmd_check_checksums(spec, "sat", os.path.join(tmp, "sat")) == 1, "a dry-run build missing a platform passes")
        os.mkdir(os.path.join(tmp, "dir"))
        with open(os.path.join(tmp, "dir", "darwin_arm64.sha256"), "w") as fh:
            fh.write(f"{'b' * 64}  kapi-sat_{v}_darwin_arm64.tar.gz\n")
        expect(read_checksums(os.path.join(tmp, "dir")) == {f"kapi-sat_{v}_darwin_arm64.tar.gz": "b" * 64},
               "a directory of .sha256 files is not read")

    reg = {"plugins": {"p": {"versions": {
        "1.0.0": {"released": "2026-01-01", "platforms": {"darwin/arm64": {}, "windows/amd64": {}}},
        "1.1.0": {"released": "2026-02-01", "platforms": {"darwin/arm64": {}, "windows/amd64": {}, "windows/arm64": {}}},
    }}}}
    prev, lost = lost_platforms(spec, reg, "p", "1.2.0", ["darwin/arm64", "windows/amd64"])
    expect(prev == "1.1.0" and lost == ["windows/arm64"], "dropping a platform the previous version had passes")
    _, lost = lost_platforms(spec, reg, "p", "1.2.0", ["darwin/arm64", "windows/amd64", "windows/arm64", "linux/amd64"])
    expect(lost == [], "adding a platform is reported")
    spec2 = dict(spec, removed={"p": {"windows/arm64": "no arm64 runner"}})
    _, lost = lost_platforms(spec2, reg, "p", "1.2.0", ["darwin/arm64", "windows/amd64"])
    expect(lost == [], "a declared removal is reported")
    prev, lost = lost_platforms(spec, reg, "new", "0.1.0", ["darwin/arm64"])
    expect(prev is None and lost == [], "a first version is reported")

    for name in spec["registry"]:
        expect(all(p.count("/") == 1 for p in spec["registry"][name]["platforms"]), f"{name}: a platform is not os/arch")

    for f in failures:
        print(f"release-assets self-test: {f}", file=sys.stderr)
    if failures:
        return 1
    print("release-assets: self-test passed")
    return 0


def main(argv: list[str]) -> int:
    if argv == ["--self-test"]:
        return self_test()
    if argv == ["check-workflows"]:
        return cmd_check_workflows(load_spec())
    spec = load_spec()
    if len(argv) >= 3 and argv[0] == "check-release":
        return cmd_check_release(spec, argv[1], argv[2], "--complete" in argv[3:])
    if len(argv) == 3 and argv[0] == "check-artifacts":
        return cmd_check_artifacts(spec, argv[1], argv[2])
    if len(argv) == 5 and argv[0] == "registry-args":
        return cmd_registry_args(spec, *argv[1:])
    if len(argv) == 3 and argv[0] == "check-checksums":
        return cmd_check_checksums(spec, argv[1], argv[2])
    if len(argv) >= 4 and argv[0] == "check-registry":
        return cmd_check_registry(spec, argv[1], argv[2], argv[3], argv[4:])
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
