//go:build windows

package client

import (
	"os"
	"syscall"
)

func bridgeSysProcAttr() *syscall.SysProcAttr {
	return nil
}

func terminateProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err == nil {
		return true
	}
	return false
}
