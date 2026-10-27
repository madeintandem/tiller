package usage

import (
	"bufio"
	"errors"
	"io"
	"os"
)

// LogRef names one harness log file. Parent is the session id of the
// top-level log a subagent log belongs to, and "" for a top-level log.
type LogRef struct {
	Harness, Path, Session, Parent string
}

// Roots says where to look. Project is the checkout root (Project[0]) plus
// live worktree paths, absolute and cleaned. Env reads an environment variable and may be nil.
type Roots struct {
	Home    string
	Env     func(string) string
	Project []string
}

func (r Roots) env(key string) string {
	if r.Env == nil {
		return ""
	}
	return r.Env(key)
}

// Reader turns one harness's logs into neutral sessions.
type Reader interface {
	Harness() string
	// Discover lists the logs whose working directory is under a project root.
	Discover(r Roots) ([]LogRef, error)
	// Parse receives one top-level log and its subagent logs.
	Parse(logs []LogRef) (Session, error)
}

const (
	scanBufferBytes = 1 << 20
	maxLineBytes    = 8 << 20
)

// scanLines calls visit with every line of the file for which keep returns
// true. A line longer than maxLineBytes is skipped by draining to its newline;
// it is never held whole. The line passed to keep and visit is only valid for
// the call.
func scanLines(path string, keep func(line []byte) bool, visit func(line []byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, scanBufferBytes)
	var pending []byte
	oversized := false
	for {
		chunk, err := br.ReadSlice('\n')
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) && err != io.EOF {
			return err
		}
		if !oversized {
			if len(pending)+len(chunk) > maxLineBytes {
				oversized = true
				pending = nil
			} else if err == nil || err == io.EOF {
				var line []byte
				if len(pending) == 0 {
					line = chunk
				} else {
					line = append(pending, chunk...)
				}
				pending = pending[:0]
				line = trimEOL(line)
				if len(line) > 0 && keep(line) {
					if verr := visit(line); verr != nil {
						return verr
					}
				}
			} else {
				pending = append(pending, chunk...)
			}
		}
		if err == nil || err == io.EOF {
			oversized = false
			pending = pending[:0]
		}
		if err == io.EOF {
			return nil
		}
	}
}

func trimEOL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
