package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestConcurrentQuestionWritersRefuseInsteadOfLosingUpdates(t *testing.T) {
	h := &harness{
		t:    t,
		bin:  buildTiller(t),
		dir:  t.TempDir(),
		home: t.TempDir(),
	}

	requireOK(t, h.tiller("init", "--name", "Concurrency", "--key", "concurrency", "--kind", "greenfield"), "init")
	requireOK(t, h.joinAsOwner("jc", "JC Grubbs"), "join")

	const writers = 20
	statements := make([]string, writers)
	for i := range statements {
		statements[i] = fmt.Sprintf("Concurrent question %02d?", i+1)
	}

	start := make(chan struct{})
	results := make([]result, writers)
	var wg sync.WaitGroup
	wg.Add(writers)
	for i, statement := range statements {
		go func() {
			defer wg.Done()
			<-start
			results[i] = runBinary(h.bin, h.dir, h.home,
				"add", "question", statement, "--title", "Shared concurrent question", "--contributor", "jc", "--json")
		}()
	}
	close(start)
	wg.Wait()

	// The lock-free reader can miss a newly acquired lock, then see that
	// writer's journal and return recovery_needed (#106). The clean load and
	// strict retries below distinguish that race from a retained journal.
	lawfulFirstAttemptRefusals := []string{store.ReasonLocked, store.ReasonMutationStale, store.ReasonMutationRecovery}
	successes := 0
	type refusal struct {
		writer int
		reason string
	}
	var refused []refusal
	histogram := map[string]int{}
	for i, res := range results {
		require.Containsf(t, []int{0, 3}, res.code,
			"first attempt %d exited %d\nstdout:\n%s\nstderr:\n%s", i, res.code, res.stdout, res.stderr)
		if res.code == 0 {
			successes++
			continue
		}
		var envelope struct {
			Error *struct {
				Reason string `json:"reason"`
			} `json:"error"`
		}
		require.NoErrorf(t, json.Unmarshal([]byte(res.stdout), &envelope),
			"decode refused attempt %d: %s", i, res.stdout)
		require.NotNil(t, envelope.Error)
		require.Containsf(t, lawfulFirstAttemptRefusals, envelope.Error.Reason,
			"writer %d refused for an unexpected reason\nstdout:\n%s", i, res.stdout)
		refused = append(refused, refusal{writer: i, reason: envelope.Error.Reason})
		histogram[envelope.Error.Reason]++
	}
	t.Logf("first attempts: %d succeeded; refusals by reason: %v", successes, histogram)

	tree, err := store.Load(h.dir)
	require.NoError(t, err)
	require.Len(t, tree.Questions, successes, "every successful first attempt remains in the shared ledger")

	// Every writer has finished. Each refused attempt must now succeed without
	// human recovery, including one that initially reported recovery_needed.
	for _, r := range refused {
		res := runBinary(h.bin, h.dir, h.home,
			"add", "question", statements[r.writer], "--title", fmt.Sprintf("Concurrent question %d", r.writer), "--contributor", "jc", "--json")
		requireOK(t, res, fmt.Sprintf("retry question %d after a %s refusal", r.writer, r.reason))
	}

	tree, err = store.Load(h.dir)
	require.NoError(t, err)
	require.Len(t, tree.Questions, writers)
	seen := make(map[string]int, writers)
	for _, q := range tree.Questions {
		seen[q.Statement]++
	}
	for _, statement := range statements {
		require.Equalf(t, 1, seen[statement], "question %q appears exactly once", statement)
	}

	evs, err := events.List(tree)
	require.NoError(t, err)
	questionEvents := 0
	for _, ev := range evs {
		if ev.Type == "add.question" {
			questionEvents++
		}
	}
	require.Equal(t, writers, questionEvents)

	cacheDir := filepath.Join(h.dir, ".tiller", ".cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	lockPath := filepath.Join(cacheDir, "lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held by concurrency test\n"), 0o644))
	locked := runBinary(h.bin, h.dir, h.home, "status", "--json")
	require.NoError(t, os.Remove(lockPath))
	require.Equalf(t, 3, locked.code, "locked read stdout:\n%s\nstderr:\n%s", locked.stdout, locked.stderr)
	var envelope struct {
		Error *struct {
			Reason string `json:"reason"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(locked.stdout), &envelope))
	require.NotNil(t, envelope.Error)
	require.Equal(t, store.ReasonLocked, envelope.Error.Reason)
}

func runBinary(bin, dir, home string, args ...string) result {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = cleanEnv(home)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
			errb.WriteString(err.Error())
		}
	}
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}
