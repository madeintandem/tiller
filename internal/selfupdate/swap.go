package selfupdate

// This file holds the filesystem half of the upgrade: naming the release
// asset for a platform, verifying a downloaded archive's checksum against
// checksums.txt, extracting the tiller member from the tar.gz, swapping it
// onto the running binary's path, resolving that path, and detecting a
// Homebrew-managed install. Task 1's release.go is the network half; this
// file is independent of it and shares only the package.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// AssetName builds the GoReleaser archive name for one release and
// platform, e.g. "tiller_0.9.5_darwin_arm64.tar.gz". version accepts either
// form, "v0.9.5" or "0.9.5"; the archive name itself never carries the "v".
func AssetName(version, goos, goarch string) string {
	bare := strings.TrimPrefix(version, "v")
	return fmt.Sprintf("tiller_%s_%s_%s.tar.gz", bare, goos, goarch)
}

// VerifyChecksum checks archivePath's sha256 against the entry for
// assetName in checksumsPath, a checksums.txt whose lines read
// "<hex><spaces><name>". A missing entry and a mismatched sum are distinct
// errors, both naming assetName; the mismatch error also carries both hex
// digests and the word "refusing", install.sh's posture that a bad sum
// never installs.
func VerifyChecksum(archivePath, checksumsPath, assetName string) error {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("reading checksums from %s: %w", checksumsPath, err)
	}

	expected, ok := findChecksum(string(data), assetName)
	if !ok {
		return fmt.Errorf("checksums.txt has no entry for %s", assetName)
	}

	actual, err := sha256File(archivePath)
	if err != nil {
		return fmt.Errorf("hashing %s: %w", archivePath, err)
	}

	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s, refusing to install", assetName, expected, actual)
	}
	return nil
}

// findChecksum scans checksums.txt content for the line naming assetName
// and returns its hex digest.
func findChecksum(checksumsText, assetName string) (string, bool) {
	for _, line := range strings.Split(checksumsText, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == assetName {
			return fields[0], true
		}
	}
	return "", false
}

// sha256File hashes the file at path and returns its hex digest.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExtractBinary streams archivePath as a tar.gz and extracts exactly the
// member named "tiller" to dstDir/tiller.download at mode 0755, returning
// that path. Every other member is skipped, including a "tiller" entry
// that is a symlink or hardlink rather than a regular file: a malformed or
// hostile archive could carry one of those ahead of the real member, and
// following it would land an empty or attacker-chosen file instead of the
// binary. A regular "tiller" member absent from the archive is an error
// naming archivePath.
func ExtractBinary(archivePath, dstDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening archive %s: %w", archivePath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("reading archive %s as gzip: %w", archivePath, err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading archive %s: %w", archivePath, err)
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Name != "tiller" {
			continue
		}
		return writeExtractedBinary(tr, dstDir)
	}

	return "", fmt.Errorf("archive %s has no tiller member", archivePath)
}

// writeExtractedBinary copies the tiller member currently positioned at r
// to dstDir/tiller.download at mode 0755 and returns that path.
func writeExtractedBinary(r io.Reader, dstDir string) (string, error) {
	dst := filepath.Join(dstDir, "tiller.download")

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("creating %s: %w", dst, err)
	}

	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		os.Remove(dst)
		return "", fmt.Errorf("extracting tiller to %s: %w", dst, err)
	}

	if err := out.Close(); err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("closing %s: %w", dst, err)
	}

	// OpenFile's mode is filtered by umask, so pin it explicitly: a running
	// binary must land executable regardless of the caller's umask.
	if err := os.Chmod(dst, 0755); err != nil {
		return "", fmt.Errorf("setting mode on %s: %w", dst, err)
	}
	return dst, nil
}

// SwapBinary replaces targetPath with newPath by rename, the one operation
// that can put a new binary in an old one's place without ever truncating
// it: a running process holds its own inode open, so a rename swaps what
// the path points to while an in-place rewrite would corrupt the bytes it
// is still executing. It guarantees the target lands at mode 0755
// regardless of newPath's own mode, so a caller never has to trust an
// upstream extraction step to have gotten the mode right; it does this by
// chmod-ing newPath itself before ever renaming it, the same order
// stageCopy already uses for the cross-device fallback's own temp file, so
// once a rename lands there is nothing left that can fail. When newPath and
// targetPath sit on different filesystems, rename fails with a cross-device
// error; the fallback copies newPath into a temp file in targetPath's own
// directory and renames that temp file into place instead, so the atomic
// swap always lands on targetPath's filesystem. Once either rename lands,
// the swap is durable and SwapBinary returns nil: cleaning up the
// now-redundant newPath is strictly best effort from that point on, its
// error ignored, so a caller can trust that a non-nil return always means
// the swap itself did not happen.
func SwapBinary(newPath, targetPath string) error {
	if err := os.Chmod(newPath, 0755); err != nil {
		return fmt.Errorf("setting mode on %s: %w", newPath, err)
	}

	if err := renameFile(newPath, targetPath); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return fmt.Errorf("swapping %s onto %s: %w", newPath, targetPath, err)
	}

	tmp, err := stageCopy(newPath, filepath.Dir(targetPath))
	if err != nil {
		return fmt.Errorf("staging %s alongside %s: %w", newPath, targetPath, err)
	}

	if err := renameFile(tmp, targetPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("swapping %s onto %s: %w", tmp, targetPath, err)
	}

	// targetPath now holds the new binary; the swap already succeeded.
	// newPath is the caller's own staged download, discarded either way,
	// so a failure here is not the caller's problem to react to.
	_ = os.Remove(newPath)
	return nil
}

// renameFile is os.Rename indirected through a package variable so tests
// can simulate a cross-device rename failure without needing two real
// filesystems.
var renameFile = os.Rename

// isCrossDevice reports whether err is the "invalid cross-device link"
// rename fails with when its source and destination sit on different
// filesystems.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// stageCopy copies srcPath into a new mode-0755 temp file inside dir and
// returns the temp file's path, ready for a same-directory rename.
func stageCopy(srcPath, dir string) (string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	tmp, err := os.CreateTemp(dir, "tiller-upgrade-*")
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// ExecutablePath resolves the running binary's real, symlink-free path via
// os.Executable and filepath.EvalSymlinks. The TILLER_UPGRADE_TARGET
// environment seam, when set, overrides both, so tests and scratch
// rehearsals can point an upgrade at a copy instead of the live test
// binary.
func ExecutablePath() (string, error) {
	if target := os.Getenv("TILLER_UPGRADE_TARGET"); target != "" {
		return target, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving the running executable: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks for %s: %w", exe, err)
	}
	return resolved, nil
}

// HomebrewManaged reports whether resolvedPath was placed by Homebrew: a
// formula lands under a "/Cellar/" segment and a cask under "/Caskroom/".
// The tiller cask's binaries stanza links from <prefix>/bin into the
// Caskroom, so the EvalSymlinks-resolved path lands there for brew
// installs, letting `tiller upgrade` recognize one and defer to `brew
// upgrade` instead of overwriting a file Homebrew manages.
func HomebrewManaged(resolvedPath string) bool {
	return strings.Contains(resolvedPath, "/Cellar/") || strings.Contains(resolvedPath, "/Caskroom/")
}
