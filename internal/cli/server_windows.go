//go:build windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func sendTermSignal(pid int) error {
	proc, err := findProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func sendKillSignal(pid int) error {
	proc, err := findProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func processAlivePlatform(pid int) bool {
	proc, err := findProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func findProcess(pid int) (*os.Process, error) {
	return os.FindProcess(pid)
}
