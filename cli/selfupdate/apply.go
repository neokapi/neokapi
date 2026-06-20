package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neokapi/neokapi/cli/pluginhost/registry"
)

// Trust anchors for verifying a self-update. The release index ships a cosign
// cert identity per platform, but we refuse to trust an identity that doesn't
// come from the neokapi release workflow — otherwise a tampered index could
// point us at an attacker-signed artifact. The issuer is pinned outright; the
// identity (which embeds the per-release tag) must carry the workflow prefix.
const (
	trustedCertIssuer         = "https://token.actions.githubusercontent.com"
	trustedCertIdentityPrefix = "https://github.com/neokapi/neokapi/.github/workflows/"
)

// maxBinaryBytes caps the extracted kapi binary size (defense against a
// decompression bomb in a tampered-but-somehow-valid archive).
const maxBinaryBytes = 512 << 20 // 512 MB

// Apply downloads, verifies, and installs the given release over the running
// binary. It refuses to proceed unless the artifact's SHA-256 matches the index
// and a cosign signature from the neokapi release workflow verifies — there is
// no unsafe override here (unlike `kapi plugin install`), because replacing the
// CLI's own binary is too sensitive to do unverified.
func Apply(ctx context.Context, rel *Release, onProgress func(downloaded, total int64)) error {
	exe, err := ExecutablePath()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}

	plat := rel.Platform
	if plat.URL == "" {
		return fmt.Errorf("release %s has no build for %s", rel.Version, registry.PlatformKey())
	}

	data, err := registry.DownloadWithProgress(ctx, plat.URL, onProgress)
	if err != nil {
		return err
	}
	if err := registry.VerifySHA256(data, plat.SHA256); err != nil {
		return err
	}
	if err := verifySignature(ctx, plat); err != nil {
		return err
	}

	bin, err := extractKapiBinary(data, plat.URL)
	if err != nil {
		return fmt.Errorf("extract kapi binary: %w", err)
	}
	if err := replaceBinary(exe, bin); err != nil {
		return fmt.Errorf("replace %s: %w", exe, err)
	}
	return nil
}

// verifySignature enforces the cosign keyless signature with pinned trust.
func verifySignature(ctx context.Context, plat registry.PlatformEntry) error {
	if plat.Signature == "" {
		return errors.New("release entry has no signature — refusing to self-update an unsigned build")
	}
	issuer := plat.CertOIDCIssuer
	if issuer == "" {
		issuer = trustedCertIssuer
	}
	if issuer != trustedCertIssuer {
		return fmt.Errorf("untrusted signing issuer %q", issuer)
	}
	if !strings.HasPrefix(plat.CertIdentity, trustedCertIdentityPrefix) {
		return fmt.Errorf("untrusted signing identity %q (must come from the neokapi release workflow)", plat.CertIdentity)
	}
	return registry.VerifyBundle(ctx, plat.Signature, strings.ToLower(plat.SHA256), plat.CertIdentity, issuer, registry.CosignVerifyOptions{})
}

// extractKapiBinary pulls the kapi (or kapi.exe) member out of a release
// archive. The archive shape is fixed by scripts/package-cli.sh:
// kapi_<ver>_<os>_<arch>.tar.gz holds a bare `kapi`; the Windows .zip holds
// `kapi.exe`.
func extractKapiBinary(archive []byte, url string) ([]byte, error) {
	want := binaryName()
	if strings.HasSuffix(strings.ToLower(url), ".zip") {
		return extractFromZip(archive, want)
	}
	return extractFromTarGz(archive, want)
}

func extractFromTarGz(archive []byte, want string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == want {
			return io.ReadAll(io.LimitReader(tr, maxBinaryBytes))
		}
	}
	return nil, fmt.Errorf("archive does not contain %q", want)
}

func extractFromZip(archive []byte, want string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range r.File {
		if filepath.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, maxBinaryBytes))
	}
	return nil, fmt.Errorf("archive does not contain %q", want)
}

// replaceBinary atomically swaps the new binary in for the one at target.
//
// The new bytes are written to a temp file in the *same* directory (so the
// final rename stays on one filesystem and is atomic). On Windows the running
// .exe cannot be deleted or overwritten, but it can be renamed: we move it
// aside to <target>.old, then move the new binary into place. The .old file is
// cleaned up on the next run.
func replaceBinary(target string, newBin []byte) error {
	dir := filepath.Dir(target)

	// Best-effort cleanup of a stale .old from a prior Windows update.
	_ = os.Remove(target + ".old")

	tmp, err := os.CreateTemp(dir, ".kapi-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanupTmp := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(newBin); err != nil {
		_ = tmp.Close()
		cleanupTmp()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanupTmp()
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		cleanupTmp()
		return err
	}

	if runtime.GOOS == "windows" {
		moved := target + ".old"
		if err := os.Rename(target, moved); err != nil {
			cleanupTmp()
			return fmt.Errorf("move running binary aside: %w", err)
		}
		if err := os.Rename(tmpName, target); err != nil {
			// Roll back: restore the original.
			_ = os.Rename(moved, target)
			cleanupTmp()
			return err
		}
		return nil
	}

	if err := os.Rename(tmpName, target); err != nil {
		cleanupTmp()
		return err
	}
	return nil
}
