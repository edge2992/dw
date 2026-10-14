//go:build unix

package summarize

import (
	"os"
	"os/exec"
	"syscall"
)

// Spawn starts `dw summarize --from-hook …` for job as a detached process and
// returns without waiting for it. The child gets its own session (Setsid), so
// hook timeouts, Ctrl-C and closing the terminal do not reach it, and its
// stdout/stderr go to the log file: Claude Code waits for the hook's own
// pipes to close, so inheriting them would block the hook for the whole LLM
// call.
func Spawn(job Job) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := openLog()
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(self, "summarize", "--from-hook",
		"--session", job.Session, "--transcript", job.Transcript, job.Workspace)
	cmd.Stdin = nil // /dev/null
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	Logf("hook: spawned worker pid=%d ws=%s session=%s", cmd.Process.Pid, shortWS(job.Workspace), shortSession(job.Session))
	return cmd.Process.Release()
}
