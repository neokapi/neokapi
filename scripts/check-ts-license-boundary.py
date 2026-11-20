"""Check workspace imports against the license nearest each file."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from tempfile import TemporaryDirectory


SOURCE_SUFFIXES = {".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}
SKIP = re.compile(
    r"(?:^|/)(?:node_modules|dist|build|coverage|storybook-static|"
    r"\.next|\.turbo|\.vite|bindings/github\.com)/"
)
DEPENDENCIES = ("dependencies", "devDependencies", "peerDependencies", "optionalDependencies")

SINGLE_QUOTED = r"'(?:[^'\\\n]|\\.)*'"
DOUBLE_QUOTED = r'"(?:[^"\\\n]|\\.)*"'
QUOTED = rf"{SINGLE_QUOTED}|{DOUBLE_QUOTED}"
COMMENT = r"//[^\n]*|/\*[\s\S]*?\*/"
SPACE = rf"(?:\s|{COMMENT})*"
IMPORT = re.compile(
    rf"\b(?:from|import|require){SPACE}(?:\({SPACE})?(?P<spec>{QUOTED})"
    rf"|{COMMENT}|{QUOTED}"
)
JSON_STRING = r'"(?:[^"\\]|\\.)*"'
JSON_TOKEN = re.compile(rf"{JSON_STRING}|{COMMENT}")
TRAILING_COMMA = re.compile(rf"{JSON_STRING}|,(?=\s*[}}\]])")


def read_config(path):
    text = path.read_text(encoding="utf-8-sig")
    if path.name != "package.json":
        # Preserve offsets so a parse error points to the original config.
        text = JSON_TOKEN.sub(
            lambda m: m[0] if m[0].startswith('"') else re.sub(r"[^\n]", " ", m[0]),
            text,
        )
        text = TRAILING_COMMA.sub(lambda m: " " if m[0] == "," else m[0], text)
    try:
        data = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ValueError(f"{path}:{exc.lineno}:{exc.colno}: {exc.msg}") from exc
    if not isinstance(data, dict):
        raise ValueError(f"{path}: expected a JSON object")
    return data


def git_files(root, *flags):
    output = subprocess.check_output(["git", "ls-files", "-z", *flags], cwd=root)
    paths = os.fsdecode(output).split("\0")
    return {path for path in paths if path and not SKIP.search(path)}


def tsconfig_targets(config, directory):
    options = config.get("compilerOptions", {})
    base = directory / options.get("baseUrl", ".")
    for values in options.get("paths", {}).values():
        for target in values:
            yield base, target
    for key in ("baseUrl", "rootDir"):
        if key in options:
            yield directory, options[key]
    for key in ("rootDirs", "typeRoots"):
        for target in options.get(key, []):
            yield directory, target
    for name in options.get("types", []):
        yield None, name
    if "jsxImportSource" in options:
        yield None, options["jsxImportSource"]
    for key in ("files", "include"):
        for target in config.get(key, []):
            yield directory, target
    for ref in config.get("references", []):
        yield directory, ref["path"]
    extends = config.get("extends", [])
    for target in [extends] if isinstance(extends, str) else extends:
        yield directory if target.startswith(".") or os.path.isabs(target) else None, target


def package_name(spec):
    return "/".join(spec.split("/")[:2]) if spec.startswith("@") else spec.split("/")[0]


def dependency_targets(name, version, directory):
    yield None, name
    protocol, _, target = version.partition(":")
    if protocol in ("file", "link") or (protocol == "workspace" and target.startswith(".")):
        yield directory, target
    elif protocol in ("npm", "workspace"):
        version_at = target.find("@", 1)
        yield None, target if version_at == -1 else target[:version_at]


def violations(root):
    names = git_files(root, "--cached", "--others", "--exclude-standard") - git_files(root, "--deleted")
    files = sorted(root / name for name in names)
    licenses = {
        path.parent: "GNU AFFERO GENERAL PUBLIC LICENSE" in path.read_text(encoding="utf-8").upper()
        for path in files if path.name == "LICENSE"
    }
    if root not in licenses:
        raise ValueError(f"{root}: missing root LICENSE")

    def is_agpl(directory):
        return next((licenses[d] for d in (directory, *directory.parents) if d in licenses), False)

    manifests = {p: read_config(p) for p in files if p.name == "package.json"}
    agpl_names = {m["name"] for p, m in manifests.items() if "name" in m and is_agpl(p.parent)}

    def crossing(base, spec):
        if base is None:
            name = package_name(spec)
            return name if name in agpl_names else None
        target = (base / spec).resolve()
        if is_agpl(target):
            return os.path.relpath(target, root)
        return None

    for path in files:
        if is_agpl(path.parent):
            continue
        rel = path.relative_to(root)
        if path in manifests:
            for field in DEPENDENCIES:
                for name, version in manifests[path].get(field, {}).items():
                    for base, spec in dependency_targets(name, version, path.parent):
                        target = crossing(base, spec)
                        if target:
                            yield f"{rel}: {field}.{name} depends on {target} under the AGPL license"
                            break
        elif path.match("tsconfig*.json"):
            for base, spec in tsconfig_targets(read_config(path), path.parent):
                target = crossing(base, spec)
                if target:
                    yield f"{rel}: tsconfig value {spec} reaches {target} under the AGPL license"
        elif path.suffix in SOURCE_SUFFIXES:
            text = path.read_text(encoding="utf-8-sig")
            for match in IMPORT.finditer(text):
                if not match["spec"]:
                    continue
                spec = match["spec"][1:-1]
                base = path.parent if spec.startswith(".") else None
                target = crossing(base, spec)
                if target:
                    line = text.count("\n", 0, match.start("spec")) + 1
                    yield f"{rel}:{line}: import {spec} reaches {target} under the AGPL license"


def self_test():
    with TemporaryDirectory() as tmp:
        root = Path(tmp).resolve()
        subprocess.run(["git", "init", "--quiet", root], check=True)
        fixture = {
            "LICENSE": "Apache License",
            "server/LICENSE": "GNU AFFERO GENERAL PUBLIC LICENSE",
            "server/package.json": '{"private":true,"name":"@fixture/server"}',
            "server/plugin/LICENSE": "Apache License",
            "server/plugin/package.json": '{"name":"@fixture/plugin"}',
            "server/api.ts": 'import "@fixture/server";',
            "app/path.mts": 'export * from\n"../server/api";',
            "app/module.cts": 'const x = require(\n"@fixture/server/subpath");',
            "app/clean.tsx": """// import "@fixture/server";
