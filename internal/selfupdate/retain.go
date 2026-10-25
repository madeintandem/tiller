package selfupdate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/madeintandem/tiller/internal/rootpath"
)

// RetainedExecutable is a usable recovery binary, not an automatic rollback.
// Directory can be prepended to PATH so child gates use the same prior binary.
type RetainedExecutable struct {
	Path      string `json:"path"`
	Directory string `json:"directory"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
}

// RetainExecutable preserves distinct outgoing builds without overwriting any
// existing file. A nonzero result with an error means a verified copy remains
// available, but the source changed and must not be replaced using this identity.
func RetainExecutable(ctx context.Context, identity ExecutableIdentity) (RetainedExecutable, error) {
	var retained RetainedExecutable
	if err := ctx.Err(); err != nil {
		return retained, err
	}
	if err := CheckExecutable(identity); err != nil {
		return retained, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return retained, fmt.Errorf("locating retained executable directory: %w", err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return retained, err
	}
	anchor, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return retained, err
	}
	defer anchor.Close()
	parent, err := (rootpath.Walker{}).OpenRoot(anchor, strings.TrimPrefix(home, string(filepath.Separator)))
	if err != nil {
		return retained, err
	}
	defer func() { parent.Close() }()
	build := identity.Version + "-" + identity.SHA256
	for index, segment := range []string{".local", "share", "tiller", "previous", build} {
		next, err := retentionDirectory(parent, segment, index >= 2)
		if err != nil {
			return retained, fmt.Errorf("opening retained executable directory: %w", err)
		}
		parent.Close()
		parent = next
	}
	directory := filepath.Join(home, ".local", "share", "tiller", "previous", build)
	path := filepath.Join(directory, "tiller")
	if _, err := parent.Lstat("tiller"); err == nil {
		if err := verifyRetained(ctx, parent, path, identity); err != nil {
			return retained, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return retained, err
	} else {
		if err := createRetained(ctx, parent, directory, identity); err != nil {
			return retained, err
		}
		if err := verifyRetained(ctx, parent, path, identity); err != nil {
			return retained, err
		}
	}
	retained = RetainedExecutable{Path: path, Directory: directory, Version: identity.Version, SHA256: identity.SHA256}
	if err := CheckExecutable(identity); err != nil {
		return retained, err
	}
	return retained, nil
}

func retentionDirectory(parent *os.Root, name string, private bool) (*os.Root, error) {
	if err := parent.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	child, err := (rootpath.Walker{}).OpenRoot(parent, name)
	if err != nil {
		return nil, err
	}
	info, err := child.Stat(".")
	if err != nil {
		child.Close()
		return nil, err
	}
	forbidden := os.FileMode(0022)
	if private {
		forbidden = 0077
	}
	if info.Mode().Perm()&forbidden != 0 {
		child.Close()
		return nil, fmt.Errorf("retention directory %s has unsafe permissions %04o", name, info.Mode().Perm())
	}
	// A synced executable is not durable if a newly created ancestor entry
	// can still disappear. Sync each parent before descending another level.
	directory, err := parent.Open(".")
	if err == nil {
		err = errors.Join(directory.Sync(), directory.Close())
	}
	if err != nil {
		child.Close()
		return nil, err
	}
	return child, nil
}

func createRetained(ctx context.Context, parent *os.Root, directory string, identity ExecutableIdentity) error {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	name := ".tiller-" + hex.EncodeToString(token[:])
	file, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0700)
	if err != nil {
		return err
	}
	defer parent.Remove(name)
	source, err := openExecutable(identity.Path)
	if err != nil {
		file.Close()
		return err
	}
	_, copyErr := io.Copy(file, source)
	err = errors.Join(copyErr, source.Close(), file.Sync(), file.Close())
	if err != nil {
		return fmt.Errorf("copying previous executable: %w", err)
	}
	if err := CheckExecutable(identity); err != nil {
		return err
	}
	if err := verifyRetained(ctx, parent, filepath.Join(directory, name), identity); err != nil {
		return err
	}
	// Link publishes without an overwrite window; Rename would replace a file
	// created by a concurrent retainer. The temporary hardlink is then removed.
	if err := parent.Link(name, "tiller"); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("publishing retained executable: %w", err)
	}
	dir, err := parent.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func verifyRetained(ctx context.Context, parent *os.Root, path string, source ExecutableIdentity) error {
	file, err := (rootpath.Walker{}).Open(parent, filepath.Base(path), false)
	if err != nil {
		return fmt.Errorf("refusing unverified retained executable %s: %w", path, err)
	}
	info, err := file.Stat()
	file.Close()
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0700 {
		return fmt.Errorf("refusing retained executable %s with unsafe permissions", path)
	}
	// Never execute a conflicting existing file just to discover its version.
	digest, observed, err := executableSnapshot(path)
	if err != nil {
		return err
	}
	if digest != source.SHA256 || !os.SameFile(info, observed) {
		return fmt.Errorf("refusing conflicting retained executable %s", path)
	}
	verified, err := ReadExecutable(ctx, path)
	if err != nil {
		return err
	}
	if verified.Version != source.Version || verified.SHA256 != source.SHA256 || !os.SameFile(info, verified.info) {
		return fmt.Errorf("retained executable %s does not match the outgoing executable", path)
	}
	return nil
}
