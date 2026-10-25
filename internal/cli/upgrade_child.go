package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/madeintandem/tiller/internal/store"
)

type maintenanceEnvelope struct {
	OK      bool           `json:"ok"`
	Command string         `json:"command"`
	Data    *upgradeResult `json:"data"`
	Error   *ErrorBody     `json:"error"`
}

type maintenanceOutput struct {
	bytes.Buffer
	truncated bool
}

func (b *maintenanceOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 8*1024*1024 - b.Len(); len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func runMaintenanceChild(parent context.Context, selected selfupdate.ExecutableIdentity, root, owner string, inspect bool) (upgradeResult, error) {
	r := newMaintenanceResult(root, inspect)
	r.Project.Status = "unknown"
	r.Project.Migration.Status = "unknown"
	r.Project.Artifacts.Status = "unknown"
	if inspect {
		r.Project.Status = "unchanged"
		r.Project.Migration.Status = "unchanged"
		r.Project.Artifacts.Status = "unchanged"
	}
	if err := selfupdate.CheckExecutable(selected); err != nil {
		return r, maintenanceError("executable_changed", err.Error())
	}
	ctx, cancel := context.WithTimeout(parent, maintenanceTimeout)
	defer cancel()
	args := []string{"upgrade", "--project-only", "--json", "--upgrade-handoff", "--expected-executable-sha256", selected.SHA256}
	if inspect {
		args = append(args, "--inspect")
	}
	if owner != "" {
		args = append(args, "--contributor", owner)
	}
	child := exec.CommandContext(ctx, selected.Path, args...)
	child.Dir = root
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.Cancel = func() error {
		err := syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	child.WaitDelay = 2 * time.Second
	var stdout, stderr maintenanceOutput
	child.Stdout = &stdout
	child.Stderr = &stderr
	err := child.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
	}
	if ctx.Err() != nil {
		return r, maintenanceError("maintenance_child_timeout", "target maintenance timed out; project durability is unknown after apply starts")
	}
	if stdout.truncated || stderr.truncated {
		return r, maintenanceError("maintenance_protocol_invalid", "target maintenance output exceeded its bounded response size")
	}
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else {
			return r, maintenanceError("maintenance_child_failed", fmt.Sprintf("target maintenance did not return a final result: %v", err))
		}
	}
	parsed, decodeErr := decodeMaintenanceChild(stdout.Bytes(), code, selected, root, inspect)
	if decodeErr != nil {
		return r, decodeErr
	}
	return parsed.result, parsed.err
}

type decodedMaintenance struct {
	result upgradeResult
	err    error
}

func decodeMaintenanceChild(data []byte, exit int, selected selfupdate.ExecutableIdentity, root string, inspect bool) (decodedMaintenance, error) {
	bad := func(message string) (decodedMaintenance, error) {
		return decodedMaintenance{}, maintenanceError("maintenance_protocol_invalid", message)
	}
	var envelope maintenanceEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return bad("target returned no valid maintenance JSON; no project completion can be established")
	}
	if envelope.Command != "upgrade" || (envelope.Data == nil) == (envelope.Error == nil) {
		return bad("target returned an invalid maintenance envelope")
	}
	var r *upgradeResult
	var childErr error
	if envelope.Error != nil {
		e := envelope.Error
		if envelope.OK || (exit != 2 && exit != 3) || e.Code != exit || e.Reason == "" || e.Message == "" {
			return bad("target error envelope disagrees with its process exit")
		}
		r = e.PartialResult
		childErr = &store.EnvError{Code: e.Code, Reason: e.Reason, Msg: e.Message}
	} else {
		r = envelope.Data
		if (exit == 0) != envelope.OK || (exit != 0 && exit != 1) {
			return bad("target data envelope disagrees with its process exit")
		}
		if exit == 1 {
			if r.Failure == nil || r.Failure.Code != 1 {
				return bad("target refusal omits its phase failure")
			}
			childErr = &store.EnvError{Code: 1, Reason: r.Failure.Reason, Msg: r.Failure.Message}
		}
	}
	if r == nil || r.Protocol != maintenanceProtocol || r.Inspect != inspect || r.Executing == nil {
		return bad("target does not implement the selected maintenance protocol")
	}
	if r.Executing.Version != selected.Version || r.Executing.SHA256 != selected.SHA256 || filepath.Clean(r.Executing.Path) != filepath.Clean(selected.Path) || filepath.Clean(r.Project.Root) != filepath.Clean(root) {
		return bad("target maintenance result does not identify the selected executable and checkout")
	}
	if r.Complete != (exit == 0) {
		return bad("target completion disagrees with its process exit")
	}
	if exit == 0 {
		if inspect {
			if r.Project.Status != "inspected" {
				return bad("target did not complete inspection")
			}
		} else if r.Project.Status != "complete" || (r.Project.Migration.Status != "current" && r.Project.Migration.Status != "migrated") || (r.Project.Artifacts.Status != "current" && r.Project.Artifacts.Status != "refreshed") {
			return bad("target success omits completed project phases")
		}
	}
	return decodedMaintenance{result: *r, err: childErr}, nil
}
