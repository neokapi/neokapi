#!/usr/bin/env bash
#
# publish-packages.sh — publish the kapi CLI .deb/.rpm into the self-hosted apt
# + yum repositories served at https://neokapi.github.io/packages/.
#
# It adds the packages built in <pkg-dir> (kapi-cli_*.deb / kapi-cli_*.rpm) to a
# clone of the packages repo, regenerates and GPG-signs the apt (flat) and yum
# (createrepo) indexes, (re)publishes the public key, and pushes. The signed
# indexes carry each package's SHA-256, so a tampered package is detected even
# though the packages themselves are not individually signed (apt: signed
# InRelease → Packages → checksums; yum: signed repomd → primary.xml →
# checksums). repo_gpgcheck on the client verifies the index signature.
#
# Required env:
#   PACKAGES_GPG_PRIVATE_KEY  ASCII-armored private signing key. MUST be RSA —
#                             rpm/dnf cannot verify ed25519 repomd signatures
#                             (apt accepts either; RSA satisfies both).
#   PACKAGES_TOKEN            token with write access to $PACKAGES_REPO
# Optional env:
#   PACKAGES_REPO   default neokapi/packages
#
# Ubuntu tools: apt-utils (apt-ftparchive), createrepo-c, gpg, gzip.
#
# Usage: publish-packages.sh <version> <pkg-dir>
set -euo pipefail

version="${1:?version required}"
pkgdir="${2:?package dir required}"
PACKAGES_REPO="${PACKAGES_REPO:-neokapi/packages}"

: "${PACKAGES_GPG_PRIVATE_KEY:?PACKAGES_GPG_PRIVATE_KEY not set}"
: "${PACKAGES_TOKEN:?PACKAGES_TOKEN not set}"

shopt -s nullglob
debs=( "$pkgdir"/kapi-cli_*.deb )
rpms=( "$pkgdir"/kapi-cli_*.rpm )
if [ ${#debs[@]} -eq 0 ] && [ ${#rpms[@]} -eq 0 ]; then
  echo "publish-packages.sh: no kapi-cli_*.deb/.rpm in $pkgdir" >&2
  exit 1
fi

# --- import the signing key into a throwaway keyring ---
GNUPGHOME="$(mktemp -d)"; export GNUPGHOME
chmod 700 "$GNUPGHOME"
printf '%s' "$PACKAGES_GPG_PRIVATE_KEY" | gpg --batch --import
KEYID="$(gpg --list-secret-keys --with-colons | awk -F: '/^sec:/{print $5; exit}')"
[ -n "$KEYID" ] || { echo "publish-packages.sh: no secret key imported" >&2; exit 1; }
echo ">> signing with key $KEYID" >&2
gpgsign() { gpg --batch --yes --pinentry-mode loopback --default-key "$KEYID" "$@"; }

# --- clone the packages repo ---
# PACKAGES_GIT_URL overrides the clone URL (tests point it at a local repo);
# by default it's the token-authenticated GitHub URL for $PACKAGES_REPO.
work="$(mktemp -d)"
git_url="${PACKAGES_GIT_URL:-https://x-access-token:${PACKAGES_TOKEN}@github.com/${PACKAGES_REPO}.git}"
git clone --depth 1 "$git_url" "$work/repo"
repo="$work/repo"
mkdir -p "$repo/apt/pool" "$repo/yum"

# (re)publish the public key for clients to trust
gpg --armor --export "$KEYID" > "$repo/neokapi.gpg"

# --- APT: flat repo (deb [signed-by=…] <base>/apt ./) ---
if [ ${#debs[@]} -gt 0 ]; then
  cp "${debs[@]}" "$repo/apt/pool/"
  (
    cd "$repo/apt"
    apt-ftparchive packages pool > Packages
    gzip -9kf Packages
    apt-ftparchive \
      -o APT::FTPArchive::Release::Origin=neokapi \
      -o APT::FTPArchive::Release::Label=neokapi \
      -o APT::FTPArchive::Release::Suite=stable \
      -o APT::FTPArchive::Release::Codename=stable \
      -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
      -o APT::FTPArchive::Release::Components=main \
      release . > Release
    gpgsign -abs -o Release.gpg Release
    gpgsign --clearsign -o InRelease Release
  )
fi

# --- YUM: createrepo + signed repomd.xml ---
if [ ${#rpms[@]} -gt 0 ]; then
  cp "${rpms[@]}" "$repo/yum/"
  createrepo_c --update "$repo/yum"
  rm -f "$repo/yum/repodata/repomd.xml.asc"
  gpgsign --detach-sign --armor "$repo/yum/repodata/repomd.xml"
fi

# --- commit + push (rebase-retry; release jobs may push concurrently) ---
cd "$repo"
git config user.email "release-bot@neokapi.dev"
git config user.name "release-bot"
git add -A
if git diff --staged --quiet; then
  echo ">> packages unchanged; nothing to publish" >&2
  exit 0
fi
git commit -m "packages: kapi-cli ${version}"
for i in 1 2 3 4 5; do
  if git push; then
    echo ">> published kapi-cli ${version} to apt + yum" >&2
    exit 0
  fi
  echo ">> push race on packages repo, rebasing (attempt $i)…" >&2
  git pull --rebase --no-edit || true
done
echo "publish-packages.sh: failed to push after retries" >&2
exit 1
