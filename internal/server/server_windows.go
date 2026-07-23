//go:build windows

package server

import (
	"os"
)

var interruptSignals = []os.Signal{os.Interrupt}

func isAddrInUse(err error) bool {
	return false
}
