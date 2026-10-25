package execution

import (
	"maps"
	"slices"
)

// Fork returns an independent view of the snapshot for one projection. Base,
// the discovered repository, the integration event list, and every parsed tree
// inventory are shared read-only. The display tree's task indexes, the
// observations, the diagnostics, and each selected source's read bookkeeping
// are copied, so one fork's Settle or ReadFile never changes another's.
func (s *Snapshot) Fork() *Snapshot {
	out := *s
	out.Tree = displayTree(s.Tree)
	out.Observations = maps.Clone(s.Observations)
	out.Diagnostics = slices.Clone(s.Diagnostics)
	out.sources = make(map[string]*source, len(s.sources))
	for id, src := range s.sources {
		out.sources[id] = src.fork()
	}
	return &out
}

func (s *source) fork() *source {
	out := *s
	out.files = maps.Clone(s.files)
	out.directories = maps.Clone(s.directories)
	return &out
}
