package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const commandOutputLimit = 1024 * 1024

// limitedOutput continues draining after its limit: a noisy subprocess must not
// consume unbounded memory or deadlock while reporting a failed installation.
type limitedOutput struct {
	bytes.Buffer
	truncated bool
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := commandOutputLimit - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func executableCommand(ctx context.Context, timeout time.Duration, path string, env []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	var stdout, stderr limitedOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		// The direct child exited while descendants retained its output pipes.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("running %s: %w", path, ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("running %s %v: %w: %s", path, args, err, stderr.String())
	}
	if stdout.truncated || stderr.truncated {
		return nil, fmt.Errorf("running %s: output exceeded %d bytes", path, commandOutputLimit)
	}
	return stdout.Bytes(), nil
}
