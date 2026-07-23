//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func sendTermSignal(pid int) error {
	proc, err := findProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

func sendKillSignal(pid int) error {
	proc, err := findProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGKILL)
}

func processAlivePlatform(pid int) bool {
	proc, err := findProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func findProcess(pid int) (*os.Process, error) {
	return os.FindProcess(pid)
}
