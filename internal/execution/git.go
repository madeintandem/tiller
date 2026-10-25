package execution

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type repository struct {
	root          string
	projectPrefix string
	common        string
	refs          map[string]gitRef
	worktrees     map[string][]string
	config        map[string][]string
	servedBranch  string
}

type gitRef struct {
	oid      string
	upstream string
	push     string
}

type treeEntry struct{ mode, kind, oid string }

// Git is invoked only for local inspection. Disable optional locks and inherited
// repository overrides so another checkout cannot masquerade as this source.
func gitRead(root string, args ...string) ([]byte, error) {
	cmd, stderr := gitCommand(root, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, gitError(args, err, stderr)
	}
	return out, nil
}

// gitReadAtMost is gitRead with a byte cap: it reads at most limit+1 bytes of
// output and refuses a longer answer with ErrTooLarge, stopping Git rather
// than buffering the rest.
func gitReadAtMost(root string, limit int64, args ...string) ([]byte, error) {
	cmd, stderr := gitCommand(root, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, gitError(args, err, stderr)
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	if int64(len(out)) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, ErrTooLarge
	}
	if err := cmd.Wait(); err != nil {
		return nil, gitError(args, err, stderr)
	}
	if readErr != nil {
		return nil, readErr
	}
	return out, nil
}

