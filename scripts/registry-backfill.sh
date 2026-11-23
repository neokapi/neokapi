#!/usr/bin/env bash
# Re-register a published plugin version with every platform its GitHub release
# carries, in a local checkout of the registry.
#
#   scripts/registry-backfill.sh <plugin> <tag> <registry checkout>
#   e.g. scripts/registry-backfill.sh bowrain bowrain-v1.3.0 ../registry
#
# From 1.2.0 on, every bowrain release built, signed and uploaded the Windows
# kapi-bowrain zips, but the registry step registered darwin and linux only, so
# Windows installs stayed on 1.1.0. This adds the missing platforms to versions
# already in the registry. It keeps the entry's released date, channel, minimum
# kapi version and signer identity, takes the checksums from the release's own
# checksums.txt, and verifies each archive's Sigstore bundle against that
# identity with cosign before writing anything. It writes manifest-plugins.json
# and commits nothing: review the diff, then commit and push the registry.
#
# Needs gh, cosign, go and python3.
set -euo pipefail

plugin="${1:?usage: registry-backfill.sh <plugin> <tag> <registry checkout>}"
tag="${2:?usage: registry-backfill.sh <plugin> <tag> <registry checkout>}"
registry="${3:?usage: registry-backfill.sh <plugin> <tag> <registry checkout>}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo="${GITHUB_REPOSITORY:-neokapi/neokapi}"
index="$registry/manifest-plugins.json"
[ -f "$index" ] || { echo "registry-backfill: no $index" >&2; exit 2; }
for tool in gh cosign go python3; do
  command -v "$tool" > /dev/null || { echo "registry-backfill: needs $tool" >&2; exit 2; }
done

version="${tag##*v}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The fields the existing entry carries, which the backfill keeps.
read -r released channel min_kapi cert_id issuer < <(python3 - "$index" "$plugin" "$version" <<'PY'
import json, sys
reg = json.load(open(sys.argv[1]))
entry = reg["plugins"][sys.argv[2]]["versions"].get(sys.argv[3])
if entry is None:
    sys.exit(f"registry-backfill: {sys.argv[2]} {sys.argv[3]} is not in the registry")
plat = next(iter((entry.get("platforms") or {}).values()), {})
print(entry.get("released", "-"), entry.get("channel", "-"), entry.get("min_kapi_version") or "-",
      plat.get("cert_identity", "-"), plat.get("cert_oidc_issuer", "-"))
PY
)

gh release download "$tag" --repo "$repo" --pattern checksums.txt --dir "$work"
base="https://github.com/${repo}/releases/download/${tag}"
args="$(python3 "$root/scripts/release-assets.py" registry-args "$plugin" "$version" "$work/checksums.txt" "$base")"
ARGS=()
while IFS= read -r line; do ARGS+=("$line"); done <<< "$args"

# Each archive's Sigstore bundle must verify against the identity the entry
# already records before the archive goes into the index.
for ((i = 0; i < ${#ARGS[@]}; i += 8)); do
  url="${ARGS[i + 3]}"
  name="${url##*/}"
  gh release download "$tag" --repo "$repo" --pattern "$name" --pattern "$name.sigstore.json" --dir "$work" --clobber
  cosign verify-blob --bundle "$work/$name.sigstore.json" \
    --certificate-identity "$cert_id" --certificate-oidc-issuer "$issuer" "$work/$name" > /dev/null
  echo "registry-backfill: $name verifies against $cert_id"
done

GOBIN="$work/bin" go install github.com/neokapi/registry/tools/registry-update@main
# A stable entry records no channel; an empty --channel keeps it that way.
[ "$channel" = "-" ] && channel=""
extra=()
[ "$min_kapi" = "-" ] || extra=(--min-kapi-version "$min_kapi")
"$work/bin/registry-update" --registry "$index" --plugin "$plugin" --version "$version" \
  --channel "$channel" --released "$released" \
  --cert-identity "$cert_id" --cert-oidc-issuer "$issuer" ${extra[@]+"${extra[@]}"} "${ARGS[@]}"
echo "registry-backfill: $plugin $version now lists $(printf '%s\n' "${ARGS[@]}" | awk 'prev == "--platform" { printf "%s ", $0 } { prev = $0 }')"
