package vcs

import (
	"path"
	"strings"
)

// Tracked reports which of paths, project-relative like every path here, git's
// index holds: a file that is itself in the index, or a directory with at least
// one indexed file beneath it. A path git tracks nothing at cannot be named to
// `git add`: once it is gone from the working tree too, the pathspec matches
// nothing and git refuses the whole command. It shells one `git ls-files -z`
// over every path, which names files relative to Dir.
func (g *Git) Tracked(paths []string) (map[string]bool, error) {
	tracked := map[string]bool{}
	if len(paths) == 0 {
		return tracked, nil
	}
	wanted := make(map[string]bool, len(paths))
	for _, p := range paths {
		wanted[path.Clean(p)] = true
	}
	out, err := g.runRaw(append([]string{"--literal-pathspecs", "ls-files", "-z", "--"}, paths...)...)
	if err != nil {
		return nil, g.asEnv(err)
	}
	for _, name := range strings.Split(out, "\x00") {
		for at := name; at != "" && at != "." && at != "/"; at = path.Dir(at) {
			if wanted[at] {
				tracked[at] = true
				break
			}
		}
	}
	return tracked, nil
}