func gitCommand(root string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	cmd := exec.Command("git", append([]string{"-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = root
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if !strings.HasPrefix(name, "GIT_") {
			cmd.Env = append(cmd.Env, variable)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd, &stderr
}

func gitError(args []string, err error, stderr *bytes.Buffer) error {
	return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
}

func gitText(root string, args ...string) (string, error) {
	out, err := gitRead(root, args...)
	return strings.TrimSpace(string(out)), err
}

func discover(root string) (*repository, error) {
	// Do not misclassify malformed .git metadata or an unavailable Git binary
	// as an ordinary non-Git fixture. Walk parents to honor nested served roots.
	if !hasGitMetadata(root) {
		return nil, nil
	}
	top, err := gitText(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil {
		return nil, err
	}
	project, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	prefix, err := filepath.Rel(top, project)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return nil, errors.New("served project is outside its Git worktree")
	}
	if prefix == "." {
		prefix = ""
	}
	root = top
	common, err := commonDir(root)
	if err != nil {
		return nil, err
	}
	repo := &repository{root: root, projectPrefix: filepath.ToSlash(prefix), common: common, refs: map[string]gitRef{}, worktrees: map[string][]string{}, config: map[string][]string{}}
	refs, err := gitRead(root, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)%00%(push)", "refs/heads/", "refs/remotes/")
	if err != nil {
		return nil, err
	}
	for _, line := range bytes.Split(bytes.TrimSuffix(refs, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		fields := bytes.Split(line, []byte{0})
		if len(fields) != 4 {
			return nil, errors.New("invalid Git ref inventory")
		}
		repo.refs[string(fields[0])] = gitRef{oid: string(fields[1]), upstream: string(fields[2]), push: string(fields[3])}
	}
	worktrees, err := gitRead(root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var path string
	for _, field := range bytes.Split(worktrees, []byte{0}) {
		switch {
		case bytes.HasPrefix(field, []byte("worktree ")):
			path = string(bytes.TrimPrefix(field, []byte("worktree ")))
		case bytes.HasPrefix(field, []byte("branch refs/heads/")):
			branch := string(bytes.TrimPrefix(field, []byte("branch refs/heads/")))
			repo.worktrees[branch] = append(repo.worktrees[branch], path)
			if samePath(path, root) {
				repo.servedBranch = branch
			}
		}
	}
	config, err := gitRead(root, "config", "--null", "--list")
	if err != nil {
		return nil, err
	}
	for _, field := range bytes.Split(config, []byte{0}) {
		if len(field) == 0 {
			continue
		}
		key, value, _ := strings.Cut(string(field), "\n")
		repo.config[key] = append(repo.config[key], value)
	}
	return repo, nil
}

func hasGitMetadata(root string) bool {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return true
	}
	for path := absolute; ; path = filepath.Dir(path) {
		_, err := os.Lstat(filepath.Join(path, ".git"))
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			return true
		}
		if filepath.Dir(path) == path {
			return false
		}
	}
}

func commonDir(root string) (string, error) {
	common, err := gitText(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(common)
}

// worktreeHead reads a worktree's Git common directory, HEAD commit, symbolic
// HEAD, and index file path in one call; a detached HEAD has no refs/heads/
// name. --path-format=absolute applies to --git-path as well.
func worktreeHead(root string) (common, head, branch, index string, err error) {
	out, err := gitText(root, "rev-parse", "--path-format=absolute", "--git-common-dir", "HEAD^{commit}", "--symbolic-full-name", "HEAD", "--git-path", "index")
	if err != nil {
		return "", "", "", "", err
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		return "", "", "", "", errors.New("invalid Git worktree identity")
	}
	common, err = filepath.EvalSymlinks(lines[0])
	return common, lines[1], lines[2], lines[3], err
}

// refOIDs reads the named refs in one call. An absent ref maps to "".
func refOIDs(root string, names []string) (map[string]string, error) {
	oids := make(map[string]string, len(names))
	for _, name := range names {
		oids[name] = ""
	}
	if len(names) == 0 {
		return oids, nil
	}
	out, err := gitRead(root, append([]string{"for-each-ref", "--format=%(refname)%00%(objectname)"}, names...)...)
	if err != nil {
		return nil, err
	}
	for _, line := range bytes.Split(bytes.TrimSuffix(out, []byte("\n")), []byte("\n")) {
		name, oid, ok := bytes.Cut(line, []byte{0})
		if !ok {
			continue
		}
		// Patterns also match refs nested beneath a name; keep exact names only.
		if _, watched := oids[string(name)]; watched {
			oids[string(name)] = string(oid)
		}
	}
	return oids, nil
}

func samePath(a, b string) bool {
	left, leftErr := filepath.EvalSymlinks(a)
	right, rightErr := filepath.EvalSymlinks(b)
	if leftErr != nil || rightErr != nil {
		return false
	}
	left, leftErr = filepath.Abs(left)
	right, rightErr = filepath.Abs(right)
	return leftErr == nil && rightErr == nil && left == right
}

func (r *repository) value(key string) string {
	values := r.config[key]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// remoteRef mirrors configured upstream/push destination without contacting it.
// Fetch refspecs, not a hard-coded origin, translate remote names to local refs.
func (r *repository) remoteRef(branch string) (string, error) {
	local := r.refs["refs/heads/"+branch]
	if strings.HasPrefix(local.upstream, "refs/remotes/") {
		return local.upstream, nil
	}
	if strings.HasPrefix(local.push, "refs/remotes/") {
		return local.push, nil
	}
	remote := r.value("branch." + branch + ".remote")
	remoteBranch := r.value("branch." + branch + ".merge")
	if remote == "" || remoteBranch == "" || remote == "." {
		// No remote upstream: follow an explicit push destination before
		// considering the served checkout's configured publication remote.
		upstreamRemote := remote
		remote = r.value("branch." + branch + ".pushremote")
		if remote == "" {
			remote = r.value("remote.pushdefault")
		}
		if remote == "" && upstreamRemote == "." {
			return "", nil
		}
		if remote == "" {
			remote = upstreamRemote
		}
		if remote == "" {
			remote = r.value("branch." + r.servedBranch + ".pushremote")
		}
		if remote == "" {
			remote = r.value("branch." + r.servedBranch + ".remote")
		}
		if remote == "." {
			return "", nil
		}
		remoteBranch = ""
	}
	if remote == "" {
		// With one configured remote Git's default push destination is
		// unambiguous; multiple remotes require an upstream/push configuration.
		for key := range r.config {
			if strings.HasPrefix(key, "remote.") && strings.HasSuffix(key, ".url") {
				candidate := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
				if remote != "" && remote != candidate {
					return "", errors.New("multiple configured remotes have no task upstream or push destination")
				}
				remote = candidate
			}
		}
	}
	if remote == "" || remote == "." {
		return "", nil
	}
	if remoteBranch == "" {
		for _, specification := range r.config["remote."+remote+".push"] {
			destination := mapRefspec(specification, "refs/heads/"+branch)
			if destination == "" {
				continue
			}
			if remoteBranch != "" && remoteBranch != destination {
				return "", errors.New("configured push refspecs map the execution branch to multiple remote destinations")
			}
			remoteBranch = destination
		}
	}
	if remoteBranch == "" {
		remoteBranch = "refs/heads/" + branch
	}
	var mapped string
	for _, specification := range r.config["remote."+remote+".fetch"] {
		name := mapRefspec(specification, remoteBranch)
		if !strings.HasPrefix(name, "refs/remotes/") {
			continue
		}
		if mapped != "" && mapped != name {
			return "", errors.New("configured fetch refspecs map the execution branch to multiple tracking refs")
		}
		mapped = name
	}
	return mapped, nil
}

func mapRefspec(specification, source string) string {
	specification = strings.TrimPrefix(specification, "+")
	if strings.HasPrefix(specification, "^") {
		return ""
	}
	from, to, ok := strings.Cut(specification, ":")
	if !ok || to == "" {
		return ""
	}
	if from == source {
		return to
	}
	if strings.Count(from, "*") != 1 || strings.Count(to, "*") != 1 {
		return ""
	}
	prefix, suffix, _ := strings.Cut(from, "*")
	if !strings.HasPrefix(source, prefix) || !strings.HasSuffix(source, suffix) || len(source) < len(prefix)+len(suffix) {
		return ""
	}
	return strings.Replace(to, "*", source[len(prefix):len(source)-len(suffix)], 1)
}

func (r *repository) selectSource(branch, taskPath string) (*source, []Diagnostic, error) {
	localName := "refs/heads/" + branch
	local := r.refs[localName]
	remoteName, remoteErr := r.remoteRef(branch)
	var diagnostics []Diagnostic
	if remoteErr != nil {
		actualBranch := local.oid != "" || len(r.worktrees[branch]) != 0
		for name := range r.refs {
			if strings.HasPrefix(name, "refs/remotes/") && strings.HasSuffix(name, "/"+branch) {
				actualBranch = true
				break
			}
		}
		if !actualBranch {
			return nil, nil, nil
		}
		diagnostics = append(diagnostics, Diagnostic{Branch: branch, Code: "remote_ambiguous", Message: remoteErr.Error()})
	}
	remote := r.refs[remoteName]
	freshness := ""
	if local.oid != "" && remote.oid != "" && local.oid != remote.oid {
		localBeforeRemote, err := ancestor(r.root, local.oid, remote.oid)
		if err != nil {
			return nil, diagnostics, err
		}
		remoteBeforeLocal, err := ancestor(r.root, remote.oid, local.oid)
		if err != nil {
			return nil, diagnostics, err
		}
		if !localBeforeRemote && !remoteBeforeLocal {
			diagnostics = append(diagnostics, Diagnostic{Branch: branch, Code: "divergent_refs", Message: fmt.Sprintf("local %s (%s) and tracking %s (%s) diverge; no execution observation selected", localName, local.oid, remoteName, remote.oid)})
			return nil, diagnostics, nil
		}
		// Related tips are provenance on the selected record, not a visibility problem.
		freshness = "remote_tracking_behind"
		if localBeforeRemote {
			freshness = "local_branch_behind"
		}
	}
	paths := r.worktrees[branch]
	for _, path := range paths {
		if samePath(path, r.root) {
			paths = []string{path}
			break
		}
	}
	if len(paths) > 1 {
		diagnostics = append(diagnostics, Diagnostic{Branch: branch, Code: "worktree_ambiguous", Message: "more than one worktree holds this execution branch; no observation selected"})
		return nil, diagnostics, nil
	}
	src := &source{repo: r, branch: branch, taskPath: taskPath, ref: localName, oid: local.oid, remoteRef: remoteName, remoteOID: remote.oid, freshness: freshness, files: map[string]fileState{}, directories: map[string]string{}}
	if len(paths) == 1 {
		src.kind, src.worktreeRoot = "working_tree", paths[0]
		src.root = filepath.Join(src.worktreeRoot, filepath.FromSlash(r.projectPrefix))
		if err := src.captureWorktree(); err != nil {
			return nil, diagnostics, err
		}
		return src, diagnostics, nil
	}
	if local.oid != "" {
		src.kind = "local_branch"
	} else if remote.oid != "" {
		src.kind, src.ref, src.oid = "remote_tracking", remoteName, remote.oid
	} else {
		return nil, diagnostics, nil
	}
	entries, err := commitTrees.inventory(r.root, r.common, src.oid)
	if err != nil {
		return nil, diagnostics, err
	}
	src.entries = entries
	return src, diagnostics, nil
}

func ancestor(root, before, after string) (bool, error) {
	_, err := gitRead(root, "merge-base", "--is-ancestor", before, after)
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func readTree(root, oid string) (map[string]treeEntry, error) {
	out, err := gitRead(root, "ls-tree", "-r", "-t", "-z", oid)
	if err != nil {
		return nil, err
	}
	entries := map[string]treeEntry{}
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		metadata, name, ok := strings.Cut(string(entry), "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 {
			return nil, errors.New("invalid Git tree inventory")
		}
		entries[name] = treeEntry{mode: fields[0], kind: fields[1], oid: fields[2]}
	}
	return entries, nil
}

// Follow the lifecycle branch-name rules without accepting revision shorthand.
func validBranch(branch string) bool {
	if branch == "" || branch == "HEAD" || branch == "@" || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "refs/") || strings.HasSuffix(branch, ".") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
		return false
	}
	for _, c := range branch {
		if c <= ' ' || c == 127 || strings.ContainsRune("~^:?*[\\", c) {
			return false
		}
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
