#!/usr/bin/env bash
# Builds the parity test sandbox: a freshly built kapi binary + a freshly
# built okapi-bridge plugin, isolated from the user's environment.
#
# Outputs the absolute sandbox path on stdout (the only line on stdout —
# all status messages go to stderr) so callers can capture it with:
#
#   sandbox=$(scripts/parity-sandbox.sh)
#
# Layout:
#   $ROOT/.parity/
#   ├── bin/kapi                     — locally built kapi
#   └── plugins/okapi-bridge/        — locally built v2 plugin tarball, unpacked
#
# Inputs (env):
#   OKAPI_BRIDGE_REPO   Path to okapi-bridge repo. Defaults to ../okapi-bridge.
#   OKAPI_VERSION       Okapi Framework version to build. Default 1.48.0.
#   PARITY_FORCE        When set, rebuild even if the sandbox exists.
#
# This script intentionally does not consult $XDG_DATA_HOME or
# ~/.local/share/kapi — the parity harness is required to measure the
# locally built bits, not whatever a developer happens to have installed.

set -euo pipefail

log() { echo "[parity-sandbox] $*" >&2; }

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
sandbox="${repo_root}/.parity"
plugins_dir="${sandbox}/plugins"
bin_dir="${sandbox}/bin"
bridge_install_dir="${plugins_dir}/okapi-bridge"

okapi_bridge_repo="${OKAPI_BRIDGE_REPO:-${repo_root}/../okapi-bridge}"
okapi_version="${OKAPI_VERSION:-1.48.0}"

if [ ! -d "$okapi_bridge_repo" ]; then
  echo "[parity-sandbox] FATAL: OKAPI_BRIDGE_REPO=$okapi_bridge_repo does not exist" >&2
  exit 1
fi

mkdir -p "$bin_dir" "$plugins_dir"

# 1. Build the kapi binary into the sandbox.
if [ -n "${PARITY_FORCE:-}" ] || [ ! -x "${bin_dir}/kapi" ]; then
  log "building kapi → ${bin_dir}/kapi"
  (cd "$repo_root" && make build BIN_DIR="$bin_dir") >&2
else
  log "kapi already built at ${bin_dir}/kapi (set PARITY_FORCE=1 to rebuild)"
fi

# 2. Build the okapi-bridge v2 plugin tarball.
host_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
host_arch="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
tarball_name="kapi-okapi-bridge_${okapi_version}_${host_os}_${host_arch}.tar.gz"
# package-release-v2.sh writes into dist/v2/.
tarball_path="${okapi_bridge_repo}/dist/v2/${tarball_name}"

if [ -n "${PARITY_FORCE:-}" ] || [ ! -f "$tarball_path" ]; then
  log "building okapi-bridge plugin v2 (V=${okapi_version})"
  (cd "$okapi_bridge_repo" && make plugin-v2 V="$okapi_version") >&2
else
  log "okapi-bridge tarball cached at ${tarball_path} (set PARITY_FORCE=1 to rebuild)"
fi

if [ ! -f "$tarball_path" ]; then
  echo "[parity-sandbox] FATAL: tarball not produced at $tarball_path" >&2
  exit 1
fi

# 3. Unpack into the sandbox plugins dir.
if [ -n "${PARITY_FORCE:-}" ] || [ ! -d "$bridge_install_dir" ]; then
  log "unpacking ${tarball_path} → ${bridge_install_dir}"
  rm -rf "$bridge_install_dir"
  mkdir -p "$bridge_install_dir"
  tar -xzf "$tarball_path" -C "$plugins_dir"
else
  log "okapi-bridge already installed at ${bridge_install_dir}"
fi

if [ ! -f "${bridge_install_dir}/manifest.json" ]; then
  echo "[parity-sandbox] FATAL: ${bridge_install_dir}/manifest.json missing after unpack" >&2
  exit 1
fi

echo "$sandbox"
