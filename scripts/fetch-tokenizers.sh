#!/usr/bin/env bash
#
# Fetch the pinned daulet/tokenizers static library, or its source, for one
# platform.
#
# kapi-sat and kapi-check link libtokenizers.a at build time. darwin and linux
# use the prebuilt archive from the release; each archive is checked against
# the SHA-256 recorded below before anything is extracted, so a changed or
# truncated download fails here and never reaches a plugin tarball. Windows
# has no prebuilt archive and builds the tokenizers-ffi crate from source: the
# `source` form clones the release tag and checks that it names the commit
# recorded below, so a moved tag fails here too.
#
# Usage:
#   scripts/fetch-tokenizers.sh lib <goos> <goarch> <dest>   # <dest>/libtokenizers.a
#   scripts/fetch-tokenizers.sh source <dest>                # a checkout at the pinned commit
#
# TOKENIZERS_VERSION selects the version (default: the pin below, without the
# leading v). A version with no recorded checksums is refused: add them here in
# the same change that raises the pin. The version must match the
# github.com/daulet/tokenizers line in plugins/sat/go.mod and
# plugins/check/go.mod (scripts/check-tool-versions.py checks that).
set -euo pipefail

readonly DEFAULT_VERSION="1.27.0"
version="${TOKENIZERS_VERSION:-$DEFAULT_VERSION}"
version="${version#v}"

# The commit the version's tag names, and the SHA-256 of each prebuilt asset.
case "$version" in
  1.27.0)
    commit="f678a7768d5479d9d5a5161c4fc45c8a5ba46146"
    sha_darwin_arm64="fb84b8b2e349a5952767ffe80ccd862fc44084de47f3b0cc3f0b7c9d4e649cf7"
    sha_darwin_amd64="6239efe5a81fde8089ef2df8ae710366542b4e5deab6d8ecb74d7d1862db2ddb"
    sha_linux_amd64="72556cdca798dd4ea7cdaba308e5f0d68a8cb93b67c96edf485b7a0edd7b07f4"
    sha_linux_arm64="e96545ad05930c26f51f63d932ee6d3bbd32bbed149e102c5290d587a2293067"
    ;;
  *)
    echo "fetch-tokenizers: no checksums recorded for tokenizers $version" >&2
    echo "fetch-tokenizers: add them to scripts/fetch-tokenizers.sh" >&2
    exit 1
    ;;
esac

usage() {
  echo "usage: $0 lib <goos> <goarch> <dest> | $0 source <dest>" >&2
  exit 2
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

fetch_lib() {
  local goos="$1" goarch="$2" dest="$3" asset sha
  case "$goos/$goarch" in
    darwin/arm64) asset="libtokenizers.darwin-arm64.tar.gz"; sha="$sha_darwin_arm64" ;;
    darwin/amd64) asset="libtokenizers.darwin-x86_64.tar.gz"; sha="$sha_darwin_amd64" ;;
    linux/amd64)  asset="libtokenizers.linux-amd64.tar.gz"; sha="$sha_linux_amd64" ;;
    linux/arm64)  asset="libtokenizers.linux-arm64.tar.gz"; sha="$sha_linux_arm64" ;;
    *)
      echo "fetch-tokenizers: no prebuilt tokenizers library for $goos/$goarch; use the source form" >&2
      exit 1
      ;;
  esac

  local work
  work="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $work now, not at trap time
  trap "rm -rf '$work'" EXIT

  local url="https://github.com/daulet/tokenizers/releases/download/v${version}/${asset}"
  echo "fetch-tokenizers: downloading $url"
  curl -sSfL --retry 3 --retry-delay 5 -o "$work/$asset" "$url"

  local actual
  actual="$(sha256_of "$work/$asset")"
  if [ "$actual" != "$sha" ]; then
    echo "fetch-tokenizers: checksum mismatch for $asset (tokenizers $version)" >&2
    echo "fetch-tokenizers:   expected $sha" >&2
    echo "fetch-tokenizers:   actual   $actual" >&2
    exit 1
  fi

  mkdir -p "$dest"
  tar -xzf "$work/$asset" -C "$dest"
  if [ ! -s "$dest/libtokenizers.a" ]; then
    echo "fetch-tokenizers: libtokenizers.a is missing from $asset" >&2
    exit 1
  fi
  echo "fetch-tokenizers: $dest/libtokenizers.a"
}

fetch_source() {
  local dest="$1"
  git -c advice.detachedHead=false clone --quiet --branch "v${version}" --depth 1 https://github.com/daulet/tokenizers "$dest"
  local head
  head="$(git -C "$dest" rev-parse HEAD)"
  if [ "$head" != "$commit" ]; then
    echo "fetch-tokenizers: tag v${version} names $head, not the recorded $commit" >&2
    exit 1
  fi
  echo "fetch-tokenizers: $dest at $commit"
}

case "${1:-}" in
  lib)
    [ "$#" -eq 4 ] || usage
    fetch_lib "$2" "$3" "$4"
    ;;
  source)
    [ "$#" -eq 2 ] || usage
    fetch_source "$2"
    ;;
  *) usage ;;
esac
