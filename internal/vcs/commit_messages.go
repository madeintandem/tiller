package vcs

import (
	"fmt"
	"strings"
)

// CommitMessage is one commit and what its author wrote: the subject line on
// its own, and the whole message (subject and body) as committed.
type CommitMessage struct {
	Commit  string
	Subject string
	Message string
}

// The separators git prints between and inside records. Both are ASCII control
// characters no commit message carries in practice, so a message with blank
// lines or any printable text splits cleanly.
const (
	commitRecordSeparator = "\x1e"
	commitFieldSeparator  = "\x1f"
)

// CodeCommitMessages returns the messages of the commits reachable from ref
// that change the project's code, newest first, at most limit of them. Code is
// what CodeIdentity calls code: every project path outside Tiller's own two
// directories, the state tree (.tiller/) and the gate evidence
// (artifacts/tiller/). A commit that only records Tiller state, an approval, a
// start, a gate result with its evidence, is therefore never returned. Paths
// resolve from g.Dir like every other project-relative read here, so in a
// nested project only commits that touch the project count.
//
// The path filter runs inside git, so a state-only commit is never read at
// all, and the limit counts the commits returned rather than the commits
// walked. --full-history keeps both sides of every merge in the walk:
// the default simplification follows one parent of a merge whose result equals
// that parent, which would hide the commits a merged branch landed.
//
// A ref that does not resolve is an error.
func (g *Git) CodeCommitMessages(ref string, limit int) ([]CommitMessage, error) {
	if limit <= 0 {
		return nil, nil
	}
	out, err := g.runRaw("log", "--full-history", fmt.Sprintf("--max-count=%d", limit),
		"--no-color", "--no-notes",
		"--format="+commitRecordSeparator+"%H"+commitFieldSeparator+"%s"+commitFieldSeparator+"%B",
		ref, "--", ".", ":(exclude).tiller", ":(exclude)artifacts/tiller")
	if err != nil {
		return nil, g.asEnv(err)
	}
	var messages []CommitMessage
	for _, record := range strings.Split(out, commitRecordSeparator) {
		fields := strings.SplitN(record, commitFieldSeparator, 3)
		if len(fields) < 3 {
			continue
		}
		messages = append(messages, CommitMessage{
			Commit:  strings.TrimSpace(fields[0]),
			Subject: strings.TrimSpace(fields[1]),
			Message: strings.TrimSpace(fields[2]),
		})
	}
	return messages, nil
}