import "@fixture/plugin";
import "../server/plugin/api";
export const title = 'Read from "@fixture/server"';
""",
            "app/package.json": json.dumps({
                "dependencies": {"@fixture/server": "*"},
                "peerDependencies": {"alias": "npm:@fixture/server@*"},
                "optionalDependencies": {"local": "link:../server"},
            }),
            "app/tsconfig.json": """{
  // "ignore": "../server/api"
  "compilerOptions": { "baseUrl": "..", "paths": { "@api": ["server/api"] } },
  "extends": "@fixture/server/tsconfig",
  "exclude": ["../server"],
}""",
        }
        for name, content in fixture.items():
            path = root / name
            path.parent.mkdir(exist_ok=True, parents=True)
            path.write_text(content, encoding="utf-8")
        hits = list(violations(root))
        expected = [
            "app/module.cts:2: import @fixture/server/subpath reaches @fixture/server under the AGPL license",
            "app/package.json: dependencies.@fixture/server depends on @fixture/server under the AGPL license",
            "app/package.json: peerDependencies.alias depends on @fixture/server under the AGPL license",
            "app/package.json: optionalDependencies.local depends on server under the AGPL license",
            "app/path.mts:2: import ../server/api reaches server/api under the AGPL license",
            "app/tsconfig.json: tsconfig value server/api reaches server/api under the AGPL license",
            "app/tsconfig.json: tsconfig value @fixture/server/tsconfig reaches @fixture/server under the AGPL license",
        ]
        if hits != expected:
            raise ValueError(f"self-test expected {expected}, found {hits}")
        (root / "app/package.json").write_text("{", encoding="utf-8")
        try:
            list(violations(root))
        except ValueError as exc:
            if "app/package.json:1:2:" not in str(exc):
                raise
        else:
            raise ValueError("self-test accepted invalid package.json")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    self_test()
    if args.self_test:
        print("TypeScript license boundary fixtures passed.")
        return 0
    hits = list(violations(Path(__file__).resolve().parents[1]))
    if hits:
        print("\n".join(hits), file=sys.stderr)
        return 1
    print("check-ts-license-boundary: no Apache TypeScript reaches the AGPL tree")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
