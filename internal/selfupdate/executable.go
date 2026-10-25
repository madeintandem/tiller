package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/rootpath"
)

// ExecutableIdentity describes bytes verified by executing this exact path.
// Keep the returned value intact: its private snapshot guards later replacement.
type ExecutableIdentity struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`

	source string
	info   os.FileInfo
}

var executableVersion = regexp.MustCompile(`^tiller version (v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

// ReadExecutable resolves aliases once, hashes the executable, asks that exact
// executable for its version, and refuses any intervening identity change.
func ReadExecutable(ctx context.Context, path string) (ExecutableIdentity, error) {
	var identity ExecutableIdentity
	if err := ctx.Err(); err != nil {
		return identity, err
	}
	source, err := filepath.Abs(path)
	if err != nil {
		return identity, err
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return identity, fmt.Errorf("resolving executable %s: %w", source, err)
	}
	digest, info, err := executableSnapshot(resolved)
	if err != nil {
		return identity, err
	}
	identity = ExecutableIdentity{Path: resolved, SHA256: digest, source: source, info: info}
	out, err := executableCommand(ctx, 10*time.Second, resolved, nil, "--version")
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("verifying executable version: %w", err)
	}
	version := executableVersion.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(version) != 2 || len(version[1]) > 128 {
		return ExecutableIdentity{}, fmt.Errorf("%s did not report a valid tiller version", resolved)
	}
	identity.Version = strings.TrimPrefix(version[1], "v")
	if err := CheckExecutable(identity); err != nil {
		return ExecutableIdentity{}, err
	}
	return identity, nil
}

// CheckExecutable checks both the original alias and the resolved bytes. It does
// not rerun the version subprocess: the verified digest already binds that fact.
// Call immediately before replacement or handing project writes to this binary.
func CheckExecutable(identity ExecutableIdentity) error {
	if identity.info == nil || identity.source == "" || len(identity.Version) > 128 || !executableVersion.MatchString("tiller version "+identity.Version) {
		return fmt.Errorf("executable identity was not verified by ReadExecutable")
	}
	resolved, err := filepath.EvalSymlinks(identity.source)
	if err != nil || resolved != identity.Path {
		return fmt.Errorf("executable %s changed since verification", identity.source)
	}
	digest, info, err := executableSnapshot(identity.Path)
	if err != nil {
		return fmt.Errorf("rechecking executable %s: %w", identity.Path, err)
	}
	if digest != identity.SHA256 || !sameExecutableFile(identity.info, info) {
		return fmt.Errorf("executable %s changed since verification", identity.Path)
	}
	return nil
}

func executableSnapshot(path string) (string, os.FileInfo, error) {
	file, err := openExecutable(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !sameExecutableFile(before, after) || !sameExecutableFile(after, current) {
		return "", nil, fmt.Errorf("executable %s changed while reading it", path)
	}
	return hex.EncodeToString(h.Sum(nil)), after, nil
}

func openExecutable(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("executable path %q is not absolute and canonical", path)
	}
	root, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := (rootpath.Walker{}).Open(root, strings.TrimPrefix(path, string(filepath.Separator)), false)
	if err != nil {
		return nil, fmt.Errorf("opening executable %s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil || info.Mode().Perm()&0111 == 0 {
		file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s is not executable", path)
	}
	return file, nil
}

func sameExecutableFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
