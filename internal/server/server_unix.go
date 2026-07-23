//go:build !windows

package server

import (
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var interruptSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

func isAddrInUse(err error) bool {
	if op, ok := err.(*net.OpError); ok {
		if sys, ok := op.Err.(*os.SyscallError); ok {
			return sys.Err == syscall.EADDRINUSE
		}
	}
	return strings.Contains(err.Error(), "address already in use")
}
