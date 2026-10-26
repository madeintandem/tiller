package selfupdate_test

// These tests pin the filesystem half of internal/selfupdate
// (docs/spec/distribution.md): naming the release asset for this OS and arch,
// verifying a downloaded archive's checksum against checksums.txt, extracting
// the tiller member from the tar.gz, swapping it onto a target path without
// ever truncating that path in place, resolving the running executable
// (with a test seam), and detecting a Homebrew-managed install. install.sh
// is the semantic reference for the checksum and extraction steps (its
// sha256/tar/chmod sequence), but every fixture archive here is built
// in-test with archive/tar and compress/gzip rather than read from disk.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/stretchr/testify/require"
)

// tarMember is one entry to place in a fixture archive built by
// buildArchive. typeflag defaults to tar.TypeReg (a plain file) when left
// zero; set it explicitly (with linkname) to build a symlink or hardlink
// entry.
type tarMember struct {
	name     string
	body     []byte
	mode     int64
	typeflag byte
	linkname string
}

// buildArchive writes a gzip-compressed tar archive named archiveName in
// dir, containing members, and returns its path.
func buildArchive(t *testing.T, dir, archiveName string, members []tarMember) string {
	t.Helper()

	path := filepath.Join(dir, archiveName)
	f, err := os.Create(path)
	require.NoError(t, err)

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	for _, m := range members {
		typeflag := m.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     m.name,
			Mode:     m.mode,
			Size:     int64(len(m.body)),
			Typeflag: typeflag,
			Linkname: m.linkname,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if len(m.body) > 0 {
			_, err := tw.Write(m.body)
			require.NoError(t, err)
		}
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
	return path
}

// sha256Hex returns the hex-encoded sha256 of the file at path.
func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// flipAByte mutates the first byte of the file at path, simulating a
// download corrupted or tampered with in flight.
func flipAByte(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	data[0] ^= 0xFF
	require.NoError(t, os.WriteFile(path, data, 0644))
}

func TestAssetNameMatchesGoreleaser(t *testing.T) {
	require.Equal(t, "tiller_0.9.5_darwin_arm64.tar.gz", selfupdate.AssetName("v0.9.5", "darwin", "arm64"))
	require.Equal(t, "tiller_0.9.5_darwin_arm64.tar.gz", selfupdate.AssetName("0.9.5", "darwin", "arm64"))
}

func TestVerifyChecksumAcceptsAMatchingSum(t *testing.T) {
	dir := t.TempDir()
	archive := buildArchive(t, dir, "tiller_0.9.5_darwin_arm64.tar.gz", []tarMember{
		{name: "tiller", body: []byte("binary-bytes"), mode: 0755},
	})
	sum := sha256Hex(t, archive)

	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums, []byte(sum+"  tiller_0.9.5_darwin_arm64.tar.gz\n"), 0644))

	err := selfupdate.VerifyChecksum(archive, checksums, "tiller_0.9.5_darwin_arm64.tar.gz")
	require.NoError(t, err)
}

func TestVerifyChecksumRefusesAMismatch(t *testing.T) {
	dir := t.TempDir()
	archive := buildArchive(t, dir, "tiller_0.9.5_darwin_arm64.tar.gz", []tarMember{
		{name: "tiller", body: []byte("binary-bytes"), mode: 0755},
	})
	recordedSum := sha256Hex(t, archive)

	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums, []byte(recordedSum+"  tiller_0.9.5_darwin_arm64.tar.gz\n"), 0644))

	flipAByte(t, archive)
	actualSum := sha256Hex(t, archive)
	require.NotEqual(t, recordedSum, actualSum)

	err := selfupdate.VerifyChecksum(archive, checksums, "tiller_0.9.5_darwin_arm64.tar.gz")
	require.Error(t, err)
	require.ErrorContains(t, err, "tiller_0.9.5_darwin_arm64.tar.gz")
	require.ErrorContains(t, err, recordedSum)
	require.ErrorContains(t, err, actualSum)
	require.ErrorContains(t, err, "refusing")
}

func TestVerifyChecksumNamesAMissingEntry(t *testing.T) {
	dir := t.TempDir()
	archive := buildArchive(t, dir, "tiller_0.9.5_darwin_arm64.tar.gz", []tarMember{
		{name: "tiller", body: []byte("binary-bytes"), mode: 0755},
	})

	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums, []byte("deadbeef  some_other_asset.tar.gz\n"), 0644))

	err := selfupdate.VerifyChecksum(archive, checksums, "tiller_0.9.5_darwin_arm64.tar.gz")
	require.Error(t, err)
	require.ErrorContains(t, err, "tiller_0.9.5_darwin_arm64.tar.gz")
}

