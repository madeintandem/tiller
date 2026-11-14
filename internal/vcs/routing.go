package vcs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// ReasonGitRoutingConflict is the store.EnvError discriminator for an inherited
// repository override that does not name the selected checkout. It maps to
// exit 3, the environment class.
const ReasonGitRoutingConflict = "git_routing_conflict"

// routingVariables are the inherited repository overrides: the environment
// variables that tell Git which repository, work tree, index, or object store
// to use instead of discovering them from its working directory.
var routingVariables = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY"}

// The routing policy, for every git process this package starts:
//
// Tiller observes the checkout Git discovers from the selected directory
// (g.Dir), and nothing else. An inherited repository override never chooses the
// repository. When one is set it is checked against that discovery, and an
// override naming anything else refuses with git_routing_conflict, because Git
// would otherwise answer for another repository while gates run in this one.
// An override that agrees changes nothing, so a Git hook or a child of Git in
// a linked worktree, where Git sets these itself, observes what a plain shell
// observes. Either way the process runs without the overrides: Git resolves a
// relative GIT_DIR, and the work tree of a GIT_DIR that came without a
// GIT_WORK_TREE, against its own working directory, which here is the selected
// directory or the repository root and not the directory Tiller was started
// in.
//
// Other inherited Git settings (configuration, discovery limits, alternate
// object stores) are passed through. They can make an observation fail, but
// they cannot make Git answer for a different checkout.

// command prepares one git process under the routing policy. Diagnostics use a
// stable locale. Every observation reads the actual local objects: replacement
// refs cannot rewrite evidence, and missing promisor objects must not trigger
// fetching or writes as a side effect of a read.
func (g *Git) command(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.Dir
	for _, setting := range cmd.Environ() {
		if name, _, _ := strings.Cut(setting, "="); !slices.Contains(routingVariables, name) {
			cmd.Env = append(cmd.Env, setting)
		}
	}
	cmd.Env = append(cmd.Env, "LC_ALL=C", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1")
	return cmd
}

// checkRouting refuses when an inherited repository override names anything
// other than the checkout Git discovers from g.Dir. With no override set, the
// ordinary case, it starts no process.
func (g *Git) checkRouting() error {
	var overrides []string
	for _, variable := range routingVariables {
		if _, set := os.LookupEnv(variable); set {
			overrides = append(overrides, variable)
		}
	}
	if len(overrides) == 0 {
		return nil
	}
	found, err := g.discoverCheckout(overrides)
	if err != nil {
		return err
	}
	for _, variable := range overrides {
		value := os.Getenv(variable)
		named, err := filepath.Abs(value)
		if err != nil {
			return routingConflict("%s=%s could not be resolved: %v", variable, value, err)
		}
		var keeps, at string
		switch variable {
		case "GIT_DIR":
			keeps, at = "its Git directory", found.gitDir
		case "GIT_WORK_TREE":
			keeps, at = "its work tree", found.workTree
		case "GIT_COMMON_DIR":
			keeps, at = "its common Git directory", found.commonDir
		case "GIT_OBJECT_DIRECTORY":
			keeps, at = "its objects", found.objects
		case "GIT_INDEX_FILE":
			// A commit hook's index is a file in the checkout's own Git
			// directory: its index, or the temporary one `git commit -a`
			// writes. Git reads a relative value against the work tree root.
			if !filepath.IsAbs(value) {
				named = filepath.Join(found.workTree, value)
			}
			named, keeps, at = filepath.Dir(named), "its index files", found.gitDir
		}
		if !sameFile(named, at) {
			return routingConflict("%s=%s does not name the checkout selected at %s, which keeps %s at %s; unset %s, or run tiller in the checkout it names",
				variable, value, g.Dir, keeps, at, variable)
		}
	}
	return nil
}

// discoveredCheckout is where the checkout Git discovers from a directory
// keeps each part an override could reroute.
type discoveredCheckout struct{ gitDir, commonDir, workTree, objects string }

// discoverCheckout asks Git, without the overrides, which checkout g.Dir is
// in. No answer is a refusal too: with overrides set, there is no discovered
// checkout to hold them to.
func (g *Git) discoverCheckout(overrides []string) (discoveredCheckout, error) {
	// --git-path objects is the object directory.
	discover := g.command("rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir", "--show-toplevel", "--git-path", "objects")
	var stderr bytes.Buffer
	discover.Stderr = &stderr
	out, err := discover.Output()
	set := strings.Join(overrides, ", ")
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return discoveredCheckout{}, g.unavailable(err.Error())
		}
		return discoveredCheckout{}, routingConflict("%s set, but Git discovers no work tree from %s (%s); Tiller observes only the checkout discovered from the selected directory",
			set, g.Dir, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != 4 {
		return discoveredCheckout{}, routingConflict("%s set, but Git's description of the checkout at %s could not be read", set, g.Dir)
	}
	return discoveredCheckout{gitDir: lines[0], commonDir: lines[1], workTree: lines[2], objects: lines[3]}, nil
}

// sameFile reports whether two paths name one existing file or directory, so a
// symlinked or differently spelled path to the same place agrees and a path
// that does not exist never does.
func sameFile(a, b string) bool {
	left, err := os.Stat(a)
	if err != nil {
		return false
	}
	right, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(left, right)
}

func routingConflict(format string, args ...any) error {
	return &store.EnvError{
		Code:   3,
		Reason: ReasonGitRoutingConflict,
		Msg:    "git routing conflict: " + fmt.Sprintf(format, args...),
	}
}