func TestExtractBinaryPullsOnlyTheTillerMember(t *testing.T) {
	dir := t.TempDir()
	archive := buildArchive(t, dir, "tiller_0.9.5_darwin_arm64.tar.gz", []tarMember{
		{name: "tiller", body: []byte("binary-bytes"), mode: 0755},
		{name: "LICENSE", body: []byte("license text"), mode: 0644},
		{name: "README.md", body: []byte("readme text"), mode: 0644},
	})

	dst := t.TempDir()
	path, err := selfupdate.ExtractBinary(archive, dst)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dst, "tiller.download"), path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "binary-bytes", string(got))

	_, err = os.Stat(filepath.Join(dst, "LICENSE"))
	require.True(t, os.IsNotExist(err), "LICENSE should not have been extracted")
	_, err = os.Stat(filepath.Join(dst, "README.md"))
	require.True(t, os.IsNotExist(err), "README.md should not have been extracted")
}

func TestExtractBinaryNamesAMissingMember(t *testing.T) {
	dir := t.TempDir()
	archive := buildArchive(t, dir, "tiller_0.9.5_darwin_arm64.tar.gz", []tarMember{
		{name: "LICENSE", body: []byte("license text"), mode: 0644},
		{name: "README.md", body: []byte("readme text"), mode: 0644},
	})

	_, err := selfupdate.ExtractBinary(archive, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, archive)
}

func TestExtractBinarySkipsANonRegularTillerEntry(t *testing.T) {
	dir := t.TempDir()
	// A malformed or hostile archive could carry a symlink named "tiller"
	// ahead of the real regular-file member; ExtractBinary must look past
	// it rather than following it and landing an empty (or attacker-chosen)
	// target.
	archive := buildArchive(t, dir, "tiller_0.9.5_darwin_arm64.tar.gz", []tarMember{
		{name: "tiller", typeflag: tar.TypeSymlink, linkname: "/usr/local/bin/tiller-real"},
		{name: "tiller", body: []byte("binary-bytes"), mode: 0755, typeflag: tar.TypeReg},
	})

	dst := t.TempDir()
	path, err := selfupdate.ExtractBinary(archive, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "binary-bytes", string(got))
}

func TestExtractBinaryNamesAMissingMemberWhenOnlyASymlinkMatches(t *testing.T) {
	dir := t.TempDir()
	archive := buildArchive(t, dir, "tiller_0.9.5_darwin_arm64.tar.gz", []tarMember{
		{name: "tiller", typeflag: tar.TypeSymlink, linkname: "/usr/local/bin/tiller-real"},
	})

	_, err := selfupdate.ExtractBinary(archive, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, archive)
}

func TestSwapBinaryReplacesTheTargetAtomically(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tiller")
	require.NoError(t, os.WriteFile(target, []byte("old-bytes"), 0755))

	newPath := filepath.Join(dir, "tiller.download")
	// Deliberately not 0755: SwapBinary must guarantee the target's mode
	// itself, not merely inherit whatever mode the source happened to
	// carry in from ExtractBinary.
	require.NoError(t, os.WriteFile(newPath, []byte("new-bytes"), 0644))

	err := selfupdate.SwapBinary(newPath, target)
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "new-bytes", string(got))

	info, err := os.Stat(target)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm())

	_, err = os.Stat(newPath)
	require.True(t, os.IsNotExist(err), "expected %s to be gone after the swap", newPath)
}

func TestExecutablePathHonorsTheTargetSeam(t *testing.T) {
	target := filepath.Join(t.TempDir(), "tiller-copy")
	require.NoError(t, os.WriteFile(target, []byte("fake-binary"), 0755))
	t.Setenv("TILLER_UPGRADE_TARGET", target)

	got, err := selfupdate.ExecutablePath()
	require.NoError(t, err)
	require.Equal(t, target, got)
}

func TestHomebrewManagedDetectsCellarAndCaskroom(t *testing.T) {
	require.True(t, selfupdate.HomebrewManaged("/opt/homebrew/Caskroom/tiller/0.9.5/tiller"))
	require.True(t, selfupdate.HomebrewManaged("/usr/local/Cellar/tiller/0.9.5/bin/tiller"))
	require.False(t, selfupdate.HomebrewManaged("/Users/x/.local/bin/tiller"))
	require.False(t, selfupdate.HomebrewManaged("/opt/CellarDoor/tiller"))
}
